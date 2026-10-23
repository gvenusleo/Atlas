package agent

import (
	"context"
	"encoding/json/jsontext"
	"slices"
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
