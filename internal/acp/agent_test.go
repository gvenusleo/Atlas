package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	agentpkg "github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	atlasruntime "github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
	"github.com/liuyuxin/atlas/internal/version"
)

func TestInitializeReportsSupportedCapabilities(t *testing.T) {
	a := NewAgent(&fakeRuntime{})

	resp, err := a.Initialize(context.Background(), acpsdk.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if resp.ProtocolVersion != acpsdk.ProtocolVersionNumber {
		t.Fatalf("protocol version = %d", resp.ProtocolVersion)
	}
	if resp.AgentInfo == nil || resp.AgentInfo.Name != "atlas" {
		t.Fatalf("agent info = %#v", resp.AgentInfo)
	}
	if resp.AgentInfo.Version != version.Current {
		t.Fatalf("agent version = %q", resp.AgentInfo.Version)
	}
	caps := resp.AgentCapabilities.SessionCapabilities
	if caps.Close == nil || caps.Delete == nil || caps.List == nil || caps.Resume == nil {
		t.Fatalf("session capabilities = %#v", caps)
	}
	if caps.AdditionalDirectories == nil {
		t.Fatalf("additional directories capability = %#v", caps)
	}
	if !resp.AgentCapabilities.PromptCapabilities.EmbeddedContext {
		t.Fatalf("prompt capabilities = %#v", resp.AgentCapabilities.PromptCapabilities)
	}
	if !resp.AgentCapabilities.LoadSession {
		t.Fatal("LoadSession capability should be enabled")
	}
}

func TestToolKindClassifiesBuiltInTools(t *testing.T) {
	tests := map[string]acpsdk.ToolKind{
		"read_file":   acpsdk.ToolKindRead,
		"edit_file":   acpsdk.ToolKindEdit,
		"write_file":  acpsdk.ToolKindEdit,
		"list_files":  acpsdk.ToolKindSearch,
		"search_text": acpsdk.ToolKindSearch,
		"web_search":  acpsdk.ToolKindSearch,
		"web_fetch":   acpsdk.ToolKindFetch,
		"run_shell":   acpsdk.ToolKindExecute,
		"custom":      acpsdk.ToolKindOther,
	}
	for name, want := range tests {
		if got := toolKind(name); got != want {
			t.Fatalf("toolKind(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNewSessionRequiresAbsoluteCWD(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)

	if _, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: "relative"}); err == nil {
		t.Fatal("NewSession() error = nil")
	}
	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: cwd})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	state, ok := a.getSession(string(resp.SessionId))
	if !ok {
		t.Fatalf("session %q was not recorded", resp.SessionId)
	}
	if state.cwd != cwd {
		t.Fatalf("cwd = %q", state.cwd)
	}
	if state.model != "test-model" {
		t.Fatalf("model = %q", state.model)
	}
	if state.reasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", state.reasoningEffort)
	}
	if got := currentModelValue(resp.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	if got := currentReasoningEffortValue(resp.ConfigOptions); got != "high" {
		t.Fatalf("current reasoning effort = %q", got)
	}
	modelOption := resp.ConfigOptions[0].Select
	if modelOption.Id != modelSessionConfigID() || modelOption.Category == nil || *modelOption.Category != acpsdk.SessionConfigOptionCategoryModel {
		t.Fatalf("model option = %#v", modelOption)
	}
	if modelOption.Options.Ungrouped == nil || len(*modelOption.Options.Ungrouped) != 2 || (*modelOption.Options.Ungrouped)[1].Name != "Other Model" {
		t.Fatalf("model options = %#v", modelOption.Options)
	}
}

func TestNewSessionStoresAdditionalDirectories(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)
	extra := testCWD(t)

	if _, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: cwd, AdditionalDirectories: []string{"relative"}}); err == nil {
		t.Fatal("NewSession() error = nil")
	}
	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: cwd, AdditionalDirectories: []string{extra}})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	state, ok := a.getSession(string(resp.SessionId))
	if !ok || len(state.additionalDirectories) != 1 || state.additionalDirectories[0] != extra {
		t.Fatalf("session state = %#v, %t", state, ok)
	}
}

func TestNewSessionSendsCompactCommand(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)
	updates := make(chan acpsdk.SessionNotification, 3)
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates <- update
		return nil
	}

	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: cwd})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	first := receiveSessionUpdate(t, updates)
	second := receiveSessionUpdate(t, updates)
	third := receiveSessionUpdate(t, updates)
	if first.SessionId != resp.SessionId {
		t.Fatalf("session id = %#v", first)
	}
	commands := first.Update.AvailableCommandsUpdate
	if commands == nil || len(commands.AvailableCommands) != 1 || commands.AvailableCommands[0].Name != "compact" {
		t.Fatalf("commands = %#v", first.Update)
	}
	if commands.AvailableCommands[0].Input == nil || commands.AvailableCommands[0].Input.Unstructured == nil {
		t.Fatalf("command input = %#v", commands.AvailableCommands[0].Input)
	}
	if second.Update.SessionInfoUpdate == nil || second.Update.SessionInfoUpdate.UpdatedAt == nil {
		t.Fatalf("session info update = %#v", second.Update)
	}
	if third.SessionId != resp.SessionId || third.Update.AvailableCommandsUpdate == nil || len(third.Update.AvailableCommandsUpdate.AvailableCommands) != 1 || third.Update.AvailableCommandsUpdate.AvailableCommands[0].Name != "compact" {
		t.Fatalf("refreshed commands update = %#v", third)
	}
}

func receiveSessionUpdate(t *testing.T, updates <-chan acpsdk.SessionNotification) acpsdk.SessionNotification {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session update")
		return acpsdk.SessionNotification{}
	}
}

