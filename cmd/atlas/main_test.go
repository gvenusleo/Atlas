package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	atlasruntime "github.com/liuyuxin/atlas/internal/runtime"
)

func TestRunWithDependenciesShowsInteractivePlaceholder(t *testing.T) {
	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), nil, runDependencies{stdout: &stdout}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "interactive mode is not implemented yet") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWithDependenciesShowsInteractivePlaceholderWithSession(t *testing.T) {
	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), []string{"--session", "work"}, runDependencies{stdout: &stdout}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Requested session: work") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWithDependenciesRejectsInvalidInteractiveSession(t *testing.T) {
	err := runWithDependencies(context.Background(), []string{"--session", "bad id"}, runDependencies{})
	if err == nil {
		t.Fatal("runWithDependencies() error = nil")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunWithDependenciesRequiresRunCommandForPrompt(t *testing.T) {
	err := runWithDependencies(context.Background(), []string{"hello"}, runDependencies{})
	if err == nil {
		t.Fatal("runWithDependencies() error = nil")
	}
	if !strings.Contains(err.Error(), "atlas run") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunWithDependenciesPassesDefaultSystemPrompt(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	var stdout bytes.Buffer

	err := runWithDependencies(context.Background(), []string{"run", "hello"}, runDependencies{
		runtime: testRuntime(filepath.Join(t.TempDir(), "atlas.db"), provider, []prompt.InstructionFile{
			{Path: "/tmp/atlas-work/AGENTS.md", Content: "project rules"},
		}),
		stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[session] 20260608-120000-test") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(provider.request.System, "You are Atlas") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if !strings.Contains(provider.request.System, "Working directory: /tmp/atlas-work") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if !strings.Contains(provider.request.System, "project rules") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if provider.request.Temperature != 0.2 {
		t.Fatalf("temperature = %f", provider.request.Temperature)
	}
	if len(provider.request.Tools) != 5 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
}

func TestRunWithDependenciesResumesSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	var stdout bytes.Buffer
	deps := runDependencies{
		runtime: testRuntime(dbPath, provider, nil),
		stdout:  &stdout,
	}

	if err := runWithDependencies(context.Background(), []string{"run", "--session", "work", "first"}, deps); err != nil {
		t.Fatalf("first runWithDependencies() error = %v", err)
	}
	if err := runWithDependencies(context.Background(), []string{"run", "--session", "work", "second"}, deps); err != nil {
		t.Fatalf("second runWithDependencies() error = %v", err)
	}

	messages := provider.request.Messages
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content != "first" || messages[1].Content != "ok" || messages[2].Content != "second" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestRunWithDependenciesCreatesNewSessionByDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	var stdout bytes.Buffer

	if err := runWithDependencies(context.Background(), []string{"run", "hello"}, runDependencies{
		runtime: testRuntime(dbPath, provider, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}

	if provider.request.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", provider.request.Messages)
	}
	if !strings.Contains(stdout.String(), "[session] 20260608-120000-test") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWithDependenciesListsSessions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stdout := saveTestSession(t, dbPath, "work", "hello")
	stdout.Reset()
	provider := &recordingProvider{}

	if err := runWithDependencies(context.Background(), []string{"sessions", "--limit", "5"}, runDependencies{
		runtime: testRuntime(dbPath, provider, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "ID\tUPDATED\tTITLE\tCWD") {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(got, "work") || !strings.Contains(got, "hello") {
		t.Fatalf("stdout = %q", got)
	}
	if provider.called {
		t.Fatal("newProvider called for sessions command")
	}
}

func TestRunWithDependenciesShowsSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stdout := saveTestSession(t, dbPath, "work", "hello")
	stdout.Reset()

	if err := runWithDependencies(context.Background(), []string{"session", "show", "work"}, runDependencies{
		runtime: testRuntime(dbPath, nil, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "id: work") || !strings.Contains(got, "[user] hello") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunWithDependenciesDeletesSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stdout := saveTestSession(t, dbPath, "work", "hello")
	stdout.Reset()

	if err := runWithDependencies(context.Background(), []string{"session", "delete", "work"}, runDependencies{
		runtime: testRuntime(dbPath, nil, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "deleted session work") {
		t.Fatalf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	if err := runWithDependencies(context.Background(), []string{"sessions"}, runDependencies{
		runtime: testRuntime(dbPath, nil, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "no sessions" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWithDependenciesRejectsSessionCommandUsage(t *testing.T) {
	if err := runWithDependencies(context.Background(), []string{"session"}, runDependencies{}); err == nil {
		t.Fatal("runWithDependencies() error = nil")
	}
	if err := runWithDependencies(context.Background(), []string{"sessions", "extra"}, runDependencies{}); err == nil {
		t.Fatal("runWithDependencies() error = nil")
	}
}

func TestPrintEventWritesToolStatus(t *testing.T) {
	var out bytes.Buffer
	observer := printEvent(&out)

	observer(agentEventModelDelta("hi"))
	observer(agentEventToolStarted("read_file"))
	observer(agentEventToolFinished("read_file", true))

	got := out.String()
	if !strings.Contains(got, "hi") {
		t.Fatalf("output = %q", got)
	}
	if !strings.Contains(got, "[tool] read_file") {
		t.Fatalf("output = %q", got)
	}
	if !strings.Contains(got, "[tool failed] read_file") {
		t.Fatalf("output = %q", got)
	}
}

func TestPrintEventBreaksLineBeforeToolStatus(t *testing.T) {
	var out bytes.Buffer
	observer := printEvent(&out)

	observer(agentEventModelDelta("hello"))
	observer(agentEventToolStarted("read_file"))

	if got := out.String(); got != "hello\n[tool] read_file\n" {
		t.Fatalf("output = %q", got)
	}
}

func agentEventModelDelta(content string) agent.Event {
	return agent.Event{
		Type:    agent.EventModelDelta,
		Content: content,
	}
}

func agentEventToolStarted(name string) agent.Event {
	return agent.Event{
		Type:     agent.EventToolStarted,
		ToolCall: model.ToolCall{Name: name},
	}
}

func agentEventToolFinished(name string, failed bool) agent.Event {
	return agent.Event{
		Type:      agent.EventToolFinished,
		ToolCall:  model.ToolCall{Name: name},
		ToolError: failed,
	}
}

type recordingProvider struct {
	request  model.ChatRequest
	events   []model.StreamEvent
	response model.ChatResponse
	called   bool
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

func testConfig(dbPath string) config.Config {
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
	}
}

func saveTestSession(t *testing.T, dbPath, sessionID, content string) bytes.Buffer {
	t.Helper()

	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), []string{"run", "--session", sessionID, content}, runDependencies{
		runtime: testRuntime(dbPath, &recordingProvider{
			events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
			response: model.ChatResponse{Content: "ok"},
		}, nil),
		stdout: &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	return stdout
}

func testRuntime(dbPath string, provider model.Provider, instructions []prompt.InstructionFile) *atlasruntime.Runtime {
	return atlasruntime.New(atlasruntime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return testConfig(dbPath), nil
		},
		NewProvider: func(config.ProviderConfig) (model.Provider, error) {
			if provider == nil {
				return &recordingProvider{}, nil
			}
			return provider, nil
		},
		Getwd: func() (string, error) { return "/tmp/atlas-work", nil },
		LoadInstructions: func(string) ([]prompt.InstructionFile, error) {
			return instructions, nil
		},
		NewSessionID: func(time.Time) (string, error) { return "20260608-120000-test", nil },
		Now:          func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	})
}
