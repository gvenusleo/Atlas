package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Run 执行一次 run：追加 prompts 后驱动 turn 循环，直到模型收尾、硬退出或触顶 MaxTurns；
// 新消息追加进 rc.Messages 并同步返回；事件从调用 goroutine 串行发出。
func Run(ctx context.Context, prompts []Message, rc *LoopContext, emit func(Event)) []Message {
	e := &emitter{emit: emit}
	newMessages := make([]Message, 0, len(prompts))
	add := func(m Message) {
		rc.Messages = append(rc.Messages, m)
		newMessages = append(newMessages, m)
	}
	record := func(m Message) { newMessages = append(newMessages, m) }
	comp := &compactor{rc: rc, e: e, record: record}

	e.send(Event{Type: EventTypeAgentStart})
	for _, p := range prompts {
		add(p)
		e.messagePair(p)
	}

	for turn := 0; ; turn++ {
		if rc.MaxTurns > 0 && turn >= rc.MaxTurns {
			// 以系统消息入史留痕，供 UI 与会话回放解释 run 为何提前收尾。
			add(SystemMessage{
				Type:      SystemMessageTypeNotice,
				Content:   fmt.Sprintf("run 终止：已达 MaxTurns=%d 上限", rc.MaxTurns),
				Timestamp: time.Now(),
			})
			e.messagePair(rc.Messages[len(rc.Messages)-1])
			break
		}

		comp.startTurn(ctx)
		e.send(Event{Type: EventTypeTurnStart})
		msg := streamResponse(ctx, rc, e, func() bool { return comp.try(ctx) })
		add(msg)

		// 正常结束的回复若仍包含工具调用，也要执行；失败、取消或截断的回复不执行工具。
		var toolResults []Message
		complete := func(m ToolResultMessage) {
			add(m)
			e.messagePair(m)
			toolResults = append(toolResults, m)
		}
		switch calls := toolCallsOf(&msg); {
		case isHardStop(msg.StopReason):
		case len(calls) == 0:
		case msg.StopReason == StopReasonMaxOutputTokens:
			failTruncated(calls, e, complete)
		default:
			executeToolCalls(ctx, rc, calls, e, complete)
		}

		e.send(Event{Type: EventTypeTurnEnd, Message: msg, Messages: toolResults})

		if len(toolResults) == 0 || ctx.Err() != nil {
			break
		}
	}

	e.send(Event{Type: EventTypeAgentEnd, Messages: newMessages})
	return newMessages
}

// streamResponse 调用模型并发出消息生命周期事件，补齐模型名、时间戳并检查终态。
// onOverflow 在请求超出上下文窗口时调用，返回 true 表示已压缩、可以重发。
func streamResponse(
	ctx context.Context, rc *LoopContext, e *emitter, onOverflow func() bool,
) AssistantMessage {
	partial := AssistantMessage{Model: rc.Model, StopReason: StopReasonPending, Timestamp: time.Now()}
	e.send(Event{Type: EventTypeMessageStart, Message: partial})

	request := func() StreamRequest {
		return StreamRequest{
			Model:         rc.Model,
			SystemPrompt:  rc.SystemPrompt,
			Messages:      rc.Messages,
			Tools:         rc.Tools,
			ThinkingLevel: rc.ThinkingLevel,
		}
	}
	emit := func(d Delta) { e.send(Event{Type: EventTypeMessageUpdate, Delta: d}) }
	final := streamWithRetry(ctx, rc, e, request, emit, onOverflow)

	if final.StopReason == "" {
		final.StopReason = StopReasonError
		final.ErrorMessage = "StreamFn 违反契约：返回了空 StopReason 的消息"
	}
	final.Model = rc.Model // 记下产生它的模型：以 LoopContext 为准，不论流函数回报的是什么
	final.Timestamp = time.Now()
	e.send(Event{Type: EventTypeMessageEnd, Message: final})
	return final
}