func TestPromptRunsRuntimeAndStreamsUpdates(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		opts.Observer(agentpkg.Event{Type: agentpkg.EventModelReasoningDelta, Content: "thinking"})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventModelDelta, Content: "hello"})
		opts.Observer(agentpkg.Event{
			Type: agentpkg.EventToolStarted,
			Step: 1,
			ToolCall: model.ToolCall{
				ID:        "call_1",
				Name:      "run_shell",
				Arguments: `{"command":"just check"}`,
			},
		})
		opts.Observer(agentpkg.Event{
			Type:       agentpkg.EventToolFinished,
			Step:       1,
			ToolCall:   model.ToolCall{ID: "call_1", Name: "run_shell"},
			ToolResult: "content",
		})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: "done"}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "other-model", "max", nil)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if rt.runOptions.SessionID != "sess" || rt.runOptions.Prompt != "hi" || rt.runOptions.CWD != "/tmp/work" || rt.runOptions.Model != "other-model" || rt.runOptions.ReasoningEffort != "max" || !rt.runOptions.ReasoningEffortSet {
		t.Fatalf("turn options = %#v", rt.runOptions)
	}
	if len(updates) != 4 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].Update.AgentThoughtChunk == nil || updates[0].Update.AgentThoughtChunk.Content.Text.Text != "thinking" {
		t.Fatalf("first update = %#v", updates[0].Update)
	}
	if updates[1].Update.AgentMessageChunk == nil || updates[1].Update.AgentMessageChunk.Content.Text.Text != "hello" {
		t.Fatalf("second update = %#v", updates[1].Update)
	}
	start := updates[2].Update.ToolCall
	if start == nil || start.ToolCallId != "call_1" || start.Kind != acpsdk.ToolKindExecute || start.Status != acpsdk.ToolCallStatusInProgress || start.Title != "Run: just check" {
		t.Fatalf("tool start = %#v", updates[2].Update)
	}
	if got := start.RawInput.(map[string]any)["command"]; got != "just check" {
		t.Fatalf("raw input = %#v", start.RawInput)
	}
	finish := updates[3].Update.ToolCallUpdate
	if finish == nil || finish.ToolCallId != "call_1" || finish.Status == nil || *finish.Status != acpsdk.ToolCallStatusCompleted {
		t.Fatalf("tool finish = %#v", updates[3].Update)
	}
}

func TestPromptSendsSessionInfoUsageAndMessageID(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 30, 0, 0, time.UTC)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work", Title: "hello", UpdatedAt: now},
		},
	}
	rt.run = func(_ context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		return atlasruntime.TurnResult{
			SessionID:     opts.SessionID,
			Content:       "done",
			Usage:         model.Usage{InputTokens: 12, OutputTokens: 3, TotalTokens: 15},
			ContextWindow: 100,
		}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", []string{"/tmp/extra"})
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}
	messageID := "00000000-0000-0000-0000-000000000001"

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		MessageId: &messageID,
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.UserMessageId == nil || *resp.UserMessageId != messageID {
		t.Fatalf("user message id = %#v", resp.UserMessageId)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if !rt.runOptions.AdditionalDirectoriesSet || len(rt.runOptions.AdditionalDirectories) != 1 || rt.runOptions.AdditionalDirectories[0] != "/tmp/extra" {
		t.Fatalf("turn roots = %#v", rt.runOptions)
	}
	if len(updates) != 2 {
		t.Fatalf("updates = %#v", updates)
	}
	info := updates[0].Update.SessionInfoUpdate
	if info == nil || info.Title == nil || *info.Title != "hello" || info.UpdatedAt == nil || *info.UpdatedAt != "2026-06-15T09:30:00Z" {
		t.Fatalf("session info update = %#v", updates[0].Update)
	}
	usage := updates[1].Update.UsageUpdate
	if usage == nil || usage.Used != 15 || usage.Size != 100 {
		t.Fatalf("usage update = %#v", updates[1].Update)
	}
}

func TestPromptDirectShellStreamsToolUpdates(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		opts.Observer(agentpkg.Event{
			Type: agentpkg.EventToolStarted,
			Step: 1,
			ToolCall: model.ToolCall{
				ID:        "direct_shell_1",
				Name:      "run_shell",
				Arguments: `{"command":"pwd"}`,
			},
		})
		opts.Observer(agentpkg.Event{
			Type:       agentpkg.EventToolFinished,
			Step:       1,
			ToolCall:   model.ToolCall{ID: "direct_shell_1", Name: "run_shell"},
			ToolResult: "/tmp/work\n",
		})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: "/tmp/work\n"}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("!pwd")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if rt.runOptions.Prompt != "!pwd" || rt.runOptions.CWD != "/tmp/work" {
		t.Fatalf("turn options = %#v", rt.runOptions)
	}
	if len(updates) != 2 {
		t.Fatalf("updates = %#v", updates)
	}
	start := updates[0].Update.ToolCall
	if start == nil || start.ToolCallId != "direct_shell_1" || start.Kind != acpsdk.ToolKindExecute || start.Title != "Run: pwd" {
		t.Fatalf("tool start = %#v", updates[0].Update)
	}
	finish := updates[1].Update.ToolCallUpdate
	if finish == nil || finish.ToolCallId != "direct_shell_1" || finish.Status == nil || *finish.Status != acpsdk.ToolCallStatusCompleted {
		t.Fatalf("tool finish = %#v", updates[1].Update)
	}
}

