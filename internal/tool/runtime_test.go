package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeTool struct{}

func (fakeTool) Definition() Definition {
	return Definition{Name: "fake"}
}

func (fakeTool) Execute(_ context.Context, raw json.RawMessage) Result {
	return Result{Content: string(raw)}
}

func TestRuntimeExecutesKnownTool(t *testing.T) {
	rt := NewRuntime(fakeTool{})
	got := rt.Execute(context.Background(), "fake", `{"ok":true}`)
	if got.Error || got.Content != `{"ok":true}` {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestRuntimeReturnsErrorForUnknownTool(t *testing.T) {
	rt := NewRuntime()
	got := rt.Execute(context.Background(), "missing", `{}`)
	if !got.Error || !strings.Contains(got.Content, "unknown tool") {
		t.Fatalf("unexpected result: %+v", got)
	}
}
