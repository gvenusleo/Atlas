package agent

import (
	"context"
	"encoding/json/jsontext"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 同一轮里连续的只读调用并发执行，其余调用按顺序一个个执行；结果与事件的顺序始终与调用顺序一致。
func TestExecuteToolCallsBatches(t *testing.T) {
	var mu sync.Mutex
	var trace []string
	note := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		trace = append(trace, s)
	}
	finish := func(name string) func(context.Context, string, jsontext.Value) (ToolResult, error) {
		return func(context.Context, string, jsontext.Value) (ToolResult, error) {
			note("end " + name)
			return ToolResult{}, nil
		}
	}

	// a 与 b 都在执行时才会放行：串行执行的话，先开始的那个永远等不到另一个。
	var barrier sync.WaitGroup
	barrier.Add(2)
	waiting := func(name string) Tool {
		return Tool{Name: name, ReadOnly: true,
			Execute: func(ctx context.Context, id string, p jsontext.Value) (ToolResult, error) {
				barrier.Done()
				barrier.Wait()
				return finish(name)(ctx, id, p)
			}}
	}
	tools := []Tool{
		waiting("a"), waiting("b"),
		{Name: "w", Execute: finish("w")},                 // 会改动状态，不能与别的调用并发
		{Name: "c", ReadOnly: true, Execute: finish("c")}, // 只读，但前面隔着 w，自成一批
	}
	calls := []ToolCall{
		{ID: "1", Name: "a"}, {ID: "2", Name: "b"}, {ID: "3", Name: "w"}, {ID: "4", Name: "c"},
	}

	var events, delivered []string
	r := newRun(Input{Tools: tools}, func(ev Event) {
		switch ev.Type {
		case EventToolStart:
			events = append(events, "start "+ev.Call.ID)
		case EventMessageEnd:
			id := ev.Message.(ToolResultMessage).ToolCallID
			events, delivered = append(events, "end "+id), append(delivered, id)
		}
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		executeToolCalls(t.Context(), r, calls)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("只读调用没有并发执行：a 与 b 互相等不到对方")
	}

	if want := []string{"1", "2", "3", "4"}; !slices.Equal(delivered, want) {
		t.Errorf("结果交付顺序 = %v，期望 %v", delivered, want)
	}
	want := []string{"start 1", "start 2", "end 1", "end 2", "start 3", "end 3", "start 4", "end 4"}
	if !slices.Equal(events, want) {
		t.Errorf("事件顺序 = %v，期望 %v", events, want)
	}
	pos := func(s string) int { return slices.Index(trace, s) }
	if !(pos("end a") >= 0 && pos("end b") >= 0 && pos("end a") < pos("end w") &&
		pos("end b") < pos("end w") && pos("end w") < pos("end c")) {
		t.Errorf("执行顺序 = %v，期望 a、b 先于 w，w 先于 c", trace)
	}
}

// 插入的新消息应参加容量判断，并在压缩及重放时保留原文。
func TestRunCompactsAfterInsertingPrompts(t *testing.T) {
	opening := UserMessage{Content: []Content{TextContent{Text: strings.Repeat("始", 375)}}}
	inserted := UserMessage{Content: []Content{TextContent{Text: strings.Repeat("新", 525)}}}
	var ordinary int
	in := Input{Model: Model{ContextWindow: 1000}}
	in.Model.Stream = func(_ context.Context, req StreamRequest, _ func(Delta)) AssistantMessage {
		if req.SystemPrompt == summarySystemPrompt {
			return AssistantMessage{StopReason: StopReasonStop,
				Content: []Content{TextContent{Text: "摘要"}}}
		}
		ordinary++
		if ordinary == 1 {
			return AssistantMessage{StopReason: StopReasonStop, Usage: Usage{TotalTokens: 900},
				Content: []Content{TextContent{Text: strings.Repeat("旧", 280)}}}
		}
		if Tokens(req.Messages) > 800 {
			return AssistantMessage{StopReason: StopReasonError,
				ErrorKind: ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
		}
		if last, ok := req.Messages[len(req.Messages)-1].(UserMessage); !ok ||
			TextOf(last.Content) != TextOf(inserted.Content) {
			t.Error("压缩后的模型请求丢失了插入消息的原文")
		}
		return AssistantMessage{StopReason: StopReasonStop}
	}
	in.PollPrompts = func(_, _, _ bool) ([]UserMessage, bool) {
		if ordinary == 1 {
			return []UserMessage{inserted}, true
		}
		return nil, false
	}
	messages := Run(t.Context(), in, opening, nil)
	last, ok := messages[len(messages)-1].(AssistantMessage)
	if !ok || last.StopReason != StopReasonStop || ordinary != 2 {
		t.Fatalf("插入后没有完成下一次模型请求：请求数=%d，最后一条=%+v", ordinary, last)
	}
	var replay []Message
	for _, message := range messages {
		replay = Fold(replay, message)
	}
	if len(replay) != 3 || !isCompaction(replay[0]) || replay[1].Role() != RoleUser {
		t.Fatalf("重放没有恢复摘要和插入的用户消息：共 %d 条", len(replay))
	}
}

// 超限后可以在预压缩之外恢复一次，且不会重跑已经执行过的工具。
func TestRunRecoversOversizedToolResult(t *testing.T) {
	var ordinary, summaries, executed int
	old := UserMessage{Content: []Content{TextContent{Text: strings.Repeat("旧", 500)}}}
	output := "工具开头" + strings.Repeat("长", 3000) + "工具结论"
	in := Input{Messages: []Message{old}, Model: Model{ContextWindow: 1000}}
	in.Tools = []Tool{{Name: "t", Execute: func(context.Context, string, jsontext.Value) (ToolResult, error) {
		executed++
		return ToolResult{Content: []Content{TextContent{Text: output}}}, nil
	}}}
	in.Model.Stream = func(_ context.Context, req StreamRequest, _ func(Delta)) AssistantMessage {
		if req.SystemPrompt == summarySystemPrompt {
			summaries++
			source := TextOf(req.Messages[0].(UserMessage).Content)
			if summaries == 1 && (ordinary != 1 || strings.Contains(source, "工具结论")) {
				t.Error("工具续答前应先压缩更早的上下文，保留工具调用及结果")
			}
			if summaries == 2 && (ordinary != 2 || !strings.Contains(source, "工具结论")) {
				t.Error("续答超限后应再压缩一次，并将工具结果结尾交给摘要模型")
			}
			return AssistantMessage{StopReason: StopReasonStop,
				Content: []Content{TextContent{Text: "摘要"}}}
		}
		ordinary++
		switch ordinary {
		case 1:
			return AssistantMessage{StopReason: StopReasonToolUse,
				Content: []Content{ToolCall{ID: "1", Name: "t"}}}
		case 2:
			return AssistantMessage{StopReason: StopReasonError,
				ErrorKind: ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
		}
		if len(req.Messages) != 1 || !isCompaction(req.Messages[0]) {
			t.Error("恢复请求仍带有已摘要的工具调用或结果")
		}
		return AssistantMessage{StopReason: StopReasonStop}
	}
	added := Run(t.Context(), in, UserMessage{Content: []Content{TextContent{Text: "开始"}}}, nil)
	if ordinary != 3 || summaries != 2 || executed != 1 || !isCompaction(added[len(added)-2]) {
		t.Fatalf("请求 %d 次、摘要 %d 次、工具 %d 次，期望 3、2、1 且写入恢复摘要",
			ordinary, summaries, executed)
	}
}

// 摘要重试只缩减摘要来源。普通请求重发时仍须携带新增用户消息和附件。
func TestRunBoundsContextOverflowRecovery(t *testing.T) {
	for _, summaryFits := range []bool{true, false} {
		t.Run(strconv.FormatBool(summaryFits), func(t *testing.T) {
			checkContextOverflowRecovery(t, summaryFits)
		})
	}
}

func checkContextOverflowRecovery(t *testing.T, summaryFits bool) {
	t.Helper()
	old := UserMessage{Content: []Content{TextContent{Text: strings.Repeat("最旧", 1800)}}}
	recent := AssistantMessage{Content: []Content{TextContent{Text: "最近进展"}}}
	image := ImageContent{Data: "AQID", MIMEType: "image/png"}
	user := UserMessage{Content: []Content{TextContent{Text: "新消息"}, image}}
	overflow := AssistantMessage{StopReason: StopReasonError,
		ErrorKind: ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
	var ordinary int
	var summaries []string
	in := Input{Messages: []Message{old, recent}, Model: Model{Images: true}}
	in.Model.Stream = func(_ context.Context, req StreamRequest, _ func(Delta)) AssistantMessage {
		if req.SystemPrompt == summarySystemPrompt {
			text := TextOf(req.Messages[0].(UserMessage).Content)
			summaries = append(summaries, text)
			if len(summaries) == 2 && summaryFits {
				return AssistantMessage{StopReason: StopReasonStop,
					Content: []Content{TextContent{Text: "摘要"}}}
			}
			return overflow
		}
		ordinary++
		last, ok := req.Messages[len(req.Messages)-1].(UserMessage)
		if !ok || !slices.Equal(last.Content, user.Content) {
			t.Error("请求丢失新用户消息或附件")
		}
		return overflow
	}
	added := Run(t.Context(), in, user, nil)
	if len(summaries) != 2 || estimateText(summaries[1]) > estimateText(summaries[0])/2 ||
		!strings.Contains(summaries[1], "最近进展") {
		t.Fatalf("摘要请求需要缩小到一半并保留近期进展，实际调用 %d 次", len(summaries))
	}
	wantRequests := 1
	if summaryFits {
		wantRequests = 2
	}
	if ordinary != wantRequests || slices.ContainsFunc(added, isCompaction) ||
		added[len(added)-1].(AssistantMessage).ErrorKind != ErrorKindContextOverflow {
		t.Fatalf("请求 %d 次，期望 %d 次，重发失败时不应保存恢复摘要", ordinary, wantRequests)
	}
}

// 摘要输入紧张时，最近任务的验收条件优先于工具结果。
func TestRunKeepsUserGoalWhenShrinkingSummary(t *testing.T) {
	const goal = "最近任务的唯一验收条件"
	in := Input{Messages: []Message{
		SystemMessage{Type: SystemMessageTypeCompaction, Content: "此前的摘要"},
		UserMessage{Content: []Content{TextContent{Text: goal}}},
	}}
	for i := range 10 {
		id := strconv.Itoa(i)
		in.Messages = append(in.Messages,
			AssistantMessage{StopReason: StopReasonToolUse,
				Content: []Content{ToolCall{ID: id, Name: "read"}}},
			ToolResultMessage{ToolCallID: id, ToolName: "read",
				Content: []Content{TextContent{Text: strings.Repeat("长", 1500)}}},
		)
	}
	var summaries int
	in.Model.Stream = func(_ context.Context, req StreamRequest, _ func(Delta)) AssistantMessage {
		if req.SystemPrompt == summarySystemPrompt {
			summaries++
			if summaries == 2 {
				text := TextOf(req.Messages[0].(UserMessage).Content)
				if !strings.Contains(text, goal) || !strings.Contains(text, "此前的摘要") {
					t.Error("缩小后的摘要请求丢失了用户目标或此前的摘要")
				}
				return AssistantMessage{StopReason: StopReasonStop,
					Content: []Content{TextContent{Text: "摘要"}}}
			}
		}
		return AssistantMessage{StopReason: StopReasonError,
			ErrorKind: ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
	}
	Run(t.Context(), in, UserMessage{Content: []Content{TextContent{Text: "继续"}}}, nil)
	if summaries != 2 {
		t.Fatalf("摘要调用 %d 次，期望 2 次", summaries)
	}
}

// 模型总是调用工具时，一轮在用完 MaxRequests 之后以通知收尾，不再请求模型；
// 最后一次回复没有工具调用的，一轮自然结束，不触发上限。
func TestRunStopsAtMaxRequests(t *testing.T) {
	requests := 0
	toolCalling := func(context.Context, StreamRequest, func(Delta)) AssistantMessage {
		requests++
		return AssistantMessage{StopReason: StopReasonToolUse,
			Content: []Content{ToolCall{ID: strconv.Itoa(requests), Name: "t"}}}
	}
	plain := func(context.Context, StreamRequest, func(Delta)) AssistantMessage {
		requests++
		return AssistantMessage{StopReason: StopReasonStop}
	}
	tools := []Tool{{Name: "t", ReadOnly: true,
		Execute: func(context.Context, string, jsontext.Value) (ToolResult, error) {
			return ToolResult{}, nil
		}}}
	play := func(stream StreamFn, max int) (added []Message, limits int) {
		requests = 0
		in := Input{Model: Model{Stream: stream}, Tools: tools, MaxRequests: max}
		added = Run(t.Context(), in, UserMessage{}, func(e Event) {
			if e.Type == EventLimit {
				limits++
			}
		})
		return added, limits
	}

	added, limits := play(toolCalling, 3)
	notice, ok := added[len(added)-1].(SystemMessage)
	if requests != 3 || limits != 1 || !ok || notice.Type != SystemMessageTypeNotice {
		t.Errorf("请求 %d 次、上限事件 %d 次、最后一条 %#v，期望 3 次、1 次、通知",
			requests, limits, added[len(added)-1])
	}
	if added, limits = play(plain, 1); requests != 1 || limits != 0 || len(added) != 2 {
		t.Errorf("没有工具调用时：请求 %d 次、上限事件 %d 次、新消息 %d 条，期望 1、0、2",
			requests, limits, len(added))
	}
}