func TestPromptUsesClientTerminalForRunShellWhenSupported(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		call := model.ToolCall{
			ID:        "call_1",
			Name:      "run_shell",
			Arguments: `{"command":"pwd"}`,
		}
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolStarted, Step: 1, ToolCall: call})
		result, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{}, fmt.Errorf("fallback should not run")
		})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolFinished, Step: 1, ToolCall: call, ToolResult: result.Content, ToolMetadata: result.Metadata, ToolError: err != nil})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: result.Content}, err
	}
	a := NewAgent(rt)
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}
	client := &fakeTerminalClient{output: "terminal-output\n"}
	a.terminalClient = client

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("run pwd")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if client.create.Command != "/bin/sh" || strings.Join(client.create.Args, " ") != "-c pwd" || client.create.Cwd == nil || *client.create.Cwd != "/tmp/work" {
		t.Fatalf("create request = %#v", client.create)
	}
	if !client.waitCalled || !client.outputCalled || !client.releaseCalled {
		t.Fatalf("terminal calls wait=%v output=%v release=%v", client.waitCalled, client.outputCalled, client.releaseCalled)
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].Update.ToolCall == nil || updates[0].Update.ToolCall.Title != "Run: pwd" {
		t.Fatalf("tool start = %#v", updates[0].Update)
	}
	terminalUpdate := updates[1].Update.ToolCallUpdate
	if terminalUpdate == nil || len(terminalUpdate.Content) != 1 || terminalUpdate.Content[0].Terminal == nil || terminalUpdate.Content[0].Terminal.TerminalId != "term-1" {
		t.Fatalf("terminal update = %#v", updates[1].Update)
	}
	finish := updates[2].Update.ToolCallUpdate
	if finish == nil || finish.RawOutput != "terminal-output\n" {
		t.Fatalf("finish update = %#v", updates[2].Update)
	}
	if len(finish.Content) != 0 {
		t.Fatalf("finish should not replace terminal content: %#v", finish.Content)
	}
}

func TestPromptUsesClientTerminalForDirectShellWhenSupported(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		call := model.ToolCall{
			ID:        "direct_shell_1",
			Name:      "run_shell",
			Arguments: `{"command":"pwd"}`,
		}
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolStarted, Step: 1, ToolCall: call})
		result, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{}, fmt.Errorf("fallback should not run")
		})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolFinished, Step: 1, ToolCall: call, ToolResult: result.Content, ToolMetadata: result.Metadata, ToolError: err != nil})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: result.Content}, err
	}
	a := NewAgent(rt)
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}
	client := &fakeTerminalClient{output: "/tmp/work\n"}
	a.terminalClient = client

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("!pwd")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if client.create.Command != "/bin/sh" || strings.Join(client.create.Args, " ") != "-c pwd" {
		t.Fatalf("create request = %#v", client.create)
	}
	if len(updates) != 3 || updates[1].Update.ToolCallUpdate == nil || updates[1].Update.ToolCallUpdate.Content[0].Terminal == nil || len(updates[2].Update.ToolCallUpdate.Content) != 0 {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestPromptFallsBackWhenClientTerminalUnsupported(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		if opts.ToolRunner == nil {
			t.Fatal("ToolRunner is nil")
		}
		call := model.ToolCall{ID: "call_1", Name: "read_file", Arguments: `{"path":"README.md"}`}
		got, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{Content: "fallback-output"}, nil
		})
		if err != nil || got.Content != "fallback-output" {
			t.Fatalf("ToolRunner() = %q, %v", got.Content, err)
		}
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: got.Content}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("read")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
}

func TestPromptUsesClientFileSystemForReadFileWhenSupported(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		call := model.ToolCall{ID: "call_1", Name: "read_file", Arguments: `{"path":"README.md","offset":2,"limit":3}`}
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolStarted, Step: 1, ToolCall: call})
		result, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{}, fmt.Errorf("fallback should not run")
		})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolFinished, Step: 1, ToolCall: call, ToolResult: result.Content, ToolMetadata: result.Metadata, ToolError: err != nil})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: result.Content}, err
	}
	a := NewAgent(rt)
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true}}
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)
	client := &fakeFileClient{readContent: "line 2\nline 3\n"}
	a.fileClient = client
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("read")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if client.read.Path != "/tmp/work/README.md" || client.read.Line == nil || *client.read.Line != 2 || client.read.Limit == nil || *client.read.Limit != 3 {
		t.Fatalf("read request = %#v", client.read)
	}
	finish := updates[1].Update.ToolCallUpdate
	if finish == nil || finish.RawOutput != "line 2\nline 3\n" || len(finish.Locations) != 1 || finish.Locations[0].Path != "/tmp/work/README.md" || finish.Locations[0].Line == nil || *finish.Locations[0].Line != 2 {
		t.Fatalf("tool finish = %#v", updates[1].Update)
	}
	if len(finish.Content) != 1 || finish.Content[0].Content == nil || finish.Content[0].Content.Content.Text.Text != "line 2\nline 3\n" {
		t.Fatalf("tool content = %#v", finish.Content)
	}
}

