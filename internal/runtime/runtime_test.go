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
	"github.com/liuyuxin/atlas/internal/skill"
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
	if provider.request.MaxTokens != 384000 {
		t.Fatalf("max tokens = %d", provider.request.MaxTokens)
	}
	if len(provider.request.Tools) != 7 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
	if provider.request.System == "" {
		t.Fatal("system prompt is empty")
	}
	if provider.providerModel != "test-model" {
		t.Fatalf("provider model = %q", provider.providerModel)
	}
}

func TestRunTurnUsesRequestedModel(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		Prompt: "hello",
		Model:  "other-model",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.providerModel != "other-model" {
		t.Fatalf("provider model = %q", provider.providerModel)
	}
}

func TestRunTurnRejectsUnknownModel(t *testing.T) {
	provider := &recordingProvider{}
	r := newTestRuntime(t, provider)

	_, err := r.RunTurn(context.Background(), TurnOptions{
		Prompt: "hello",
		Model:  "missing-model",
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.called {
		t.Fatal("provider was called")
	}
}

func TestModelOptions(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	options, err := r.ModelOptions(context.Background())
	if err != nil {
		t.Fatalf("ModelOptions() error = %v", err)
	}
	if options.Default != "test-model" {
		t.Fatalf("default = %q", options.Default)
	}
	if len(options.Models) != 2 || options.Models[1].Value != "other-model" || options.Models[1].ContextWindow != 1000000 || options.Models[1].MaxTokens != 128000 {
		t.Fatalf("models = %#v", options.Models)
	}
}

func TestRunTurnIncludesSkillSummaries(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "write",
		Description: "polish prose",
		Content:     "# Write\nfull body",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	r := newTestRuntime(t, provider)
	r.deps.LoadSkills = func(string) (*skill.Catalog, error) {
		return catalog, nil
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{Prompt: "hello"}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !strings.Contains(provider.request.System, "`write`: polish prose") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if strings.Contains(provider.request.System, "# Write") {
		t.Fatalf("system prompt includes skill body: %q", provider.request.System)
	}
}

func TestRunTurnUsesCWDForSkillLoading(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)
	var gotCWD string
	r.deps.LoadSkills = func(cwd string) (*skill.Catalog, error) {
		gotCWD = cwd
		return skill.NewCatalog(nil)
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		Prompt: "hello",
		CWD:    "/tmp/acp-work",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if gotCWD != "/tmp/acp-work" {
		t.Fatalf("LoadSkills cwd = %q", gotCWD)
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
	request       model.ChatRequest
	events        []model.StreamEvent
	response      model.ChatResponse
	providerModel string
	called        bool
}

func (p *recordingProvider) Stream(_ context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.called = true
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
					BaseURL:      "https://api.example.com",
					APIKey:       "sk-test",
					DefaultModel: "test-model",
					Models: []config.ProviderModel{
						{Value: "test-model", Name: "Test Model", ContextWindow: 1000000, MaxTokens: 384000},
						{Value: "other-model", Name: "Other Model", ContextWindow: 1000000, MaxTokens: 128000},
					},
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
		NewProvider: func(_ config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			if provider, ok := provider.(*recordingProvider); ok {
				provider.providerModel = selected.Value
			}
			return provider, nil
		},
		Getwd: func() (string, error) { return "/tmp/atlas-work", nil },
		LoadInstructions: func(string) ([]prompt.InstructionFile, error) {
			return []prompt.InstructionFile{
				{Path: "/tmp/atlas-work/AGENTS.md", Content: "project rules"},
			}, nil
		},
		LoadSkills: func(string) (*skill.Catalog, error) {
			return skill.NewCatalog(nil)
		},
		NewSessionID: func(time.Time) (string, error) { return "20260608-120000-test", nil },
		Now:          func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	})
}