// streamWithRetry 调用模型；暂时性的失败按策略退避后重试，失败的尝试整个作废：
// 只返回最后一次的结果，所以中间的失败不会入历史，半截的工具调用也不会被执行。
// 退避期间 ctx 取消则按取消收尾。上下文溢出时重试没用：onOverflow 非空就交给它压缩，
// 它返回 true 就重发，只给一次机会。request 每次尝试时重新构造，因为压缩会改变历史。
func streamWithRetry(
	ctx context.Context, rc *LoopContext, e *emitter,
	request func() StreamRequest, emit func(Delta), onOverflow func() bool,
) AssistantMessage {
	overflowed := false
	for retries := 0; ; {
		final := rc.Stream(ctx, request(), emit)
		if final.StopReason != StopReasonError {
			return final
		}
		switch {
		case final.ErrorKind == ErrorKindContextOverflow:
			if overflowed || onOverflow == nil || !onOverflow() {
				return final
			}
			overflowed = true
		case final.ErrorKind == ErrorKindRetryable && retries < rc.Retry.Max:
			retries++
			info := RetryInfo{
				Attempt: retries, Max: rc.Retry.Max,
				Delay: rc.Retry.delay(retries), Error: final.ErrorMessage,
			}
			e.send(Event{Type: EventTypeRetry, Retry: info})
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
func failTruncated(calls []ToolCall, e *emitter, complete func(ToolResultMessage)) {
	for _, call := range calls {
		e.send(toolStart(call))
		result := failResult(fmt.Sprintf(
			"工具调用 %q 未执行：响应撞上输出上限，参数可能不完整；请带完整参数重发", call.Name))
		e.send(toolEnd(call, result))
		complete(newToolResultMessage(call, result))
	}
}

// maxParallel 是并发执行的只读调用数的上限，免得一轮里的大批读取同时占满内存。
const maxParallel = 8

// executeToolCalls 执行一轮里的调用并同步交付结果，交付顺序与调用顺序一致。连续的只读调用
// 并发执行，其余的调用按顺序一个个执行；取消后为剩余调用补齐错误结果。
func executeToolCalls(
	ctx context.Context, rc *LoopContext, calls []ToolCall, e *emitter, complete func(ToolResultMessage),
) {
	for len(calls) > 0 {
		n := batchLen(rc.Tools, calls)
		runBatch(ctx, rc, calls[:n], e, complete)
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

// runBatch 先发出整批的开始事件，并发执行，再按调用顺序发出结束事件并交付结果。
func runBatch(
	ctx context.Context, rc *LoopContext, batch []ToolCall,
	e *emitter, complete func(ToolResultMessage),
) {
	for _, call := range batch {
		e.send(toolStart(call))
	}
	results := make([]ToolResult, len(batch))
	var wg sync.WaitGroup
	for i, call := range batch {
		wg.Go(func() { results[i] = runCall(ctx, rc, call) })
	}
	wg.Wait()
	for i, call := range batch {
		e.send(toolEnd(call, results[i]))
		complete(newToolResultMessage(call, results[i]))
	}
}

// runCall 执行一次调用：工具不存在或已取消时直接给出错误结果。
func runCall(ctx context.Context, rc *LoopContext, call ToolCall) ToolResult {
	tool := findTool(rc.Tools, call.Name)
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
func toolCallsOf(msg *AssistantMessage) []ToolCall {
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

// toolStart 构造工具开始事件，携带调用标识与原始参数。
func toolStart(call ToolCall) Event {
	return Event{
		Type:       EventTypeToolExecutionStart,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Arguments:  call.Arguments,
	}
}

// toolEnd 构造工具结束事件，成功或失败由 Result.IsError 表示。
func toolEnd(call ToolCall, result ToolResult) Event {
	return Event{
		Type:       EventTypeToolExecutionEnd,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Result:     result,
	}
}

// emitter 为事件补时间戳后转发；emit 为 nil 时丢弃。
type emitter struct {
	emit func(Event)
}

// send 补齐事件时间戳并同步转发，未设置回调时丢弃事件。
func (e *emitter) send(ev Event) {
	ev.Timestamp = time.Now()
	if e.emit != nil {
		e.emit(ev)
	}
}

// messagePair 为无需流式更新的完整消息连续发出开始、结束事件。
func (e *emitter) messagePair(m Message) {
	e.send(Event{Type: EventTypeMessageStart, Message: m})
	e.send(Event{Type: EventTypeMessageEnd, Message: m})
}