func TestToolRunnerFallsBackWhenClientReadFileFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true}}
	a.fileClient = &fakeFileClient{readErr: errors.New("resource not found")}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "read_file",
		Arguments: `{"path":"README.md","offset":2,"limit":3}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		if !strings.Contains(call.Arguments, `"/tmp/work/README.md"`) || !strings.Contains(call.Arguments, `"offset":2`) || !strings.Contains(call.Arguments, `"limit":3`) {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "local"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "local" || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != "/tmp/work/README.md" || got.Metadata.Locations[0].Line != 2 {
		t.Fatalf("result = %#v", got)
	}
}

func TestPromptUsesClientFileSystemForWriteFileWhenSupported(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		call := model.ToolCall{ID: "call_1", Name: "write_file", Arguments: `{"path":"README.md","content":"new\n"}`}
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolStarted, Step: 1, ToolCall: call})
		result, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{}, fmt.Errorf("fallback should not run")
		})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolFinished, Step: 1, ToolCall: call, ToolResult: result.Content, ToolMetadata: result.Metadata, ToolError: err != nil})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: result.Content}, err
	}
	a := NewAgent(rt)
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.setSession("sess", "/tmp/work", "test-model", "high", nil)
	client := &fakeFileClient{readContent: "old\n"}
	a.fileClient = client
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("write")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if client.write.Path != "/tmp/work/README.md" || client.write.Content != "new\n" {
		t.Fatalf("write request = %#v", client.write)
	}
	finish := updates[1].Update.ToolCallUpdate
	if finish == nil || len(finish.Locations) != 1 || finish.Locations[0].Path != "/tmp/work/README.md" {
		t.Fatalf("tool finish = %#v", updates[1].Update)
	}
	if len(finish.Content) != 1 || finish.Content[0].Diff == nil || finish.Content[0].Diff.Path != "/tmp/work/README.md" || finish.Content[0].Diff.OldText == nil || *finish.Content[0].Diff.OldText != "old\n" || finish.Content[0].Diff.NewText != "new\n" {
		t.Fatalf("tool diff = %#v", finish.Content)
	}
}

func TestToolRunnerFallsBackWhenClientWriteFileFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.fileClient = &fakeFileClient{writeErr: errors.New("resource not found")}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "write_file",
		Arguments: `{"path":"README.md","content":"new\n"}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		if !strings.Contains(call.Arguments, `"/tmp/work/README.md"`) || !strings.Contains(call.Arguments, `"new\n"`) {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "wrote /tmp/work/README.md"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "wrote /tmp/work/README.md" || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != "/tmp/work/README.md" || got.Metadata.Diff == nil || got.Metadata.Diff.NewText != "new\n" {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerUsesClientFileSystemForEditFileWhenSupported(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	client := &fakeFileClient{readContent: "old\n"}
	a.fileClient = client
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","edits":[{"old_text":"old","new_text":"new"}]}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		return tool.RunResult{}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if fallbackCalled {
		t.Fatal("fallback should not run")
	}
	if got.Content != "replaced 1 blocks in /tmp/work/README.md" {
		t.Fatalf("content = %q", got.Content)
	}
	if client.read.Path != "/tmp/work/README.md" || client.write.Path != "/tmp/work/README.md" || client.write.Content != "new\n" {
		t.Fatalf("file client read=%#v write=%#v", client.read, client.write)
	}
	if got.Metadata.Diff == nil || got.Metadata.Diff.OldText == nil || *got.Metadata.Diff.OldText != "old\n" || got.Metadata.Diff.NewText != "new\n" {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
}

func TestToolRunnerFallsBackWhenClientEditFileReadFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.fileClient = &fakeFileClient{readErr: errors.New("resource not found")}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","edits":[{"old_text":"old","new_text":"new"}]}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		if !strings.Contains(call.Arguments, `"/tmp/work/README.md"`) || !strings.Contains(call.Arguments, `"old_text":"old"`) {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "replaced 1 blocks in /tmp/work/README.md"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "replaced 1 blocks in /tmp/work/README.md" || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != "/tmp/work/README.md" {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerFallsBackWhenClientEditFileWriteFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.fileClient = &fakeFileClient{readContent: "old\n", writeErr: errors.New("resource not found")}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","edits":[{"old_text":"old","new_text":"new"}]}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		if !strings.Contains(call.Arguments, `"/tmp/work/README.md"`) || !strings.Contains(call.Arguments, `"old_text":"old"`) {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "replaced 1 blocks in /tmp/work/README.md"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "replaced 1 blocks in /tmp/work/README.md" || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != "/tmp/work/README.md" {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerFallsBackForEmptyClientFileSystemWrite(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{WriteTextFile: true}}
	a.fileClient = &fakeFileClient{}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "write_file",
		Arguments: `{"path":"README.md","content":""}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		if !strings.Contains(call.Arguments, `"/tmp/work/README.md"`) {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "wrote /tmp/work/README.md"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "wrote /tmp/work/README.md" || got.Metadata.Diff == nil || got.Metadata.Diff.NewText != "" {
		t.Fatalf("result = %#v", got)
	}
}

func TestSearchResultLocationsUsesFileParentForSingleFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := searchResultLocations(path, "note.txt:1:needle")

	if len(got) != 1 || got[0].Path != path || got[0].Line != 1 {
		t.Fatalf("locations = %#v", got)
	}
}

func TestToolRunnerPreservesListFilesMissingPathError(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	runner := a.toolRunner("sess", "/tmp/work")

	_, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "list_files",
		Arguments: `{}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		if call.Arguments != `{}` {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{}, errors.New("list_files path is required")
	})

	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("ToolRunner() error = %v", err)
	}
}

