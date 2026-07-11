package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

type fakeTool struct {
	definition model.ToolDefinition
	result     string
	err        error
	gotArgs    string
}

type fakeRunResultTool struct {
	fakeTool
	runResult RunResult
	resultErr error
}

func (t *fakeRunResultTool) RunResult(_ context.Context, arguments string) (RunResult, error) {
	t.gotArgs = arguments
	return t.runResult, t.resultErr
}

func (t *fakeTool) Definition() model.ToolDefinition {
	return t.definition
}

func (t *fakeTool) Run(_ context.Context, arguments string) (string, error) {
	t.gotArgs = arguments
	return t.result, t.err
}

func TestRegistryRun(t *testing.T) {
	ft := &fakeTool{
		definition: model.ToolDefinition{Name: "test_tool"},
		result:     "ok",
	}
	registry, err := NewRegistry(ft)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	got, err := registry.Run(context.Background(), model.ToolCall{
		Name:      "test_tool",
		Arguments: `{"path":"README.md"}`,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got.Content != "ok" {
		t.Fatalf("Run() = %q, want %q", got.Content, "ok")
	}
	if ft.gotArgs != `{"path":"README.md"}` {
		t.Fatalf("tool arguments = %q", ft.gotArgs)
	}
}

func TestRegistryRunUnknownTool(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if _, err := registry.Run(context.Background(), model.ToolCall{Name: "missing"}); err == nil {
		t.Fatal("Run() error = nil, want unknown tool error")
	}
}

func TestNewRegistryDuplicateTool(t *testing.T) {
	_, err := NewRegistry(
		&fakeTool{definition: model.ToolDefinition{Name: "test_tool"}},
		&fakeTool{definition: model.ToolDefinition{Name: "test_tool"}},
	)
	if err == nil {
		t.Fatal("NewRegistry() error = nil, want duplicate tool error")
	}
}

func TestRegistryDefinitions(t *testing.T) {
	registry, err := NewRegistry(
		&fakeTool{definition: model.ToolDefinition{
			Name:       "first",
			Parameters: map[string]any{"type": "object"},
		}},
		&fakeTool{definition: model.ToolDefinition{Name: "second"}},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	defs := registry.Definitions()
	if len(defs) != 2 {
		t.Fatalf("len(Definitions()) = %d, want 2", len(defs))
	}
	if defs[0].Name != "first" || defs[1].Name != "second" {
		t.Fatalf("Definitions() order = %#v", defs)
	}

	defs[0].Name = "changed"
	defs[0].Parameters["type"] = "changed"
	got := registry.Definitions()
	if got[0].Name != "first" {
		t.Fatalf("Definitions()[0].Name = %q, want %q", got[0].Name, "first")
	}
	if got[0].Parameters["type"] != "object" {
		t.Fatalf("Definitions()[0].Parameters[type] = %#v, want %q", got[0].Parameters["type"], "object")
	}
}

func TestRegistryRunReturnsToolError(t *testing.T) {
	want := errors.New("failed")
	registry, err := NewRegistry(&fakeTool{
		definition: model.ToolDefinition{Name: "fail"},
		err:        want,
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	_, err = registry.Run(context.Background(), model.ToolCall{Name: "fail"})
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestRegistryRunUsesStructuredResultOnError(t *testing.T) {
	want := errors.New("partial failure")
	structured := &fakeRunResultTool{
		fakeTool: fakeTool{definition: model.ToolDefinition{Name: "structured"}},
		runResult: RunResult{
			Content: "partial",
			Metadata: model.ToolMetadata{
				Locations: []model.ToolLocation{{Path: "/tmp/file.txt"}},
			},
		},
		resultErr: want,
	}
	registry, err := NewRegistry(structured)
	if err != nil {
		t.Fatal(err)
	}

	got, err := registry.Run(context.Background(), model.ToolCall{Name: "structured", Arguments: `{}`})
	if !errors.Is(err, want) || got.Content != "partial" || len(got.Metadata.Locations) != 1 {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
}
