package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	atlasacp "github.com/liuyuxin/atlas/internal/acp"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	atlasruntime "github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/skill"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/version"
	"github.com/liuyuxin/atlas/internal/weixin"
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

func TestRunWithDependenciesShowsVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		var stdout bytes.Buffer
		if err := runWithDependencies(context.Background(), args, runDependencies{stdout: &stdout}); err != nil {
			t.Fatalf("runWithDependencies(%v) error = %v", args, err)
		}
		if got := stdout.String(); got != "atlas "+version.Current+"\n" {
			t.Fatalf("runWithDependencies(%v) stdout = %q", args, got)
		}
	}
}

func TestRunWithDependenciesRejectsVersionUsage(t *testing.T) {
	err := runWithDependencies(context.Background(), []string{"version", "extra"}, runDependencies{})
	if err == nil || !strings.Contains(err.Error(), "usage: atlas version") {
		t.Fatalf("runWithDependencies() error = %v", err)
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
	if provider.request.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", provider.request.ReasoningEffort)
	}
	if len(provider.request.Tools) != 7 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
	if provider.providerModel != "test-model" {
		t.Fatalf("provider model = %q", provider.providerModel)
	}
}

func TestRunWithDependenciesDirectShellPrintsOutput(t *testing.T) {
	provider := &recordingProvider{}
	var stdout bytes.Buffer
	cwd := t.TempDir()

	err := runWithDependencies(context.Background(), []string{"run", "! " + shellEchoCommand("cli-direct")}, runDependencies{
		runtime: testRuntimeInCWD(filepath.Join(t.TempDir(), "atlas.db"), provider, nil, cwd),
		stdout:  &stdout,
	})
	if err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if provider.called {
		t.Fatal("provider was called")
	}
	if !strings.Contains(stdout.String(), "cli-direct") || !strings.Contains(stdout.String(), "[tool] run_shell") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWithDependenciesPassesModelFlag(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	var stdout bytes.Buffer

	err := runWithDependencies(context.Background(), []string{"run", "--model", "other-model", "hello"}, runDependencies{
		runtime: testRuntime(filepath.Join(t.TempDir(), "atlas.db"), provider, nil),
		stdout:  &stdout,
	})
	if err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if provider.providerModel != "other-model" {
		t.Fatalf("provider model = %q", provider.providerModel)
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

func TestRunWithDependenciesStartsACP(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	var stdin strings.Reader
	var stdout bytes.Buffer
	rt := testRuntime(dbPath, nil, nil)
	called := false

	if err := runWithDependencies(context.Background(), []string{"acp"}, runDependencies{
		runtime: rt,
		stdin:   &stdin,
		stdout:  &stdout,
		runACP: func(_ context.Context, opts atlasacp.Options) error {
			called = true
			if opts.Runtime != rt {
				t.Fatalf("runtime = %#v", opts.Runtime)
			}
			if opts.Input != &stdin {
				t.Fatalf("input = %#v", opts.Input)
			}
			if opts.Output != &stdout {
				t.Fatalf("output = %#v", opts.Output)
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if !called {
		t.Fatal("ACP runner was not called")
	}
}

func TestRunWithDependenciesRejectsACPUsage(t *testing.T) {
	err := runWithDependencies(context.Background(), []string{"acp", "extra"}, runDependencies{})
	if err == nil || !strings.Contains(err.Error(), "usage: atlas acp") {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
}

func TestRunWithDependenciesRunsDoctor(t *testing.T) {
	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), []string{"doctor"}, runDependencies{
		runtime: testRuntime(filepath.Join(t.TempDir(), "atlas.db"), nil, nil),
		stdout:  &stdout,
	}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"OK config:",
		"OK provider: https://api.example.com, default test-model, 2 models",
		"OK session:",
		"WARN tavily: disabled",
		"OK shell: " + tool.DefaultShell().DisplayName,
		"doctor: ok",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout = %q, missing %q", got, want)
		}
	}
}

func TestRunWithDependenciesReportsDoctorFailure(t *testing.T) {
	var stdout bytes.Buffer
	rt := atlasruntime.New(atlasruntime.Dependencies{
		ConfigPath: func() (string, error) { return "/tmp/config.json", nil },
		LoadConfig: func() (config.Config, error) {
			return config.Config{}, context.Canceled
		},
	})

	err := runWithDependencies(context.Background(), []string{"doctor"}, runDependencies{
		runtime: rt,
		stdout:  &stdout,
	})
	if err == nil || !strings.Contains(err.Error(), "doctor failed") {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "FAIL config: /tmp/config.json: context canceled") || !strings.Contains(got, "doctor: failed") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunWithDependenciesRejectsDoctorUsage(t *testing.T) {
	err := runWithDependencies(context.Background(), []string{"doctor", "extra"}, runDependencies{})
	if err == nil || !strings.Contains(err.Error(), "usage: atlas doctor") {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
}

func TestRunWithDependenciesListsWeixinAccounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, err := weixin.NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.SaveAccount(weixin.Account{
		ID:        "bot-1",
		Token:     "token",
		BaseURL:   "https://weixin.example.com",
		UserID:    "user-1",
		UpdatedAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveAccount() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), []string{"weixin", "accounts"}, runDependencies{stdout: &stdout}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "bot-1") || !strings.Contains(got, "user-1") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunWithDependenciesLogsOutWeixinAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, err := weixin.NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.SaveAccount(weixin.Account{ID: "bot-1", Token: "token", BaseURL: "https://weixin.example.com", UserID: "user-1"}); err != nil {
		t.Fatalf("SaveAccount() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := runWithDependencies(context.Background(), []string{"weixin", "logout", "bot-1"}, runDependencies{stdout: &stdout}); err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "logged out weixin account bot-1") {
		t.Fatalf("stdout = %q", got)
	}
	if _, err := store.LoadAccount("bot-1"); err == nil {
		t.Fatal("LoadAccount() error = nil")
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

func TestRunWithDependenciesCompactsSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "old response"},
			{Content: "recent response"},
			{Content: "summary"},
		},
	}
	rt := testRuntime(dbPath, provider, nil)
	var stdout bytes.Buffer

	if err := runWithDependencies(context.Background(), []string{"run", "--session", "work", "old"}, runDependencies{runtime: rt, stdout: &stdout}); err != nil {
		t.Fatalf("first runWithDependencies() error = %v", err)
	}
	stdout.Reset()
	if err := runWithDependencies(context.Background(), []string{"run", "--session", "work", "recent"}, runDependencies{runtime: rt, stdout: &stdout}); err != nil {
		t.Fatalf("second runWithDependencies() error = %v", err)
	}
	stdout.Reset()
	if err := runWithDependencies(context.Background(), []string{"session", "compact", "work"}, runDependencies{runtime: rt, stdout: &stdout}); err != nil {
		t.Fatalf("compact runWithDependencies() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "compacted session work") {
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

func TestPrintEventDoesNotRepeatStreamedTurnContent(t *testing.T) {
	var out bytes.Buffer
	observer := printEvent(&out)

	observer(agentEventModelDelta("hello"))
	observer(agentEventTurnFinished("hello"))

	if got := out.String(); got != "hello\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestPrintEventWritesUnstreamedTurnContent(t *testing.T) {
	var out bytes.Buffer
	observer := printEvent(&out)

	observer(agentEventToolStarted("run_shell"))
	observer(agentEventToolFinished("run_shell", false))
	observer(agentEventTurnFinished("shell output"))

	if got := out.String(); got != "[tool] run_shell\nshell output\n" {
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

func agentEventTurnFinished(content string) agent.Event {
	return agent.Event{
		Type:    agent.EventTurnFinished,
		Content: content,
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
		if emit != nil {
			if err := emit(event); err != nil {
				return model.ChatResponse{}, err
			}
		}
	}
	return p.response, nil
}

type sequenceProvider struct {
	requests      []model.ChatRequest
	responses     []model.ChatResponse
	providerModel string
}

func (p *sequenceProvider) Stream(_ context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.requests = append(p.requests, req)
	if len(p.responses) == 0 {
		return model.ChatResponse{}, context.Canceled
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	if emit != nil && resp.Content != "" {
		if err := emit(model.StreamEvent{Type: model.StreamTextDelta, Delta: resp.Content}); err != nil {
			return model.ChatResponse{}, err
		}
	}
	return resp, nil
}

func testConfig(dbPath string) config.Config {
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
			MaxSteps:               4,
			Temperature:            0.2,
			ReasoningEffort:        "high",
			CompactionTriggerRatio: 0.8,
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
	return testRuntimeInCWD(dbPath, provider, instructions, "/tmp/atlas-work")
}

func testRuntimeInCWD(dbPath string, provider model.Provider, instructions []prompt.InstructionFile, cwd string) *atlasruntime.Runtime {
	return atlasruntime.New(atlasruntime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return testConfig(dbPath), nil
		},
		ConfigPath: func() (string, error) {
			return filepath.Join(filepath.Dir(dbPath), "config.json"), nil
		},
		NewProvider: func(_ config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			if provider == nil {
				return &recordingProvider{providerModel: selected.Value}, nil
			}
			if provider, ok := provider.(*recordingProvider); ok {
				provider.providerModel = selected.Value
			}
			if provider, ok := provider.(*sequenceProvider); ok {
				provider.providerModel = selected.Value
			}
			return provider, nil
		},
		Getwd: func() (string, error) { return cwd, nil },
		LoadInstructions: func(string) ([]prompt.InstructionFile, error) {
			return instructions, nil
		},
		LoadSkills: func(string) (*skill.Catalog, error) {
			return skill.NewCatalog(nil)
		},
		NewSessionID: func(time.Time) (string, error) { return "20260608-120000-test", nil },
		Now:          func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	})
}

func shellEchoCommand(text string) string {
	if tool.DefaultShell().Command == "/bin/sh" {
		return "printf '%s\\n' " + quoteShell(text)
	}
	return "Write-Output " + quotePowerShell(text)
}

func quoteShell(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'"
}

func quotePowerShell(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "''") + "'"
}
