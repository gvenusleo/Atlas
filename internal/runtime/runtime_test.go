package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/skill"
	"github.com/liuyuxin/atlas/internal/tool"
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

func TestRunTurnPersistsProviderUsage(t *testing.T) {
	provider := &recordingProvider{
		events: []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{
			Content: "ok",
			Usage:   model.Usage{InputTokens: 11, OutputTokens: 4, TotalTokens: 15},
		},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "hello",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	info, trans, err := r.ShowSession(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if info.LastInputTokens != 11 || info.LastOutputTokens != 4 || info.LastTotalTokens != 15 {
		t.Fatalf("session usage = %#v", info)
	}
	messages := trans.Messages()
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[1].Usage != (model.Usage{InputTokens: 11, OutputTokens: 4, TotalTokens: 15}) {
		t.Fatalf("message usage = %#v", messages[1].Usage)
	}
}

func TestCompactSessionSummarizesOldTurnsAndRunTurnUsesActiveContext(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "old one", Usage: model.Usage{InputTokens: 100, OutputTokens: 2, TotalTokens: 102}},
			{Content: "old two", Usage: model.Usage{InputTokens: 120, OutputTokens: 2, TotalTokens: 122}},
			{Content: "summary"},
			{Content: "new response", Usage: model.Usage{InputTokens: 50, OutputTokens: 2, TotalTokens: 52}},
		},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "first"}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "second"}); err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}
	result, err := r.CompactSession(context.Background(), CompactOptions{SessionID: "work", Instruction: "focus decisions"})
	if err != nil {
		t.Fatalf("CompactSession() error = %v", err)
	}
	if !result.Compacted || result.CompactCount != 2 || result.KeepCount != 2 {
		t.Fatalf("compact result = %#v", result)
	}
	info, trans, err := r.ShowSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if info.ContextSummary != "summary" || info.CompactedMessageCount != 2 {
		t.Fatalf("session info = %#v", info)
	}
	if len(trans.Messages()) != 4 {
		t.Fatalf("full transcript = %#v", trans.Messages())
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "third"}); err != nil {
		t.Fatalf("third RunTurn() error = %v", err)
	}
	if len(provider.requests) != 4 {
		t.Fatalf("requests = %#v", provider.requests)
	}
	messages := provider.requests[3].Messages
	if len(messages) != 4 {
		t.Fatalf("active messages = %#v", messages)
	}
	if !strings.Contains(messages[0].Content, "summary") || messages[1].Content != "second" || messages[3].Content != "third" {
		t.Fatalf("active messages = %#v", messages)
	}
	_, full, err := r.ShowSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if len(full.Messages()) != 6 {
		t.Fatalf("full transcript after run = %#v", full.Messages())
	}
}

func TestCompactSessionReturnsNotCompactedWithoutSafeBoundary(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{{Content: "only response"}},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "only"}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	result, err := r.CompactSession(context.Background(), CompactOptions{SessionID: "work"})
	if err != nil {
		t.Fatalf("CompactSession() error = %v", err)
	}
	if result.Compacted || result.Reason == "" {
		t.Fatalf("compact result = %#v", result)
	}
}

func TestRunTurnAutoCompactsWhenThresholdExceeded(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "old one", Usage: model.Usage{InputTokens: 80, OutputTokens: 2, TotalTokens: 82}},
			{Content: "old two", Usage: model.Usage{InputTokens: 90, OutputTokens: 2, TotalTokens: 92}},
			{Content: "auto summary"},
			{Content: "new response"},
		},
	}
	r := newTestRuntime(t, provider)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	r.deps.LoadConfig = func() (config.Config, error) {
		cfg := testConfig(dbPath)
		cfg.Provider.Models[0].ContextWindow = 100
		cfg.Provider.Models[0].MaxTokens = 100
		cfg.Agent.CompactionTriggerRatio = 0.8
		return cfg, nil
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "first"}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "second"}); err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "third"}); err != nil {
		t.Fatalf("third RunTurn() error = %v", err)
	}
	if len(provider.requests) != 4 {
		t.Fatalf("requests = %#v", provider.requests)
	}
	if !strings.Contains(provider.requests[2].Messages[0].Content, "Conversation to summarize") {
		t.Fatalf("summary request = %#v", provider.requests[2])
	}
	if !strings.Contains(provider.requests[3].Messages[0].Content, "auto summary") {
		t.Fatalf("active request = %#v", provider.requests[3].Messages)
	}
}

