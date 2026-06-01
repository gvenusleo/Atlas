package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

type captureProvider struct {
	last model.ChatRequest
}

func (p *captureProvider) StreamChat(_ context.Context, req model.ChatRequest) (<-chan model.StreamEvent, <-chan error) {
	events := make(chan model.StreamEvent, 2)
	errs := make(chan error, 1)
	p.last = req
	go func() {
		defer close(events)
		defer close(errs)
		events <- model.StreamEvent{TextDelta: "ok"}
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

func TestAgentInjectsMentionedSkillIntoRequest(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "think")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBody := "---\nname: think\ndescription: Plan before coding\n---\n\nUse a plan first.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenSQLite(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	provider := &captureProvider{}
	agent := New(store, provider, tool.NewRuntime(), Config{
		Workdir:    dir,
		Model:      "test-model",
		SkillRoots: []string{filepath.Join(dir, ".agents", "skills")},
	})
	session, err := agent.CreateSession(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	events, errs := agent.RunTurn(context.Background(), session.ID, "$think make a plan")
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(provider.last.System, "<skills_instructions>") ||
		!strings.Contains(provider.last.System, "Plan before coding") {
		t.Fatalf("request system prompt should include available skills: %q", provider.last.System)
	}
	var found bool
	for _, message := range provider.last.Messages {
		if strings.Contains(message.Content, "<skill>") && strings.Contains(message.Content, "Use a plan first.") {
			found = true
		}
	}
	if !found {
		t.Fatalf("request messages should include full skill content: %#v", provider.last.Messages)
	}
}

func TestAgentWritesDebugSessionLogWhenEnabled(t *testing.T) {
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

	debugDir := filepath.Join(dir, "debug")
	provider := &scriptedProvider{path: readme}
	agent := New(store, provider, tool.NewRuntime(tool.ReadFile{}), Config{
		Workdir:    dir,
		Model:      "test-model",
		SkillRoots: []string{filepath.Join(dir, ".agents", "skills")},
		Debug:      true,
		DebugDir:   debugDir,
	})
	session, err := agent.CreateSession(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	events, errs := agent.RunTurn(context.Background(), session.ID, "hello")
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(debugDir, session.ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var eventsSeen []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid debug entry %q: %v", line, err)
		}
		eventsSeen = append(eventsSeen, entry.Event)
	}
	for _, want := range []string{"session_created", "turn_started", "model_request", "model_tool_call", "assistant_result", "tool_started", "tool_finished", "model_text_delta", "turn_finished"} {
		if !contains(eventsSeen, want) {
			t.Fatalf("debug log missing %q in %v", want, eventsSeen)
		}
	}
}

func TestAgentDoesNotWriteDebugLogWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.OpenSQLite(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	debugDir := filepath.Join(dir, "debug")
	agent := New(store, &captureProvider{}, tool.NewRuntime(), Config{
		Workdir:    dir,
		Model:      "test-model",
		SkillRoots: []string{filepath.Join(dir, ".agents", "skills")},
		DebugDir:   debugDir,
	})
	session, err := agent.CreateSession(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	events, errs := agent.RunTurn(context.Background(), session.ID, "hello")
	for range events {
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(debugDir, session.ID+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("disabled debug mode should not create a log file, err=%v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