func TestToolRunnerReturnsClientTerminalExitErrorWithoutFallback(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	exitCode := 7
	a.terminalClient = &fakeTerminalClient{output: "failed\n", exitCode: &exitCode}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "run_shell",
		Arguments: `{"command":"false"}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		return tool.RunResult{Content: "fallback"}, nil
	})

	if err == nil || !strings.Contains(err.Error(), "command exited with code 7") {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !strings.Contains(got.Content, "failed") || !strings.Contains(got.Content, "[command exited with code 7]") {
		t.Fatalf("ToolRunner() output = %q", got.Content)
	}
	if fallbackCalled {
		t.Fatal("fallback should not run after client terminal command exit")
	}
}

func TestToolRunnerFallsBackWhenClientTerminalCreateFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	a.terminalClient = &fakeTerminalClient{createErr: errors.New("terminal unavailable")}
	runner := a.toolRunner("sess", "/tmp/work")
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "run_shell",
		Arguments: `{"command":"pwd"}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		return tool.RunResult{Content: "fallback"}, nil
	})

	if err != nil || got.Content != "fallback" {
		t.Fatalf("ToolRunner() = %q, %v", got.Content, err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
}

func TestToolRunnerKillsClientTerminalOnCancel(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	client := &fakeTerminalClient{output: "partial\n"}
	a.terminalClient = client
	runner := a.toolRunner("sess", "/tmp/work")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := runner(ctx, model.ToolCall{
		ID:        "call_1",
		Name:      "run_shell",
		Arguments: `{"command":"sleep 10"}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		return tool.RunResult{}, fmt.Errorf("fallback should not run")
	})

	if err != context.Canceled {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !client.killCalled {
		t.Fatal("terminal was not killed after cancellation")
	}
	if !client.releaseCalled {
		t.Fatal("terminal was not released after cancellation")
	}
	if !strings.Contains(got.Content, "partial") || !strings.Contains(got.Content, "[command cancelled]") {
		t.Fatalf("ToolRunner() output = %q", got.Content)
	}
}

func TestToolRunnerKillsClientTerminalWhenTerminalUpdateFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Terminal: true}
	client := &fakeTerminalClient{output: "partial\n"}
	a.terminalClient = client
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		return errors.New("send failed")
	}
	runner := a.toolRunner("sess", "/tmp/work")

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "run_shell",
		Arguments: `{"command":"pwd"}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		return tool.RunResult{}, fmt.Errorf("fallback should not run")
	})

	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if got.Content != "" {
		t.Fatalf("ToolRunner() output = %q", got.Content)
	}
	if !client.killCalled {
		t.Fatal("terminal was not killed after terminal update failure")
	}
	if !client.releaseCalled {
		t.Fatal("terminal was not released after terminal update failure")
	}
}

func TestPromptCompactCommandRunsCompaction(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "other-model", "max", nil)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("/compact keep files")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if rt.runOptions.SessionID != "" {
		t.Fatalf("RunTurn should not be called: %#v", rt.runOptions)
	}
	if rt.compactOptions.SessionID != "sess" || rt.compactOptions.Model != "other-model" || rt.compactOptions.ReasoningEffort != "max" || rt.compactOptions.Instruction != "keep files" {
		t.Fatalf("compact options = %#v", rt.compactOptions)
	}
	if len(updates) != 1 || updates[0].Update.AgentMessageChunk == nil || !strings.Contains(updates[0].Update.AgentMessageChunk.Content.Text.Text, "Context compacted") {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestPromptOnlyInterceptsCompactCommand(t *testing.T) {
	tests := []struct {
		name            string
		prompt          string
		wantInstruction string
		wantCompact     bool
	}{
		{name: "compact without instruction", prompt: "/compact", wantCompact: true},
		{name: "compact with instruction", prompt: "/compact keep files", wantInstruction: "keep files", wantCompact: true},
		{name: "compact prefix is normal prompt", prompt: "/compactness matters", wantCompact: false},
		{name: "other slash prompt is normal prompt", prompt: "/help", wantCompact: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRuntime{}
			a := NewAgent(rt)
			a.setSession("sess", "/tmp/work", "test-model", "", nil)

			if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
				SessionId: "sess",
				Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(tt.prompt)},
			}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			if tt.wantCompact {
				if rt.compactOptions.SessionID != "sess" || rt.compactOptions.Instruction != tt.wantInstruction {
					t.Fatalf("compact options = %#v", rt.compactOptions)
				}
				return
			}
			if rt.runOptions.Prompt != tt.prompt {
				t.Fatalf("run options = %#v", rt.runOptions)
			}
		})
	}
}

func TestPromptResourceLinkText(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt: []acpsdk.ContentBlock{
			acpsdk.TextBlock("review"),
			acpsdk.ResourceLinkBlock("README", "file:///tmp/work/README.md"),
		},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	want := "review\n\nResource: README (file:///tmp/work/README.md)"
	if rt.runOptions.Prompt != want {
		t.Fatalf("prompt = %q", rt.runOptions.Prompt)
	}
}

func TestPromptEmbeddedTextResource(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)
	mimeType := "text/markdown"

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt: []acpsdk.ContentBlock{
			acpsdk.TextBlock("review"),
			acpsdk.ResourceBlock(acpsdk.EmbeddedResourceResource{
				TextResourceContents: &acpsdk.TextResourceContents{
					Uri:      "file:///tmp/work/README.md",
					MimeType: &mimeType,
					Text:     "# Atlas\n",
				},
			}),
		},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	want := "review\n\nResource: file:///tmp/work/README.md\nMIME: text/markdown\n\n# Atlas"
	if rt.runOptions.Prompt != want {
		t.Fatalf("prompt = %q", rt.runOptions.Prompt)
	}
}

func TestPromptUnsupportedContent(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.ImageBlock("data", "image/png")},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported ACP prompt content block") {
		t.Fatalf("Prompt() error = %v", err)
	}
}

func TestPromptReturnsCancelledWhenContextStops(t *testing.T) {
	rt := &fakeRuntime{}
	started := make(chan struct{})
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		close(started)
		<-ctx.Done()
		return atlasruntime.TurnResult{}, ctx.Err()
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	done := make(chan acpsdk.PromptResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
			SessionId: "sess",
			Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
		})
		done <- resp
		errCh <- err
	}()

	<-started
	a.Cancel(context.Background(), acpsdk.CancelNotification{SessionId: "sess"})
	resp := <-done
	if err := <-errCh; err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonCancelled {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
}

func TestResumeListCloseAndDeleteSessions(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	cwd := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd, UpdatedAt: now},
		},
		sessionsForCWD: []session.Session{
			{ID: "sess", Title: "hello", CWD: cwd, UpdatedAt: now},
		},
	}
	a := NewAgent(rt)
	updates := make(chan acpsdk.SessionNotification, 3)
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates <- update
		return nil
	}

	resume, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       cwd,
	})
	if err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
	}
	if got := currentModelValue(resume.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok || state.cwd != cwd {
		t.Fatalf("session state = %#v, %t", state, ok)
	}
	first := receiveSessionUpdate(t, updates)
	second := receiveSessionUpdate(t, updates)
	third := receiveSessionUpdate(t, updates)
	if first.Update.AvailableCommandsUpdate == nil || first.Update.AvailableCommandsUpdate.AvailableCommands[0].Name != "compact" {
		t.Fatalf("commands update = %#v", first.Update)
	}
	if second.Update.SessionInfoUpdate == nil {
		t.Fatalf("session info update = %#v", second.Update)
	}
	if third.Update.AvailableCommandsUpdate == nil || third.Update.AvailableCommandsUpdate.AvailableCommands[0].Name != "compact" {
		t.Fatalf("refreshed commands update = %#v", third.Update)
	}
	list, err := a.ListSessions(context.Background(), acpsdk.ListSessionsRequest{Cwd: &cwd})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].SessionId != "sess" || list.Sessions[0].UpdatedAt == nil {
		t.Fatalf("sessions = %#v", list.Sessions)
	}
	if !reflect.DeepEqual(rt.listedCWDs, []string{cwd}) {
		t.Fatalf("listed cwds = %#v", rt.listedCWDs)
	}

	if _, err := a.CloseSession(context.Background(), acpsdk.CloseSessionRequest{SessionId: "sess"}); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	if _, ok := a.getSession("sess"); ok {
		t.Fatal("session still active after close")
	}
	if _, err := a.UnstableDeleteSession(context.Background(), acpsdk.UnstableDeleteSessionRequest{SessionId: "sess"}); err != nil {
		t.Fatalf("UnstableDeleteSession() error = %v", err)
	}
	if !reflect.DeepEqual(rt.deleted, []string{"sess"}) {
		t.Fatalf("deleted = %#v", rt.deleted)
	}
}

