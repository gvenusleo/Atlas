package agent

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Run 把用户消息加进上下文，循环请求模型、执行工具，在请求之间接收插入的用户消息。
// 事件从调用 goroutine 串行发出，in 不被修改。
func Run(ctx context.Context, in Input, user UserMessage, emit func(Event)) []Message {
	r := newRun(in, emit)
	comp := &compactor{r: r}
	r.add(user)
	var pending []UserMessage
	for requests := 1; ctx.Err() == nil; requests++ {
		if requests > 1 && len(pending) == 0 {
			r.readModel()
			if in.PollPrompts != nil {
				pending, _ = in.PollPrompts(r.Model.Images, false, false)
			}
		}
		for _, prompt := range pending {
			if ctx.Err() != nil {
				return r.added
			}
			prompt.InTurn = true
			r.add(prompt)
		}
		if ctx.Err() != nil {
			break
		}
		keepMessages := len(pending)
		if requests == 1 {
			keepMessages = 1
		}
		comp.prepareRequest(ctx, keepMessages)
		if ctx.Err() != nil {
			break
		}
		msg := streamResponse(ctx, r, keepMessages)
		r.add(msg)
		if isHardStop(msg.StopReason) {
			break
		}
		calls := toolCallsOf(msg)
		if msg.StopReason == StopReasonMaxOutputTokens {
			failTruncated(r, calls)
		} else {
			executeToolCalls(ctx, r, calls)
		}
		ending := len(calls) == 0
		if ctx.Err() != nil || ending && msg.StopReason == StopReasonMaxOutputTokens {
			break
		}
		atLimit := in.MaxRequests > 0 && requests >= in.MaxRequests
		pending = nil
		var waiting bool
		if (ending || atLimit) && in.PollPrompts != nil {
			// 收尾时领取的新消息须按下一次请求的模型校验图片。
			if !atLimit {
				r.readModel()
			}
			pending, waiting = in.PollPrompts(r.Model.Images, ending, atLimit)
		}
		if atLimit && (!ending || waiting) {
			stopAtLimit(r)
			break
		}
		if ending && len(pending) == 0 {
			break
		}
	}
	return r.added
}

// stopAtLimit 在请求次数用完时收尾：追加说明原因的通知，再发出 EventLimit。
// 通知不进入上下文，此时上下文以工具结果结尾，之后的用户消息照常接上。
func stopAtLimit(r *run) {
	r.add(SystemMessage{
		Type: SystemMessageTypeNotice,
		Content: fmt.Sprintf("本轮已达到 %d 次模型请求的上限，已停止；发送消息可以继续。",
			r.MaxRequests),
		Timestamp: time.Now(),
	})
	r.emit(Event{Type: EventLimit})
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

func (r *run) readModel() {
	if r.ReadModel != nil {
		r.Model, r.Thinking = r.ReadModel()
	}
}

// add 把一条定稿的消息并进上下文，记入这一轮的结果，并发出 EventMessageEnd。
func (r *run) add(m Message) {
	r.Messages = Fold(r.Messages, m)
	r.added = append(r.added, m)
	r.emit(Event{Type: EventMessageEnd, Message: m})
}

// streamResponse 调用模型并发出开始与增量事件，补齐模型名、时间戳并检查终态。
// 超限后的临时摘要只在重发成功后保存，定稿由调用方另行保存。
func streamResponse(ctx context.Context, r *run, keepMessages int) AssistantMessage {
	r.emit(Event{Type: EventMessageStart, Message: AssistantMessage{
		Model: r.Model.Ref, StopReason: StopReasonPending, Timestamp: time.Now(),
	}})
	var recovery SystemMessage
	request := func() StreamRequest {
		messages := r.Messages
		if recovery.Content != "" {
			messages = Fold(messages, recovery)
		}
		return StreamRequest{
			Model:         r.Model.Ref,
			SystemPrompt:  r.SystemPrompt,
			Messages:      messages,
			Tools:         r.Tools,
			ThinkingLevel: r.Thinking,
		}
	}
	// 重发的回复在恢复摘要落库后一次交付，避免失败流污染实时界面。
	var streamed bool
	emit := func(d Delta) {
		if recovery.Content == "" {
			streamed = true
			r.emit(Event{Type: EventDelta, Delta: d})
		}
	}
	onOverflow := func() bool {
		recovery, _ = prepareCompaction(ctx, r, len(r.Messages)-keepMessages)
		if recovery.Content == "" {
			return false
		}
		if streamed {
			r.emit(Event{Type: EventRetry, Retry: RetryInfo{Attempt: 1, Max: 1, Error: "上下文超限"}})
		}
		return true
	}
	final := streamWithRetry(ctx, r, request, emit, onOverflow)
	if final.StopReason == "" {
		final.StopReason = StopReasonError
		final.ErrorMessage = "StreamFn 违反契约：返回了空 StopReason 的消息"
	}
	if recovery.Content != "" {
		if ctx.Err() != nil {
			final = AssistantMessage{StopReason: StopReasonCanceled}
		} else if !isHardStop(final.StopReason) {
			r.add(recovery)
		}
	}
	final.Model = r.Model.Ref
	final.Timestamp = time.Now()
	return final
}

// streamWithRetry 调用模型；暂时性的失败按策略退避后重试，失败的尝试整个作废。
// 上下文溢出时，onOverflow 可以缩减上下文并重发一次。每次尝试都重新构造请求。
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
			if overflowed || onOverflow == nil {
				return final
			}
			prepared := ctx.Err() == nil && onOverflow()
			if ctx.Err() != nil {
				return AssistantMessage{StopReason: StopReasonCanceled}
			}
			if !prepared {
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
