package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/storage"
	"github.com/liuyuxin/atlas/internal/tool"
)

type scriptedProvider struct {
	calls int
	path  string
}

func (p *scriptedProvider) StreamChat(_ context.Context, _ model.ChatRequest) (<-chan model.StreamEvent, <-chan error) {
	events := make(chan model.StreamEvent, 4)
	errs := make(chan error, 1)
	p.calls++
	go func() {
		defer close(events)
		defer close(errs)
		if p.calls == 1 {
			events <- model.StreamEvent{ToolCall: &model.ToolCall{
				ID:        "call_1",
				Name:      "read_file",
				Arguments: `{"path":"` + p.path + `"}`,
			}}
			events <- model.StreamEvent{Done: true}
			return
		}
		events <- model.StreamEvent{TextDelta: "done"}
		events <- model.StreamEvent{Done: true}
	}()
	return events, errs
}

func TestAgentRunsToolLoopUntilFinalText(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenSQLite(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rt := tool.NewRuntime(tool.ReadFile{})
	agent := New(store, &scriptedProvider{path: readme}, rt, Config{Workdir: dir, Model: "test-model"})
	session, err := agent.CreateSession(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	events, errs := agent.RunTurn(context.Background(), session.ID, "start")
	var sawTool bool
	var sawDone bool
	for event := range events {
		if event.Type == EventToolFinished {
			sawTool = true
		}
		if event.Type == EventTurnFinished {
			sawDone = true
		}
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if !sawTool || !sawDone {
		t.Fatalf("expected tool and done events, sawTool=%v sawDone=%v", sawTool, sawDone)
	}

	messages, err := store.Messages(session.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) < 4 {
		t.Fatalf("expected persisted loop messages, got %d", len(messages))
	}
}