func TestLoadSessionSavesAdditionalDirectories(t *testing.T) {
	cwd := testCWD(t)
	extra := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd},
		},
	}
	a := NewAgent(rt)

	if _, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:             "sess",
		Cwd:                   cwd,
		AdditionalDirectories: []string{extra},
		McpServers:            []acpsdk.McpServer{},
	}); err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if !reflect.DeepEqual(rt.savedRoots["sess"], []string{extra}) {
		t.Fatalf("saved roots = %#v", rt.savedRoots)
	}
	state, ok := a.getSession("sess")
	if !ok || len(state.additionalDirectories) != 1 || state.additionalDirectories[0] != extra {
		t.Fatalf("session state = %#v, %t", state, ok)
	}
}

func TestLoadSessionReplaysTranscript(t *testing.T) {
	cwd := testCWD(t)
	oldText := "old"
	trans := transcript.New()
	trans.Append(model.Message{Role: model.RoleUser, Content: "hi"})
	trans.Append(model.Message{
		Role:             model.RoleAssistant,
		ReasoningContent: "thinking",
		Content:          "I will check",
		ToolCalls: []model.ToolCall{{
			ID:        "call_1",
			Name:      "run_shell",
			Arguments: `{"command":"just check"}`,
		}},
	})
	trans.Append(model.Message{
		Role:       model.RoleTool,
		Content:    "ok",
		ToolCallID: "call_1",
		ToolMetadata: model.ToolMetadata{
			Locations: []model.ToolLocation{{Path: filepath.Join(cwd, "README.md"), Line: 4}},
			Diff:      &model.ToolDiff{Path: filepath.Join(cwd, "README.md"), OldText: &oldText, NewText: "new"},
		},
	})
	trans.Append(model.Message{Role: model.RoleAssistant, Content: "done"})
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd},
		},
		showTranscripts: map[string]*transcript.Transcript{
			"sess": trans,
		},
	}
	a := NewAgent(rt)
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	resp, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:  "sess",
		Cwd:        cwd,
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got := currentModelValue(resp.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok || state.cwd != cwd || state.model != "test-model" || state.reasoningEffort != "high" {
		t.Fatalf("session state = %#v, %t", state, ok)
	}
	if len(updates) != 7 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].SessionId != "sess" || updates[0].Update.UserMessageChunk == nil || updates[0].Update.UserMessageChunk.Content.Text.Text != "hi" {
		t.Fatalf("user update = %#v", updates[0])
	}
	if updates[1].Update.AgentThoughtChunk == nil || updates[1].Update.AgentThoughtChunk.Content.Text.Text != "thinking" {
		t.Fatalf("thought update = %#v", updates[1].Update)
	}
	if updates[2].Update.AgentMessageChunk == nil || updates[2].Update.AgentMessageChunk.Content.Text.Text != "I will check" {
		t.Fatalf("agent update = %#v", updates[2].Update)
	}
	start := updates[3].Update.ToolCall
	if start == nil || start.ToolCallId != "call_1" || start.Title != "Run: just check" || start.Kind != acpsdk.ToolKindExecute || start.Status != acpsdk.ToolCallStatusInProgress {
		t.Fatalf("tool start = %#v", updates[3].Update)
	}
	if got := start.RawInput.(map[string]any)["command"]; got != "just check" {
		t.Fatalf("raw input = %#v", start.RawInput)
	}
	finish := updates[4].Update.ToolCallUpdate
	if finish == nil || finish.ToolCallId != "call_1" || finish.Status == nil || *finish.Status != acpsdk.ToolCallStatusCompleted || finish.RawOutput != "ok" {
		t.Fatalf("tool finish = %#v", updates[4].Update)
	}
	if len(finish.Locations) != 1 || finish.Locations[0].Path != filepath.Join(cwd, "README.md") || finish.Locations[0].Line == nil || *finish.Locations[0].Line != 4 {
		t.Fatalf("tool locations = %#v", finish.Locations)
	}
	if len(finish.Content) != 1 || finish.Content[0].Diff == nil || finish.Content[0].Diff.NewText != "new" {
		t.Fatalf("tool content = %#v", finish.Content)
	}
	if updates[5].Update.AgentMessageChunk == nil || updates[5].Update.AgentMessageChunk.Content.Text.Text != "done" {
		t.Fatalf("final update = %#v", updates[5].Update)
	}
	if updates[6].Update.AvailableCommandsUpdate == nil || len(updates[6].Update.AvailableCommandsUpdate.AvailableCommands) != 1 || updates[6].Update.AvailableCommandsUpdate.AvailableCommands[0].Name != "compact" {
		t.Fatalf("commands update = %#v", updates[6].Update)
	}
}

func TestLoadSessionRejectsCWDMismatch(t *testing.T) {
	cwd := testCWD(t)
	otherCWD := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd},
		},
	}
	a := NewAgent(rt)

	_, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:  "sess",
		Cwd:        otherCWD,
		McpServers: []acpsdk.McpServer{},
	})
	if err == nil || !strings.Contains(err.Error(), "cwd mismatch") {
		t.Fatalf("LoadSession() error = %v", err)
	}
}