func TestAutoKeepRecentTokensUsesModelThresholdAsCeiling(t *testing.T) {
	if got := autoKeepRecentTokens(10000, 0.8); got != 8000 {
		t.Fatalf("autoKeepRecentTokens() = %d, want 8000", got)
	}
	if got := autoKeepRecentTokens(200000, 0.8); got != 20000 {
		t.Fatalf("autoKeepRecentTokens() = %d, want 20000", got)
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
	if provider.request.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", provider.request.ReasoningEffort)
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

func TestRunTurnRegistersTavilyToolsWhenConfigured(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	r.deps.LoadConfig = func() (config.Config, error) {
		cfg := testConfig(dbPath)
		cfg.Services.Tavily.APIKey = "tvly-test"
		cfg.Services.Tavily.BaseURL = "https://api.tavily.com"
		return cfg, nil
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{Prompt: "hello"}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(provider.request.Tools) != 9 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
	names := make(map[string]bool, len(provider.request.Tools))
	for _, definition := range provider.request.Tools {
		names[definition.Name] = true
	}
	if !names["web_search"] || !names["web_fetch"] {
		t.Fatalf("tools = %#v", provider.request.Tools)
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

func TestRunTurnUsesReasoningEffortOverride(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		Prompt:             "hello",
		ReasoningEffort:    "max",
		ReasoningEffortSet: true,
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.request.ReasoningEffort != "max" {
		t.Fatalf("reasoning effort = %q", provider.request.ReasoningEffort)
	}
}

func TestRunTurnAllowsEmptyReasoningEffortOverride(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		Prompt:             "hello",
		ReasoningEffort:    "",
		ReasoningEffortSet: true,
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.request.ReasoningEffort != "" {
		t.Fatalf("reasoning effort = %q", provider.request.ReasoningEffort)
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

func TestRunTurnDirectShellRunsCommandWithoutProvider(t *testing.T) {
	provider := &recordingProvider{}
	r := newTestRuntime(t, provider)
	workdir := t.TempDir()
	var events []agentEvent

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "! " + shellEchoCommand("direct-output"),
		CWD:       workdir,
		Observer: func(event agent.Event) {
			events = append(events, agentEvent{Type: event.Type, Content: event.Content, ToolCall: event.ToolCall, ToolResult: event.ToolResult, ToolError: event.ToolError})
		},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.called {
		t.Fatal("provider was called")
	}
	if !strings.Contains(result.Content, "direct-output") {
		t.Fatalf("content = %q", result.Content)
	}
	if len(events) != 4 || events[1].Type != agent.EventToolStarted || events[1].ToolCall.Name != "run_shell" || !strings.Contains(events[1].ToolCall.Arguments, workdir) {
		t.Fatalf("events = %#v", events)
	}
	if events[2].Type != agent.EventToolFinished || events[2].ToolError || !strings.Contains(events[2].ToolResult, "direct-output") {
		t.Fatalf("events = %#v", events)
	}
	_, trans, err := r.ShowSession(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	messages := trans.Messages()
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content == "" || messages[1].Role != model.RoleAssistant || len(messages[1].ToolCalls) != 1 || messages[2].Role != model.RoleTool {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestRunTurnDirectShellUsesToolRunner(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})
	var called bool

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "!pwd",
		CWD:       t.TempDir(),
		ToolRunner: func(ctx context.Context, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
			called = true
			if call.Name != "run_shell" || !strings.Contains(call.Arguments, "pwd") {
				t.Fatalf("call = %#v", call)
			}
			return tool.RunResult{Content: "runner-output"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !called {
		t.Fatal("ToolRunner was not called")
	}
	if result.Content != "runner-output" {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestRunTurnDirectShellKeepsFailedCommandAsTurnResult(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "! " + shellFailCommand("direct-fail", 7),
		CWD:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !strings.Contains(result.Content, "direct-fail") || !strings.Contains(result.Content, "command exited with code 7") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestRunTurnDirectShellRejectsEmptyCommand(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	_, err := r.RunTurn(context.Background(), TurnOptions{Prompt: "!"})
	if err == nil || !strings.Contains(err.Error(), "shell command is required") {
		t.Fatalf("RunTurn() error = %v", err)
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
	if options.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", options.ReasoningEffort)
	}
	if len(options.Models) != 2 || options.Models[1].Value != "other-model" || options.Models[1].ContextWindow != 1000000 || options.Models[1].MaxTokens != 128000 {
		t.Fatalf("models = %#v", options.Models)
	}
}

func TestDoctorReportsOfflineRuntimeChecks(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	report := r.Doctor(context.Background())
	if report.Failed() {
		t.Fatalf("report failed: %#v", report.Checks)
	}
	assertDoctorCheck(t, report, "config", DoctorStatusOK, "config.json")
	assertDoctorCheck(t, report, "provider", DoctorStatusOK, "https://api.example.com, default test-model, 2 models")
	assertDoctorCheck(t, report, "agent", DoctorStatusOK, "reasoning_effort high")
	assertDoctorCheck(t, report, "session", DoctorStatusOK, "atlas.db")
	assertDoctorCheck(t, report, "tavily", DoctorStatusWarn, "disabled")
	assertDoctorCheck(t, report, "shell", DoctorStatusOK, expectedShellDetail())
}

func TestDoctorReportsTavilyWhenConfigured(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	r.deps.LoadConfig = func() (config.Config, error) {
		cfg := testConfig(dbPath)
		cfg.Services.Tavily.APIKey = "tvly-test"
		cfg.Services.Tavily.BaseURL = "https://api.tavily.com"
		return cfg, nil
	}

	report := r.Doctor(context.Background())
	assertDoctorCheck(t, report, "tavily", DoctorStatusOK, "https://api.tavily.com")
}

func TestDoctorReportsConfigFailure(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})
	r.deps.ConfigPath = func() (string, error) { return "/tmp/config.json", nil }
	r.deps.LoadConfig = func() (config.Config, error) {
		return config.Config{}, fmt.Errorf("provider.api_key is required")
	}

	report := r.Doctor(context.Background())
	if !report.Failed() {
		t.Fatalf("report did not fail: %#v", report.Checks)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("checks = %#v", report.Checks)
	}
	assertDoctorCheck(t, report, "config", DoctorStatusFail, "/tmp/config.json: provider.api_key is required")
}

func TestDoctorReportsSessionFailure(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "not-a-dir", "atlas.db")
	if err := os.WriteFile(filepath.Dir(dbPath), []byte("file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	r.deps.LoadConfig = func() (config.Config, error) {
		return testConfig(dbPath), nil
	}

	report := r.Doctor(context.Background())
	if !report.Failed() {
		t.Fatalf("report did not fail: %#v", report.Checks)
	}
	assertDoctorCheck(t, report, "session", DoctorStatusFail, dbPath)
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

func TestRunTurnPersistsAdditionalDirectories(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID:                "work",
		Prompt:                   "hello",
		CWD:                      "/tmp/acp-work",
		AdditionalDirectories:    []string{"/tmp/extra"},
		AdditionalDirectoriesSet: true,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	info, _, err := r.ShowSession(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if len(info.AdditionalDirectories) != 1 || info.AdditionalDirectories[0] != "/tmp/extra" {
		t.Fatalf("additional directories = %#v", info.AdditionalDirectories)
	}
	page, err := r.ListSessionsPage(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("ListSessionsPage() error = %v", err)
	}
	if len(page.Sessions) != 1 || len(page.Sessions[0].AdditionalDirectories) != 1 || page.Sessions[0].AdditionalDirectories[0] != "/tmp/extra" {
		t.Fatalf("page = %#v", page)
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
		return model.ChatResponse{}, fmt.Errorf("missing response")
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

func newTestRuntime(t *testing.T, provider model.Provider) *Runtime {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	return New(Dependencies{
		LoadConfig: func() (config.Config, error) {
			return testConfig(dbPath), nil
		},
		ConfigPath: func() (string, error) {
			return filepath.Join(t.TempDir(), "config.json"), nil
		},
		NewProvider: func(_ config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			if provider, ok := provider.(*recordingProvider); ok {
				provider.providerModel = selected.Value
			}
			if provider, ok := provider.(*sequenceProvider); ok {
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

func assertDoctorCheck(t *testing.T, report DoctorReport, name string, status DoctorStatus, detail string) {
	t.Helper()

	for _, check := range report.Checks {
		if check.Name != name {
			continue
		}
		if check.Status != status {
			t.Fatalf("%s status = %q, want %q", name, check.Status, status)
		}
		if !strings.Contains(check.Detail, detail) {
			t.Fatalf("%s detail = %q, want substring %q", name, check.Detail, detail)
		}
		return
	}
	t.Fatalf("missing doctor check %q in %#v", name, report.Checks)
}

func expectedShellDetail() string {
	if tool.DefaultShell().DisplayName == "" {
		return "shell"
	}
	return tool.DefaultShell().DisplayName
}

type agentEvent struct {
	Type       agent.EventType
	Content    string
	ToolCall   model.ToolCall
	ToolResult string
	ToolError  bool
}

func shellEchoCommand(text string) string {
	if tool.DefaultShell().Command == "/bin/sh" {
		return "printf '%s\\n' " + quoteShell(text)
	}
	return "Write-Output " + quotePowerShell(text)
}

func shellFailCommand(text string, code int) string {
	if tool.DefaultShell().Command == "/bin/sh" {
		return "printf '%s\\n' " + quoteShell(text) + "; exit " + fmt.Sprint(code)
	}
	return "Write-Output " + quotePowerShell(text) + "; exit " + fmt.Sprint(code)
}

func quoteShell(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'"
}

func quotePowerShell(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "''") + "'"
}
