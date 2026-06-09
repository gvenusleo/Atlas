package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
)

func TestRunTurnSavesAndResumesSession(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "first",
	}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "second",
	}); err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}

	messages := provider.request.Messages
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content != "first" || messages[1].Content != "ok" || messages[2].Content != "second" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestRunTurnBuildsSystemPromptAndTools(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{Prompt: "hello"})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.SessionID != "20260608-120000-test" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if provider.request.Temperature != 0.2 {
		t.Fatalf("temperature = %f", provider.request.Temperature)
	}
	if len(provider.request.Tools) != 5 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
	if provider.request.System == "" {
		t.Fatal("system prompt is empty")
	}
}

func TestRunTurnUsesCWDOverride(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "hello",
		CWD:       "/tmp/acp-work",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !strings.Contains(provider.request.System, "Working directory: /tmp/acp-work") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	info, _, err := r.ShowSession(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if info.CWD != "/tmp/acp-work" {
		t.Fatalf("cwd = %q", info.CWD)
	}
}

func TestListSessionsForCWD(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	for _, tc := range []struct {
		id  string
		cwd string
	}{
		{id: "one", cwd: "/tmp/shared"},
		{id: "two", cwd: "/tmp/other"},
		{id: "three", cwd: "/tmp/shared"},
	} {
		if _, err := r.RunTurn(context.Background(), TurnOptions{
			SessionID: tc.id,
			Prompt:    tc.id,
			CWD:       tc.cwd,
		}); err != nil {
			t.Fatalf("RunTurn(%s) error = %v", tc.id, err)
		}
	}

	sessions, err := r.ListSessionsForCWD(context.Background(), "/tmp/shared", 10)
	if err != nil {
		t.Fatalf("ListSessionsForCWD() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v", sessions)
	}
	for _, sess := range sessions {
		if sess.CWD != "/tmp/shared" {
			t.Fatalf("session = %#v", sess)
		}
	}
}

func TestDeleteSessionIfExistsIgnoresMissingSession(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	if err := r.DeleteSessionIfExists(context.Background(), "missing"); err != nil {
		t.Fatalf("DeleteSessionIfExists() error = %v", err)
	}
}

type recordingProvider struct {
	request  model.ChatRequest
	events   []model.StreamEvent
	response model.ChatResponse
}

func (p *recordingProvider) Stream(_ context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.request = req
	for _, event := range p.events {
		if err := emit(event); err != nil {
			return model.ChatResponse{}, err
		}
	}
	return p.response, nil
}

func newTestRuntime(t *testing.T, provider model.Provider) *Runtime {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	return New(Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				Provider: config.ProviderConfig{
					BaseURL: "https://api.example.com",
					APIKey:  "sk-test",
					Model:   "test-model",
				},
				Agent: config.AgentConfig{
					MaxSteps:    4,
					Temperature: 0.2,
				},
				Session: config.SessionConfig{
					DBPath: dbPath,
				},
			}, nil
		},
		NewProvider: func(config.ProviderConfig) (model.Provider, error) {
			return provider, nil
		},
		Getwd: func() (string, error) { return "/tmp/atlas-work", nil },
		LoadInstructions: func(string) ([]prompt.InstructionFile, error) {
			return []prompt.InstructionFile{
				{Path: "/tmp/atlas-work/AGENTS.md", Content: "project rules"},
			}, nil
		},
		NewSessionID: func(time.Time) (string, error) { return "20260608-120000-test", nil },
		Now:          func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	})
}