func TestLoadSessionRejectsRelativeCWD(t *testing.T) {
	a := NewAgent(&fakeRuntime{})

	_, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:  "sess",
		Cwd:        "relative",
		McpServers: []acpsdk.McpServer{},
	})
	if err == nil || !strings.Contains(err.Error(), "cwd must be absolute") {
		t.Fatalf("LoadSession() error = %v", err)
	}
}

func TestLoadSessionReturnsReplayError(t *testing.T) {
	cwd := testCWD(t)
	trans := transcript.New()
	trans.Append(model.Message{Role: model.RoleUser, Content: "hi"})
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd},
		},
		showTranscripts: map[string]*transcript.Transcript{
			"sess": trans,
		},
	}
	a := NewAgent(rt)
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		return errors.New("send failed")
	}

	_, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:  "sess",
		Cwd:        cwd,
		McpServers: []acpsdk.McpServer{},
	})
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if _, ok := a.getSession("sess"); ok {
		t.Fatal("session should not be active after replay failure")
	}
}

func TestResumeSessionRejectsCWDMismatch(t *testing.T) {
	cwd := testCWD(t)
	otherCWD := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd},
		},
	}
	a := NewAgent(rt)

	_, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       otherCWD,
	})
	if err == nil || !strings.Contains(err.Error(), "cwd mismatch") {
		t.Fatalf("ResumeSession() error = %v", err)
	}
}

func TestSetSessionConfigOptionUpdatesModel(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	resp, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
		ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: "sess",
			ConfigId:  modelSessionConfigID(),
			Value:     "other-model",
		},
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}
	if got := currentModelValue(resp.ConfigOptions); got != "other-model" {
		t.Fatalf("current model = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok {
		t.Fatal("session missing")
	}
	if state.model != "other-model" {
		t.Fatalf("session model = %q", state.model)
	}
	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if rt.runOptions.Model != "other-model" {
		t.Fatalf("turn model = %q", rt.runOptions.Model)
	}
}

func TestSetSessionConfigOptionUpdatesReasoningEffort(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	resp, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
		ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: "sess",
			ConfigId:  reasoningEffortSessionConfigID(),
			Value:     "max",
		},
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}
	if got := currentReasoningEffortValue(resp.ConfigOptions); got != "max" {
		t.Fatalf("current reasoning effort = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok {
		t.Fatal("session missing")
	}
	if state.reasoningEffort != "max" {
		t.Fatalf("session reasoning effort = %q", state.reasoningEffort)
	}
	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if rt.runOptions.ReasoningEffort != "max" {
		t.Fatalf("turn reasoning effort = %q", rt.runOptions.ReasoningEffort)
	}
}

func TestSetSessionConfigOptionRejectsInvalidReasoningEffort(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
		ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: "sess",
			ConfigId:  reasoningEffortSessionConfigID(),
			Value:     "medium",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}
}

func TestSetSessionConfigOptionRejectsInvalidModel(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
		ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: "sess",
			ConfigId:  modelSessionConfigID(),
			Value:     "missing-model",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}
}

func TestSetSessionConfigOptionRejectsUnsupportedOption(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
		ValueId: &acpsdk.SetSessionConfigOptionValueId{
			SessionId: "sess",
			ConfigId:  "mode",
			Value:     "other-model",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported session config option") {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}
}

func TestListSessionsSupportsCursor(t *testing.T) {
	rt := &fakeRuntime{
		sessionPage: session.ListPage{
			Sessions:   []session.Session{{ID: "sess", CWD: "/tmp/work", AdditionalDirectories: []string{"/tmp/extra"}}},
			NextCursor: "next-page",
		},
	}
	a := NewAgent(rt)
	cursor := "next"

	resp, err := a.ListSessions(context.Background(), acpsdk.ListSessionsRequest{Cursor: &cursor})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if rt.listedCursor != cursor {
		t.Fatalf("cursor = %q", rt.listedCursor)
	}
	if resp.NextCursor == nil || *resp.NextCursor != "next-page" {
		t.Fatalf("next cursor = %#v", resp.NextCursor)
	}
	if len(resp.Sessions) != 1 || !reflect.DeepEqual(resp.Sessions[0].AdditionalDirectories, []string{"/tmp/extra"}) {
		t.Fatalf("sessions = %#v", resp.Sessions)
	}
}

func TestUnsupportedAgentMethodsReturnMethodNotFound(t *testing.T) {
	a := NewAgent(&fakeRuntime{})

	if _, err := a.Authenticate(context.Background(), acpsdk.AuthenticateRequest{}); err == nil {
		t.Fatal("Authenticate() error = nil")
	}
	if _, err := a.Logout(context.Background(), acpsdk.LogoutRequest{}); err == nil {
		t.Fatal("Logout() error = nil")
	}
}

type fakeRuntime struct {
	run func(context.Context, atlasruntime.TurnOptions) (atlasruntime.TurnResult, error)

	runOptions      atlasruntime.TurnOptions
	compactOptions  atlasruntime.CompactOptions
	compactResult   atlasruntime.CompactResult
	listedCWDs      []string
	listedCursor    string
	savedRoots      map[string][]string
	deleted         []string
	sessions        []session.Session
	sessionsForCWD  []session.Session
	sessionPage     session.ListPage
	sessionCWDPage  session.ListPage
	showSessions    map[string]session.Session
	showTranscripts map[string]*transcript.Transcript
	showErr         error
}

type fakeTerminalClient struct {
	create        acpsdk.CreateTerminalRequest
	createErr     error
	output        string
	exitCode      *int
	waitCalled    bool
	outputCalled  bool
	killCalled    bool
	releaseCalled bool
}

type fakeFileClient struct {
	read        acpsdk.ReadTextFileRequest
	readContent string
	readErr     error
	write       acpsdk.WriteTextFileRequest
	writeErr    error
}

func (f *fakeFileClient) ReadTextFile(_ context.Context, req acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	f.read = req
	if f.readErr != nil {
		return acpsdk.ReadTextFileResponse{}, f.readErr
	}
	return acpsdk.ReadTextFileResponse{Content: f.readContent}, nil
}

func (f *fakeFileClient) WriteTextFile(_ context.Context, req acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	f.write = req
	return acpsdk.WriteTextFileResponse{}, f.writeErr
}

func (f *fakeTerminalClient) CreateTerminal(_ context.Context, req acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	f.create = req
	if f.createErr != nil {
		return acpsdk.CreateTerminalResponse{}, f.createErr
	}
	return acpsdk.CreateTerminalResponse{TerminalId: "term-1"}, nil
}

func (f *fakeTerminalClient) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	f.killCalled = true
	return acpsdk.KillTerminalResponse{}, nil
}

