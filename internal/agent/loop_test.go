package agent

import (
	"context"
	"encoding/json/jsontext"
	"slices"
	"strconv"
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
