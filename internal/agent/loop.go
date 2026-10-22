package agent

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Run 把 user 加进上下文，循环请求模型、执行工具，直到模型不再调用工具或 ctx 取消；
// 返回这一轮新产生的消息。事件从调用 goroutine 串行发出；in 不被修改。
func Run(ctx context.Context, in Input, user UserMessage, emit func(Event)) []Message {
	r := newRun(in, emit)
	comp := &compactor{r: r}
	r.add(user)
	for {
		comp.startTurn(ctx)
		msg := streamResponse(ctx, r, func() bool { return comp.try(ctx) })
		r.add(msg)
		n := len(r.added)

		// 正常结束的回复若仍包含工具调用，也要执行；失败、取消或截断的回复不执行工具。
		switch calls := toolCallsOf(msg); {
		case isHardStop(msg.StopReason):
		case len(calls) == 0:
		case msg.StopReason == StopReasonMaxOutputTokens:
			failTruncated(r, calls)
		default:
			executeToolCalls(ctx, r, calls)
		}
		if len(r.added) == n || ctx.Err() != nil {
			return r.added
		}
	}
}

// run 是一轮的工作状态：Input 的副本，其中的上下文随新消息推进；added 是这一轮新产生的消息。
type run struct {
	Input
	emit  func(Event)
	added []Message
}

func newRun(in Input, emit func(Event)) *run {
	in.Messages = slices.Clip(in.Messages) // 追加时另起底层数组，不改调用方的
	if emit == nil {
		emit = func(Event) {}
	}
	return &run{Input: in, emit: emit}
}

// add 把一条定稿的消息并进上下文，记入这一轮的结果，并发出 EventMessageEnd。
func (r *run) add(m Message) {
	r.Messages = Fold(r.Messages, m)
	r.added = append(r.added, m)
	r.emit(Event{Type: EventMessageEnd, Message: m})
}

// streamResponse 调用模型并发出开始与增量事件，补齐模型名、时间戳并检查终态；定稿由调用方 add。
// onOverflow 在请求超出上下文窗口时调用，返回 true 表示已压缩、可以重发。
func streamResponse(ctx context.Context, r *run, onOverflow func() bool) AssistantMessage {
	r.emit(Event{Type: EventMessageStart, Message: AssistantMessage{
		Model: r.Model.Ref, StopReason: StopReasonPending, Timestamp: time.Now(),
	}})
	request := func() StreamRequest {
		return StreamRequest{
			Model:         r.Model.Ref,
			SystemPrompt:  r.Prompt,
			Messages:      r.Messages,
			Tools:         r.Tools,
			ThinkingLevel: r.Thinking,
		}
	}
	emit := func(d Delta) { r.emit(Event{Type: EventDelta, Delta: d}) }
	final := streamWithRetry(ctx, r, request, emit, onOverflow)

	if final.StopReason == "" {
		final.StopReason = StopReasonError
		final.ErrorMessage = "StreamFn 违反契约：返回了空 StopReason 的消息"
	}
	final.Model = r.Model.Ref // 记下产生它的模型：以 Input 为准，不论流函数回报的是什么
	final.Timestamp = time.Now()
	return final
}

// streamWithRetry 调用模型；暂时性的失败按策略退避后重试，失败的尝试整个作废：
// 只返回最后一次的结果，所以中间的失败不会入历史，半截的工具调用也不会被执行。
// 退避期间 ctx 取消则按取消收尾。上下文溢出时重试没用：onOverflow 非空就交给它压缩，
// 它返回 true 就重发，只给一次机会。request 每次尝试时重新构造，因为压缩会改变上下文。
func streamWithRetry(
	ctx context.Context, r *run,
	request func() StreamRequest, emit func(Delta), onOverflow func() bool,
) AssistantMessage {
	overflowed := false
	for retries := 0; ; {
		final := r.Model.Stream(ctx, request(), emit)
		if final.StopReason != StopReasonError {
			return final
		}
		switch {
		case final.ErrorKind == ErrorKindContextOverflow:
			if overflowed || onOverflow == nil || !onOverflow() {
				return final
			}
			overflowed = true
		case final.ErrorKind == ErrorKindRetryable && retries < retryPolicy.Max:
			retries++
			info := RetryInfo{
				Attempt: retries, Max: retryPolicy.Max,
				Delay: retryPolicy.delay(retries), Error: final.ErrorMessage,
			}
			r.emit(Event{Type: EventRetry, Retry: info})
			if !sleep(ctx, info.Delay) {
				final.StopReason, final.ErrorKind, final.ErrorMessage = StopReasonCanceled, "", ""
				return final
			}
		default:
			return final
		}
	}
}

