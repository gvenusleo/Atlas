package deepseek

import (
	"strings"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestReadSSEStreamsTextAndToolCalls(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hi "}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"README.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n"))
	events := make(chan model.StreamEvent, 8)
	if err := readSSE(input, events); err != nil {
		t.Fatal(err)
	}
	close(events)

	var text string
	var call *model.ToolCall
	var done bool
	for event := range events {
		text += event.TextDelta
		if event.ToolCall != nil {
			call = event.ToolCall
		}
		done = done || event.Done
	}
	if text != "hi " {
		t.Fatalf("unexpected text %q", text)
	}
	if call == nil || call.Name != "read_file" || call.Arguments != `{"path":"README.md"}` {
		t.Fatalf("unexpected tool call: %+v", call)
	}
	if !done {
		t.Fatal("expected done event")
	}
}

func TestToolAccumulatorFlushesInIndexOrder(t *testing.T) {
	acc := newToolAccumulator()
	acc.add(toolCallDelta{Index: 1, ID: "call_b", Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "b", Arguments: "{}"}})
	acc.add(toolCallDelta{Index: 0, ID: "call_a", Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "a", Arguments: "{}"}})

	events := make(chan model.StreamEvent, 2)
	acc.flush(events)
	close(events)

	var names []string
	for event := range events {
		names = append(names, event.ToolCall.Name)
	}
	if strings.Join(names, ",") != "a,b" {
		t.Fatalf("unexpected order: %v", names)
	}
}