func (f *fakeTerminalClient) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	f.outputCalled = true
	return acpsdk.TerminalOutputResponse{Output: f.output}, nil
}

func (f *fakeTerminalClient) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	f.releaseCalled = true
	return acpsdk.ReleaseTerminalResponse{}, nil
}

func (f *fakeTerminalClient) WaitForTerminalExit(ctx context.Context, _ acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	f.waitCalled = true
	if err := ctx.Err(); err != nil {
		return acpsdk.WaitForTerminalExitResponse{}, err
	}
	code := 0
	if f.exitCode != nil {
		code = *f.exitCode
	}
	return acpsdk.WaitForTerminalExitResponse{ExitCode: &code}, nil
}

func (f *fakeRuntime) RunTurn(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
	f.runOptions = opts
	if f.run != nil {
		return f.run(ctx, opts)
	}
	return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: "ok"}, nil
}

func (f *fakeRuntime) CompactSession(_ context.Context, opts atlasruntime.CompactOptions) (atlasruntime.CompactResult, error) {
	f.compactOptions = opts
	if !f.compactResult.Compacted && f.compactResult.Reason == "" {
		return atlasruntime.CompactResult{SessionID: opts.SessionID, Compacted: true, KeepCount: 2}, nil
	}
	return f.compactResult, nil
}

func (f *fakeRuntime) ModelOptions(context.Context) (atlasruntime.ModelOptions, error) {
	return atlasruntime.ModelOptions{
		Default:         "test-model",
		ReasoningEffort: "high",
		Models: []atlasruntime.ModelOption{
			{Value: "test-model", Name: "Test Model", ContextWindow: 1000000, MaxTokens: 384000},
			{Value: "other-model", Name: "Other Model", Description: "alternate", ContextWindow: 1000000, MaxTokens: 128000},
		},
	}, nil
}

func (f *fakeRuntime) ShowSession(_ context.Context, sessionID string) (session.Session, *transcript.Transcript, error) {
	if f.showErr != nil {
		return session.Session{}, nil, f.showErr
	}
	if sess, ok := f.showSessions[sessionID]; ok {
		if trans, ok := f.showTranscripts[sessionID]; ok {
			return sess, trans, nil
		}
		return sess, transcript.New(), nil
	}
	if sessionID == f.runOptions.SessionID || sessionID == f.compactOptions.SessionID {
		return session.Session{ID: sessionID, CWD: f.runOptions.CWD}, transcript.New(), nil
	}
	return session.Session{}, nil, errors.New("session not found")
}

func (f *fakeRuntime) ListSessions(context.Context, int) ([]session.Session, error) {
	return f.sessions, nil
}

func (f *fakeRuntime) ListSessionsForCWD(_ context.Context, cwd string, _ int) ([]session.Session, error) {
	f.listedCWDs = append(f.listedCWDs, cwd)
	return f.sessionsForCWD, nil
}

func (f *fakeRuntime) ListSessionsPage(_ context.Context, cursor string, _ int) (session.ListPage, error) {
	f.listedCursor = cursor
	page := f.sessionPage
	if len(page.Sessions) == 0 {
		page.Sessions = f.sessions
	}
	return page, nil
}

func (f *fakeRuntime) ListSessionsForCWDPage(_ context.Context, cwd, cursor string, _ int) (session.ListPage, error) {
	f.listedCWDs = append(f.listedCWDs, cwd)
	f.listedCursor = cursor
	page := f.sessionCWDPage
	if len(page.Sessions) == 0 {
		page.Sessions = f.sessionsForCWD
	}
	return page, nil
}

func (f *fakeRuntime) SaveSessionRoots(_ context.Context, sessionID string, additionalDirectories []string) error {
	if f.savedRoots == nil {
		f.savedRoots = make(map[string][]string)
	}
	f.savedRoots[sessionID] = append([]string(nil), additionalDirectories...)
	return nil
}

func (f *fakeRuntime) DeleteSessionIfExists(_ context.Context, sessionID string) error {
	f.deleted = append(f.deleted, sessionID)
	return nil
}

func (f *fakeRuntime) RunMemoryWorker(context.Context) error {
	return nil
}

func currentModelValue(options []acpsdk.SessionConfigOption) string {
	return currentConfigValue(options, modelSessionConfigID())
}

func currentReasoningEffortValue(options []acpsdk.SessionConfigOption) string {
	return currentConfigValue(options, reasoningEffortSessionConfigID())
}

func currentConfigValue(options []acpsdk.SessionConfigOption, id acpsdk.SessionConfigId) string {
	for _, option := range options {
		if option.Select != nil && option.Select.Id == id {
			return string(option.Select.CurrentValue)
		}
	}
	return ""
}

func modelSessionConfigID() acpsdk.SessionConfigId {
	return acpsdk.SessionConfigId(modelConfigID)
}

func reasoningEffortSessionConfigID() acpsdk.SessionConfigId {
	return acpsdk.SessionConfigId(reasoningEffortConfigID)
}

func testCWD(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "work")
}