// isHardStop 判定必须硬退出的终态：失败、取消，以及不应出现在定稿消息上的占位值。
func isHardStop(r StopReason) bool {
	switch r {
	case StopReasonError, StopReasonCanceled, StopReasonPending:
		return true
	}
	return false
}

// failTruncated 处理输出截断：流式拼出的参数能解析但可能不完整，
// 全批作废、绝不执行。
func failTruncated(r *run, calls []ToolCall) {
	for _, call := range calls {
		r.emit(Event{Type: EventToolStart, Call: call})
		r.add(newToolResultMessage(call, failResult(fmt.Sprintf(
			"工具调用 %q 未执行：响应撞上输出上限，参数可能不完整；请带完整参数重发", call.Name))))
	}
}

// maxParallel 是并发执行的只读调用数的上限，免得一轮里的大批读取同时占满内存。
const maxParallel = 8

// executeToolCalls 执行一轮里的调用并同步交付结果，交付顺序与调用顺序一致。连续的只读调用
// 并发执行，其余的调用按顺序一个个执行；取消后为剩余调用补齐错误结果。
func executeToolCalls(ctx context.Context, r *run, calls []ToolCall) {
	for len(calls) > 0 {
		n := batchLen(r.Tools, calls)
		runBatch(ctx, r, calls[:n])
		calls = calls[n:]
	}
}

// batchLen 返回 calls 开头可以一起执行的调用数：开头连续的只读调用（至多 maxParallel 个）
// 并发；开头不是只读调用时，它自己单独成批。
func batchLen(tools []Tool, calls []ToolCall) int {
	n := 0
	for n < len(calls) && n < maxParallel {
		if tool := findTool(tools, calls[n].Name); tool == nil || !tool.ReadOnly {
			break
		}
		n++
	}
	return max(n, 1)
}

// runBatch 先发出整批的开始事件，并发执行，再按调用顺序交付结果。
func runBatch(ctx context.Context, r *run, batch []ToolCall) {
	for _, call := range batch {
		r.emit(Event{Type: EventToolStart, Call: call})
	}
	results := make([]ToolResult, len(batch))
	var wg sync.WaitGroup
	for i, call := range batch {
		wg.Go(func() { results[i] = runCall(ctx, r.Tools, call) })
	}
	wg.Wait()
	for i, call := range batch {
		r.add(newToolResultMessage(call, results[i]))
	}
}

// runCall 执行一次调用：工具不存在或已取消时直接给出错误结果。
func runCall(ctx context.Context, tools []Tool, call ToolCall) ToolResult {
	tool := findTool(tools, call.Name)
	switch {
	case tool == nil:
		return failResult(fmt.Sprintf("工具 %s 不存在", call.Name))
	case ctx.Err() != nil:
		return failResult("已取消，未执行")
	}
	return executeCall(ctx, tool, call)
}

// executeCall 执行单个调用，工具的 error 与 panic 一律收敛为错误结果。
func executeCall(ctx context.Context, tool *Tool, call ToolCall) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = failResult(fmt.Sprintf("panic: %v", r))
		}
	}()

	result, err := tool.Execute(ctx, call.ID, call.Arguments)
	if err != nil {
		return failResult(err.Error())
	}
	return result
}

// findTool 按名称返回工具切片中的首个匹配项，未找到时返回 nil。
func findTool(tools []Tool, name string) *Tool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

// toolCallsOf 按内容顺序提取值形式的工具调用。
func toolCallsOf(msg AssistantMessage) []ToolCall {
	var calls []ToolCall
	for _, c := range msg.Content {
		if v, ok := c.(ToolCall); ok {
			calls = append(calls, v)
		}
	}
	return calls
}

// failResult 生成可回填模型的文本错误结果。
func failResult(msg string) ToolResult {
	return ToolResult{Content: []Content{TextContent{Text: msg}}, IsError: true}
}

// newToolResultMessage 将调用与结果配对为历史消息，并记录生成时间。
func newToolResultMessage(call ToolCall, result ToolResult) ToolResultMessage {
	return ToolResultMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    result.Content,
		IsError:    result.IsError,
		Details:    result.Details,
		Timestamp:  time.Now(),
	}
}
