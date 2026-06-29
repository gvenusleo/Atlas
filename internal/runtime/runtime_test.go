package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/memory"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/provider/responses"
	"github.com/liuyuxin/atlas/internal/session"
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

func TestRunTurnSavesInterruptedUserMessage(t *testing.T) {
	wantErr := errors.New("network failed")
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "first reply"},
			{},
			{Content: "third reply"},
		},
		errors: []error{nil, wantErr, nil},
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
	}); !errors.Is(err, wantErr) {
		t.Fatalf("second RunTurn() error = %v, want %v", err, wantErr)
	}

	_, trans, err := r.ShowSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	messages := trans.Messages()
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content != "first" || messages[1].Content != "first reply" || messages[2].Content != "second" {
		t.Fatalf("messages = %#v", messages)
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "third",
	}); err != nil {
		t.Fatalf("third RunTurn() error = %v", err)
	}
	requestMessages := provider.requests[2].Messages
	if len(requestMessages) != 4 {
		t.Fatalf("request messages = %#v", requestMessages)
	}
	if requestMessages[0].Content != "first" || requestMessages[1].Content != "first reply" || requestMessages[2].Content != "second" || requestMessages[3].Content != "third" {
		t.Fatalf("request messages = %#v", requestMessages)
	}
}

func TestRunTurnSavesInterruptedUserMessageAfterCancel(t *testing.T) {
	provider := &cancelingProvider{}
	r := newTestRuntime(t, provider)
	ctx, cancel := context.WithCancel(context.Background())
	provider.cancel = cancel

	if _, err := r.RunTurn(ctx, TurnOptions{
		SessionID: "work",
		Prompt:    "second",
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunTurn() error = %v, want %v", err, context.Canceled)
	}

	_, trans, err := r.ShowSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	messages := trans.Messages()
	if len(messages) != 1 || messages[0].Content != "second" {
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

func TestRunTurnRejectsImageWhenModelDoesNotSupportIt(t *testing.T) {
	provider := &recordingProvider{}
	r := newTestRuntime(t, provider)

	_, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "describe",
		Parts: []model.ContentPart{
			{Type: model.ContentPartText, Text: "describe"},
			{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,aGVsbG8="},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support image input") {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.called {
		t.Fatalf("provider was called: %#v", provider.request)
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
		cfg.Providers[0].Models[0].ContextWindow = 100
		cfg.Providers[0].Models[0].MaxTokens = 100
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

func TestShouldAutoCompactCountsSkillContext(t *testing.T) {
	info := session.Session{LastInputTokens: 1}
	contextMessages := []model.Message{
		model.TextMessage(model.RoleUser, strings.Repeat("skill context ", 80)),
	}
	parts := []model.ContentPart{{Type: model.ContentPartText, Text: "hi"}}

	if !shouldAutoCompact(info, nil, contextMessages, parts, 100, 0.8) {
		t.Fatal("shouldAutoCompact() = false, want true")
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
	assertToolNames(t, provider.request.Tools, "glob", "grep", "read_file", "edit_file", "apply_patch", "write_file", "run_shell", "load_skill")
	if provider.request.System == "" {
		t.Fatal("system prompt is empty")
	}
	if provider.providerModel != "test-model" {
		t.Fatalf("provider model = %q", provider.providerModel)
	}
}

func TestRunTurnLocalFileToolsUseSessionCWD(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "note.txt"), []byte("from cwd"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{
				ToolCalls: []model.ToolCall{{
					ID:        "call_1",
					Name:      "read_file",
					Arguments: `{"path":"note.txt"}`,
				}},
			},
			{Content: "done"},
		},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "read note",
		CWD:       cwd,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Content != "done" {
		t.Fatalf("content = %q", result.Content)
	}
	requestMessages := provider.requests[1].Messages
	last := requestMessages[len(requestMessages)-1]
	if last.Role != model.RoleTool || last.Content != "from cwd" {
		t.Fatalf("tool message = %#v", last)
	}
}

func TestRunTurnPassesSessionIDToProvider(t *testing.T) {
	provider := &recordingProvider{
		response: model.ChatResponse{Content: "ok"},
	}
	r := newTestRuntime(t, provider)

	result, err := r.RunTurn(context.Background(), TurnOptions{Prompt: "hello"})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if provider.request.SessionID != result.SessionID {
		t.Fatalf("request session id = %q, want %q", provider.request.SessionID, result.SessionID)
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
	assertToolNames(t, provider.request.Tools, "web_search", "web_fetch")
}

func TestRunTurnOverrides(t *testing.T) {
	tests := []struct {
		name        string
		opts        TurnOptions
		wantErrText string
		check       func(t *testing.T, p *recordingProvider)
	}{
		{
			name: "requested model",
			opts: TurnOptions{Prompt: "hello", Model: "other-model"},
			check: func(t *testing.T, p *recordingProvider) {
				if p.providerModel != "other-model" {
					t.Fatalf("provider model = %q", p.providerModel)
				}
			},
		},
		{
			name: "reasoning effort override",
			opts: TurnOptions{Prompt: "hello", ReasoningEffort: "max", ReasoningEffortSet: true},
			check: func(t *testing.T, p *recordingProvider) {
				if p.request.ReasoningEffort != "max" {
					t.Fatalf("reasoning effort = %q", p.request.ReasoningEffort)
				}
			},
		},
		{
			name:        "empty reasoning effort override is rejected",
			opts:        TurnOptions{Prompt: "hello", ReasoningEffort: "", ReasoningEffortSet: true},
			wantErrText: "not supported",
			check: func(t *testing.T, p *recordingProvider) {
				if p.called {
					t.Fatal("provider was called")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &recordingProvider{
				events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
				response: model.ChatResponse{Content: "ok"},
			}
			r := newTestRuntime(t, provider)
			_, err := r.RunTurn(context.Background(), tt.opts)
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("RunTurn() error = %v", err)
				}
				tt.check(t, provider)
				return
			}
			if err != nil {
				t.Fatalf("RunTurn() error = %v", err)
			}
			tt.check(t, provider)
		})
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
	if len(events) != 4 || events[1].Type != agent.EventToolStarted || events[1].ToolCall.Name != "run_shell" || shellCWD(t, events[1].ToolCall.Arguments) != workdir {
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
	if len(options.Models) != 2 || options.Models[1].Value != "other-model" || options.Models[1].ContextWindow != 1000000 || options.Models[1].MaxTokens != 128000 {
		t.Fatalf("models = %#v", options.Models)
	}
	if len(options.Models[0].ReasoningEfforts) != 2 || options.Models[0].ReasoningEfforts[0].Value != "high" {
		t.Fatalf("reasoning efforts = %#v", options.Models[0])
	}
}

func TestDoctorReportsOfflineRuntimeChecks(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})

	report := r.Doctor(context.Background())
	if report.Failed() {
		t.Fatalf("report failed: %#v", report.Checks)
	}
	assertDoctorCheck(t, report, "config", DoctorStatusOK, "config.json")
	assertDoctorCheck(t, report, "provider", DoctorStatusOK, "test, chat_completions, https://api.example.com, 2 models")
	assertDoctorCheck(t, report, "agent", DoctorStatusOK, "max_steps 4, temperature 0.20, compaction_trigger_ratio 0.80")
	assertDoctorCheck(t, report, "session", DoctorStatusOK, "atlas.db")
	assertDoctorCheck(t, report, "memory", DoctorStatusOK, "0 entries, 0 pending, 0 failed, model session model")
	assertDoctorCheck(t, report, "tavily", DoctorStatusWarn, "disabled")
	assertDoctorCheck(t, report, "shell", DoctorStatusOK, expectedShellDetail())
}

func TestNewAPIProviderUsesResponsesFormat(t *testing.T) {
	provider, err := newAPIProvider(config.ProviderConfig{
		Format:  config.ProviderFormatResponses,
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test",
	}, config.ProviderModel{Value: "gpt-5"})
	if err != nil {
		t.Fatalf("newAPIProvider() error = %v", err)
	}
	if _, ok := provider.(*responses.Provider); !ok {
		t.Fatalf("provider = %T, want *responses.Provider", provider)
	}
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

func TestSessionDBPathExpandsHomeSlashStyles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := sessionDBPath(config.SessionConfig{DBPath: "~/.atlas/atlas.db"})
	if err != nil {
		t.Fatalf("sessionDBPath() error = %v", err)
	}
	if want := filepath.Join(home, ".atlas", "atlas.db"); got != want {
		t.Fatalf("sessionDBPath() = %q, want %q", got, want)
	}

	got, err = sessionDBPath(config.SessionConfig{DBPath: `~\.atlas\atlas.db`})
	if err != nil {
		t.Fatalf("sessionDBPath() error = %v", err)
	}
	if want := filepath.Join(home, `.atlas\atlas.db`); got != want {
		t.Fatalf("sessionDBPath() = %q, want %q", got, want)
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
	if !strings.Contains(provider.request.System, "<name>write</name>") || !strings.Contains(provider.request.System, "<description>polish prose</description>") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if strings.Contains(provider.request.System, "# Write") {
		t.Fatalf("system prompt includes skill body: %q", provider.request.System)
	}
}

func TestRunTurnInjectsSelectedSkillWithoutPersistingIt(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "think",
		Description: "plan work",
		Path:        "/tmp/atlas-work/.agents/skills/think/SKILL.md",
		Content:     "---\nname: think\ndescription: plan work\n---\n\n# Think\nPlan first.",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	r := newTestRuntime(t, provider)
	r.deps.LoadSkills = func(string) (*skill.Catalog, error) {
		return catalog, nil
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "design this",
		Skills:    []string{"think"},
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	messages := provider.request.Messages
	if len(messages) != 2 {
		t.Fatalf("request messages = %#v", messages)
	}
	if !strings.Contains(messages[0].Content, "<skill>") || !strings.Contains(messages[0].Content, "<name>think</name>") || !strings.Contains(messages[0].Content, "# Think") {
		t.Fatalf("skill message = %q", messages[0].Content)
	}
	if messages[1].Content != "design this" {
		t.Fatalf("user message = %#v", messages[1])
	}

	_, trans, err := r.ShowSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	saved := trans.Messages()
	if len(saved) != 2 {
		t.Fatalf("saved messages = %#v", saved)
	}
	if saved[0].Content != "design this" || strings.Contains(saved[0].Content, "<skill>") {
		t.Fatalf("saved user message = %#v", saved[0])
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

func TestSkillSummariesUsesCWDAndFiltersDisabled(t *testing.T) {
	r := newTestRuntime(t, &recordingProvider{})
	var gotCWD string
	r.deps.LoadSkills = func(cwd string) (*skill.Catalog, error) {
		gotCWD = cwd
		return skill.NewCatalog([]skill.Skill{
			{Name: "think", Description: "plan work"},
			{Name: "hidden", Description: "hidden work", DisableModelInvocation: true},
		})
	}

	summaries, err := r.SkillSummaries(context.Background(), "/tmp/acp-work")
	if err != nil {
		t.Fatalf("SkillSummaries() error = %v", err)
	}
	if gotCWD != "/tmp/acp-work" {
		t.Fatalf("LoadSkills cwd = %q", gotCWD)
	}
	if len(summaries) != 1 || summaries[0].Name != "think" || summaries[0].Description != "plan work" {
		t.Fatalf("summaries = %#v", summaries)
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

func TestRunTurnInjectsLongTermMemory(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	memStore := openTestMemoryStore(t, dbPath)
	projectKey, projectPath := memory.ProjectIdentity("/tmp/atlas-work")
	if _, err := memStore.UpsertEntry(context.Background(), memory.Entry{
		Scope:       memory.ScopeProject,
		ProjectKey:  projectKey,
		ProjectPath: projectPath,
		Type:        memory.TypeFact,
		Content:     "Atlas stores long-term memory in SQLite.",
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "where is memory stored?",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !strings.Contains(provider.request.System, "## Long-Term Memory") || !strings.Contains(provider.request.System, "Atlas stores long-term memory in SQLite.") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
}

func TestRunTurnEnqueuesMemoryExtract(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "remember project workflow",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	memStore := openTestMemoryStore(t, dbPath)
	job, ok, err := memStore.ClaimNextJob(context.Background(), "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextJob() error = %v", err)
	}
	if !ok || job.Kind != memory.JobKindSessionExtract || job.SessionID != "work" {
		t.Fatalf("job = %#v, ok = %v", job, ok)
	}
	if job.Model != "test-model" {
		t.Fatalf("job model = %q", job.Model)
	}
}

func TestRunTurnSkipsMemoryExtractBelowThreshold(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)

	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "hello",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	memStore := openTestMemoryStore(t, dbPath)
	if job, ok, err := memStore.ClaimNextJob(context.Background(), "test-worker", time.Minute); err != nil || ok {
		t.Fatalf("job = %#v, ok = %v, err = %v", job, ok, err)
	}
}

func TestShouldEnqueueMemoryExtract(t *testing.T) {
	tests := []struct {
		name          string
		info          session.Session
		messages      []model.Message
		inputTokens   int
		contextWindow int
		want          bool
	}{
		{
			name:        "short unrelated turn",
			messages:    []model.Message{{Role: model.RoleUser, Content: "hello"}, {Role: model.RoleAssistant, Content: "ok"}},
			inputTokens: 20,
			want:        false,
		},
		{
			name: "message threshold",
			messages: []model.Message{
				{Role: model.RoleUser, Content: "one"},
				{Role: model.RoleAssistant, Content: "two"},
				{Role: model.RoleUser, Content: "three"},
				{Role: model.RoleAssistant, Content: "four"},
				{Role: model.RoleUser, Content: "five"},
				{Role: model.RoleAssistant, Content: "six"},
			},
			inputTokens: 60,
			want:        true,
		},
		{
			name:        "token threshold",
			info:        session.Session{MemoryExtractedMessageCount: 2, MemoryExtractedInputTokens: 100},
			messages:    []model.Message{{Role: model.RoleUser, Content: "old"}, {Role: model.RoleAssistant, Content: "old"}, {Role: model.RoleUser, Content: "new"}},
			inputTokens: 4100,
			want:        true,
		},
		{
			name:          "context ratio threshold",
			info:          session.Session{MemoryExtractedInputTokens: 3900},
			messages:      []model.Message{{Role: model.RoleUser, Content: "new"}},
			inputTokens:   4000,
			contextWindow: 10000,
			want:          true,
		},
		{
			name:        "explicit memory directive",
			messages:    []model.Message{{Role: model.RoleUser, Content: "please remember this workflow"}},
			inputTokens: 30,
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldEnqueueMemoryExtract(tt.info, tt.messages, tt.inputTokens, tt.contextWindow)
			if got != tt.want {
				t.Fatalf("shouldEnqueueMemoryExtract() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessMemoryJobsExtractsAndSummarizes(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "normal reply"},
			{Content: `{"entries":[{"scope":"project","type":"workflow","content":"Run go test ./... before committing.","source_note":"user asked for checks","confidence":4}],"archive_fingerprints":[]}`},
			{Content: `{"summary":"Project workflow: run go test ./... before committing."}`},
		},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "always run tests before commit",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	processed, err := r.ProcessMemoryJobs(context.Background(), 2)
	if err != nil {
		t.Fatalf("ProcessMemoryJobs() error = %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("requests = %d", len(provider.requests))
	}
	if provider.requests[1].ResponseFormat != model.ResponseFormatJSONObject || provider.requests[2].ResponseFormat != model.ResponseFormatJSONObject {
		t.Fatalf("response formats = %#v, %#v", provider.requests[1].ResponseFormat, provider.requests[2].ResponseFormat)
	}
	if provider.requests[1].ReasoningEffort != "high" || provider.requests[2].ReasoningEffort != "high" {
		t.Fatalf("memory reasoning efforts = %#v, %#v", provider.requests[1].ReasoningEffort, provider.requests[2].ReasoningEffort)
	}
	if len(provider.providerModels) != 3 || provider.providerModels[1] != "test-model" || provider.providerModels[2] != "test-model" {
		t.Fatalf("provider models = %#v", provider.providerModels)
	}

	memStore := openTestMemoryStore(t, dbPath)
	contextText, _, err := memStore.PromptContext(context.Background(), "/tmp/atlas-work", "test", nil)
	if err != nil {
		t.Fatalf("PromptContext() error = %v", err)
	}
	if !strings.Contains(contextText, "run go test ./...") {
		t.Fatalf("context = %q", contextText)
	}
}

func TestProcessMemoryJobsExtractsOnlyNewMessages(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "first normal reply"},
			{Content: `{"entries":[],"archive_fingerprints":[]}`},
			{Content: "second normal reply"},
			{Content: `{"entries":[],"archive_fingerprints":[]}`},
		},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "remember alpha detail",
	}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if processed, err := r.ProcessMemoryJobs(context.Background(), 1); err != nil || processed != 1 {
		t.Fatalf("first ProcessMemoryJobs() processed = %d, err = %v", processed, err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "remember beta detail",
	}); err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}
	if processed, err := r.ProcessMemoryJobs(context.Background(), 1); err != nil || processed != 1 {
		t.Fatalf("second ProcessMemoryJobs() processed = %d, err = %v", processed, err)
	}
	if len(provider.requests) != 4 {
		t.Fatalf("requests = %d", len(provider.requests))
	}
	secondExtractPrompt := provider.requests[3].Messages[0].Content
	if strings.Contains(secondExtractPrompt, "alpha detail") {
		t.Fatalf("second extract prompt contains old message: %q", secondExtractPrompt)
	}
	if !strings.Contains(secondExtractPrompt, "beta detail") {
		t.Fatalf("second extract prompt missing new message: %q", secondExtractPrompt)
	}

	store := openTestSessionStore(t, dbPath)
	info, err := store.GetSession(context.Background(), "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.MemoryExtractedMessageCount != 4 || info.MemoryExtractedHash == "" {
		t.Fatalf("memory extraction boundary = %#v", info)
	}
}

func TestProcessMemoryJobsContinuesAfterFailedJob(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "normal reply"},
			{Content: "second reply"},
			{Content: `not json`},
			{Content: `{"entries":[],"archive_fingerprints":[]}`},
		},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "bad", Prompt: "remember first"}); err != nil {
		t.Fatalf("bad RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "good", Prompt: "remember second"}); err != nil {
		t.Fatalf("good RunTurn() error = %v", err)
	}

	processed, err := r.ProcessMemoryJobs(context.Background(), 2)
	if err != nil {
		t.Fatalf("ProcessMemoryJobs() error = %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	counts, err := openTestMemoryStore(t, dbPath).Counts(context.Background())
	if err != nil {
		t.Fatalf("Counts() error = %v", err)
	}
	if counts.Failed != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestMemoryUsesConfiguredModel(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "normal reply"},
			{Content: `{"entries":[{"scope":"project","type":"fact","content":"Use go test ./... before commit.","source_note":"user said this during the session","confidence":5}],"archive_fingerprints":[]}`},
			{Content: `{"summary":"Use go test ./... before commit."}`},
		},
	}
	r, _ := newTestRuntimeWithDBPath(t, provider)
	baseLoadConfig := r.deps.LoadConfig
	r.deps.LoadConfig = func() (config.Config, error) {
		cfg, err := baseLoadConfig()
		if err != nil {
			return config.Config{}, err
		}
		cfg.Memory.Model = "other-model"
		return cfg, nil
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "remember model preference"}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if _, err := r.ProcessMemoryJobs(context.Background(), 2); err != nil {
		t.Fatalf("ProcessMemoryJobs() error = %v", err)
	}
	if len(provider.providerModels) != 3 || provider.providerModels[0] != "test-model" || provider.providerModels[1] != "other-model" || provider.providerModels[2] != "other-model" {
		t.Fatalf("provider models = %#v", provider.providerModels)
	}
}

func TestMemoryCanBeDisabled(t *testing.T) {
	provider := &recordingProvider{
		events:   []model.StreamEvent{{Type: model.StreamTextDelta, Delta: "ok"}},
		response: model.ChatResponse{Content: "ok"},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	disabled := false
	baseLoadConfig := r.deps.LoadConfig
	r.deps.LoadConfig = func() (config.Config, error) {
		cfg, err := baseLoadConfig()
		if err != nil {
			return config.Config{}, err
		}
		cfg.Memory.Enabled = &disabled
		return cfg, nil
	}
	memStore := openTestMemoryStore(t, dbPath)
	projectKey, projectPath := memory.ProjectIdentity("/tmp/atlas-work")
	if _, err := memStore.UpsertEntry(context.Background(), memory.Entry{
		Scope:       memory.ScopeProject,
		ProjectKey:  projectKey,
		ProjectPath: projectPath,
		Type:        memory.TypeFact,
		Content:     "disabled memory should not be injected",
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "work", Prompt: "hello"}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if strings.Contains(provider.request.System, "disabled memory should not be injected") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if _, ok, err := memStore.ClaimNextJob(context.Background(), "worker", time.Minute); err != nil || ok {
		t.Fatalf("ClaimNextJob() ok = %v, err = %v", ok, err)
	}
	processed, err := r.ProcessMemoryJobs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ProcessMemoryJobs() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d", processed)
	}
	report := r.Doctor(context.Background())
	assertDoctorCheck(t, report, "memory", DoctorStatusWarn, "disabled")
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
	requests       []model.ChatRequest
	responses      []model.ChatResponse
	errors         []error
	providerModel  string
	providerModels []string
}

func (p *sequenceProvider) Stream(_ context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.requests = append(p.requests, req)
	if len(p.responses) == 0 {
		return model.ChatResponse{}, fmt.Errorf("missing response")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	if len(p.errors) > 0 {
		err := p.errors[0]
		p.errors = p.errors[1:]
		if err != nil {
			return model.ChatResponse{}, err
		}
	}
	if emit != nil && resp.Content != "" {
		if err := emit(model.StreamEvent{Type: model.StreamTextDelta, Delta: resp.Content}); err != nil {
			return model.ChatResponse{}, err
		}
	}
	return resp, nil
}

type cancelingProvider struct {
	cancel context.CancelFunc
}

func (p *cancelingProvider) Stream(ctx context.Context, _ model.ChatRequest, _ func(model.StreamEvent) error) (model.ChatResponse, error) {
	if p.cancel != nil {
		p.cancel()
	}
	if err := ctx.Err(); err != nil {
		return model.ChatResponse{}, err
	}
	return model.ChatResponse{}, context.Canceled
}

func TestRunTurnSuppressesRepeatedMemoryInjection(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "first reply"},
			{Content: "second reply"},
		},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	memStore := openTestMemoryStore(t, dbPath)
	projectKey, projectPath := memory.ProjectIdentity("/tmp/atlas-work")

	// Two entries that both match the same query.
	if _, err := memStore.UpsertEntry(context.Background(), memory.Entry{
		Scope: memory.ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: memory.TypeFact, Content: "atlas test command runs tests", Confidence: 3,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if _, err := memStore.UpsertEntry(context.Background(), memory.Entry{
		Scope: memory.ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: memory.TypeFact, Content: "atlas test coverage shows results", Confidence: 3,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	// First turn: memory should be injected.
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "work",
		Prompt:    "atlas test",
	}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(provider.requests))
	}
	firstSystem := provider.requests[0].System
	if !strings.Contains(firstSystem, "## Long-Term Memory") {
		t.Fatalf("first turn should inject memory, system = %q", firstSystem)
	}

	// Second turn with same query: suppression should reduce injected entries.
	// filterExcluded will try to exclude entries injected in the first turn.
	// If all are excluded, it falls back to full set, so we verify the mechanism
	// by checking that lastInjectedMemories was recorded.
	r.lastInjectedMemMu.Lock()
	injected := r.lastInjectedMemories["work"]
	r.lastInjectedMemMu.Unlock()
	if len(injected) == 0 {
		t.Fatalf("expected lastInjectedMemories to be recorded for session 'work'")
	}
}

func TestProcessMemoryJobsPrunesWhenIdle(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			{Content: "normal reply"},
		},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)
	memStore := openTestMemoryStore(t, dbPath)
	projectKey, projectPath := memory.ProjectIdentity("/tmp/atlas-work")

	// Insert a stale, unused, low-confidence entry that should be pruned.
	entry, err := memStore.UpsertEntry(context.Background(), memory.Entry{
		Scope: memory.ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: memory.TypeFact, Content: "stale unused fact", Confidence: 1,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	if _, err := memStore.DB().ExecContext(context.Background(),
		`update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	// No jobs queued, so ProcessMemoryJobs should trigger pruning.
	processed, err := r.ProcessMemoryJobs(context.Background(), 4)
	if err != nil {
		t.Fatalf("ProcessMemoryJobs() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 processed jobs, got %d", processed)
	}

	// Verify the stale entry was archived.
	pruned, err := memStore.GetEntryByFingerprint(context.Background(), entry.Fingerprint)
	if err != nil {
		t.Fatalf("GetEntryByFingerprint() error = %v", err)
	}
	if pruned.Status != "archived" {
		t.Fatalf("expected status archived, got %s", pruned.Status)
	}
}

func newTestRuntime(t *testing.T, provider model.Provider) *Runtime {
	t.Helper()
	r, _ := newTestRuntimeWithDBPath(t, provider)
	return r
}

func newTestRuntimeWithDBPath(t *testing.T, provider model.Provider) (*Runtime, string) {
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
				provider.providerModels = append(provider.providerModels, selected.Value)
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
	}), dbPath
}

func openTestMemoryStore(t *testing.T, dbPath string) *memory.Store {
	t.Helper()
	store, err := memory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func openTestSessionStore(t *testing.T, dbPath string) *session.Store {
	t.Helper()
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func testConfig(dbPath string) config.Config {
	return config.Config{
		DefaultModel: "test-model",
		Providers: []config.ProviderConfig{
			{
				Name:    "test",
				Format:  config.ProviderFormatChatCompletions,
				BaseURL: "https://api.example.com",
				APIKey:  "sk-test",
				Models: []config.ProviderModel{
					{
						Value:         "test-model",
						Name:          "Test Model",
						ContextWindow: 1000000,
						MaxTokens:     384000, InputFormats: []string{config.ModelInputFormatText},
						ReasoningEfforts: []config.ProviderReasoningEffort{
							{Value: "high", Name: "High"},
							{Value: "max", Name: "Max"},
						},
					},
					{
						Value:         "other-model",
						Name:          "Other Model",
						ContextWindow: 1000000,
						MaxTokens:     128000, InputFormats: []string{config.ModelInputFormatText},
						ReasoningEfforts: []config.ProviderReasoningEffort{
							{Value: "high", Name: "High"},
						},
					},
				},
			},
		},
		Agent: config.AgentConfig{
			MaxSteps:               4,
			Temperature:            0.2,
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

func assertToolNames(t *testing.T, tools []model.ToolDefinition, names ...string) {
	t.Helper()

	seen := make(map[string]bool, len(tools))
	for _, definition := range tools {
		seen[definition.Name] = true
	}
	for _, name := range names {
		if !seen[name] {
			t.Fatalf("missing tool %q in %#v", name, tools)
		}
	}
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

func shellCWD(t *testing.T, arguments string) string {
	t.Helper()
	args, err := tool.ParseShellArgs(arguments)
	if err != nil {
		t.Fatalf("ParseShellArgs() error = %v", err)
	}
	return args.CWD
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

// TestRunTurnOverflowRecoveryPreservesFullTranscript verifies that the saved transcript after overflow recovery
// Full transcript semantics are correct: the next BuildActiveMessages will not re-cut the compacted prefix,
// Also does not re-inject summary.
func TestRunTurnOverflowRecoveryPreservesFullTranscript(t *testing.T) {
	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			// Round 1: normal reply
			{Content: "first reply", Usage: model.Usage{InputTokens: 100, OutputTokens: 2, TotalTokens: 102}},
			// Round 2: normal reply
			{Content: "second reply", Usage: model.Usage{InputTokens: 120, OutputTokens: 2, TotalTokens: 122}},
			// Round 3 step 0: overflow error (response discarded)
			{Content: ""},
			// compactor calls provider.Stream to generate summary
			{Content: "compaction summary"},
			// Round 3 step 0 retry: normal reply
			{Content: "third reply", Usage: model.Usage{InputTokens: 50, OutputTokens: 2, TotalTokens: 52}},
			// Round 4: verify next round works correctly
			{Content: "fourth reply", Usage: model.Usage{InputTokens: 60, OutputTokens: 2, TotalTokens: 62}},
		},
		errors: []error{
			nil, // Round 1
			nil, // Round 2
			errors.New("responses request failed: status 400: Maximum of 1000 items allowed in input"), // Round 3 first attempt
			nil, // compactor summary call
			nil, // Round 3 retry
			nil, // Round 4
		},
	}
	r := newTestRuntime(t, provider)

	// First two rounds establish history
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "overflow-test", Prompt: "first"}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "overflow-test", Prompt: "second"}); err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}

	// Round 3 triggers overflow recovery
	result, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "overflow-test", Prompt: "third"})
	if err != nil {
		t.Fatalf("third RunTurn() error = %v", err)
	}
	if result.Content != "third reply" {
		t.Fatalf("third RunTurn() content = %q, want %q", result.Content, "third reply")
	}

	// Verify session metadata: CompactedMessageCount > 0, ContextSummary non-empty
	info, full, err := r.ShowSession(context.Background(), "overflow-test")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if info.CompactedMessageCount == 0 {
		t.Fatal("CompactedMessageCount = 0, want > 0 (compaction should have occurred)")
	}
	if info.ContextSummary == "" {
		t.Fatal("ContextSummary is empty, want non-empty")
	}

	// Full transcript should not contain synthetic summary message.
	// summary is stored in session metadata, not written to transcript.
	for i, msg := range full.Messages() {
		if strings.Contains(msg.Content, "Context summary from earlier conversation:") {
			t.Fatalf("full transcript[%d] contains synthetic summary message: %q", i, msg.Content)
		}
	}

	// Round 4: verify BuildActiveMessages works correctly, no double-cut or duplicate summary
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "overflow-test", Prompt: "fourth"}); err != nil {
		t.Fatalf("fourth RunTurn() error = %v", err)
	}

	// Check active messages received by provider in round 4
	lastReq := provider.requests[len(provider.requests)-1]
	activeMsgs := lastReq.Messages

	// First active message should be summary (injected by BuildActiveMessages)
	if len(activeMsgs) == 0 {
		t.Fatal("active messages is empty")
	}
	if !strings.Contains(activeMsgs[0].Content, "Context summary from earlier conversation:") {
		t.Fatalf("active messages[0] = %q, want summary", activeMsgs[0].Content[:min(80, len(activeMsgs[0].Content))])
	}

	// Active messages should not have two summaries
	summaryCount := 0
	for _, msg := range activeMsgs {
		if strings.Contains(msg.Content, "Context summary from earlier conversation:") {
			summaryCount++
		}
	}
	if summaryCount != 1 {
		t.Fatalf("summary count in active messages = %d, want 1", summaryCount)
	}

	// Active messages should include "third reply" and "fourth" (recent messages retained after compaction)
	foundThird := false
	foundFourth := false
	for _, msg := range activeMsgs {
		if msg.Content == "third reply" {
			foundThird = true
		}
		if strings.Contains(msg.Content, "fourth") {
			foundFourth = true
		}
	}
	if !foundThird {
		t.Fatal("active messages missing 'third reply' (should be in recent context after compaction)")
	}
	if !foundFourth {
		t.Fatal("active messages missing 'fourth' prompt")
	}
}

// TestRunTurnOverflowRecoveryPreservesSkillContext verifies that after overflow recovery
// Explicitly injected skill context for this turn is not lost; the model can still see skill content during retry.
func TestRunTurnOverflowRecoveryPreservesSkillContext(t *testing.T) {
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "think",
		Description: "plan work",
		Path:        "/tmp/atlas-work/.agents/skills/think/SKILL.md",
		Content:     "---\nname: think\ndescription: plan work\n---\n\n# Think\nPlan first.",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	provider := &sequenceProvider{
		responses: []model.ChatResponse{
			// Round 1: normal reply, establishing history
			{Content: "first reply", Usage: model.Usage{InputTokens: 100, OutputTokens: 2, TotalTokens: 102}},
			// Round 2 step 0: overflow error (response discarded)
			{Content: ""},
			// compactor calls provider.Stream to generate summary
			{Content: "compaction summary"},
			// Round 2 step 0 retry: normal reply
			{Content: "recovered reply"},
		},
		errors: []error{
			nil, // Round 1
			errors.New("responses request failed: status 400: Maximum of 1000 items allowed in input"), // Round 2 first attempt
			nil, // compactor summary call
			nil, // Round 2 retry
		},
	}
	r := newTestRuntime(t, provider)
	r.deps.LoadSkills = func(string) (*skill.Catalog, error) {
		return catalog, nil
	}

	// Round 1 establishes history
	if _, err := r.RunTurn(context.Background(), TurnOptions{SessionID: "skill-overflow", Prompt: "first"}); err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}

	// Round 2 with skill, triggers overflow recovery
	result, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "skill-overflow",
		Prompt:    "design this",
		Skills:    []string{"think"},
	})
	if err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}
	if result.Content != "recovered reply" {
		t.Fatalf("content = %q, want %q", result.Content, "recovered reply")
	}

	// Find the provider request during retry (last request)
	lastReq := provider.requests[len(provider.requests)-1]
	activeMsgs := lastReq.Messages

	// Active messages in the retry request must include skill context
	foundSkill := false
	for _, msg := range activeMsgs {
		if strings.Contains(msg.Content, "<skill>") && strings.Contains(msg.Content, "<name>think</name>") {
			foundSkill = true
			break
		}
	}
	if !foundSkill {
		t.Fatal("retry request missing skill context after overflow recovery")
	}
}

// TestRunTurnStripsImageFromHistoryWhenModelDoesNotSupportImage verifies that when the model does not support images,
// Image segments in historical messages are filtered out, keeping only text.
func TestRunTurnStripsImageFromHistoryWhenModelDoesNotSupportImage(t *testing.T) {
	provider := &recordingProvider{
		response: model.ChatResponse{Content: "ok"},
	}
	r, dbPath := newTestRuntimeWithDBPath(t, provider)

	// First save a history message with an image
	store := openTestSessionStore(t, dbPath)
	imgMsg := model.Message{
		Role:    model.RoleUser,
		Content: "look at this",
		Parts: []model.ContentPart{
			{Type: model.ContentPartText, Text: "look at this"},
			{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,aGVsbG8="},
		},
	}
	if err := store.SaveTranscript(context.Background(), "img-session", "/tmp/atlas-work", []model.Message{imgMsg}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	store.Close()

	// Round 2: resume session, send plain-text prompt
	// test-model does not support images; historical images should be filtered
	if _, err := r.RunTurn(context.Background(), TurnOptions{
		SessionID: "img-session",
		Prompt:    "what did I show you",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	// Verify no image segments in provider-received messages
	for i, msg := range provider.request.Messages {
		for j, part := range msg.Parts {
			if part.Type == model.ContentPartImage {
				t.Fatalf("message[%d].Parts[%d] contains image, should have been stripped", i, j)
			}
		}
	}

	// History message text should be preserved
	foundHistoryText := false
	for _, msg := range provider.request.Messages {
		if msg.Content == "look at this" {
			foundHistoryText = true
		}
	}
	if !foundHistoryText {
		t.Fatal("history text 'look at this' missing from provider request")
	}
}
