package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
	if !resp.AgentCapabilities.PromptCapabilities.Image {
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
		"glob":        acpsdk.ToolKindSearch,
		"grep":        acpsdk.ToolKindSearch,
		"apply_patch": acpsdk.ToolKindEdit,
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
	a := NewAgent(&fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
		},
	})
	cwd := testCWD(t)
	updates := make(chan acpsdk.SessionNotification, 2)
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
	if first.SessionId != resp.SessionId {
		t.Fatalf("session id = %#v", first)
	}
	commands := first.Update.AvailableCommandsUpdate
	if commands == nil || len(commands.AvailableCommands) != 2 {
		t.Fatalf("commands = %#v", first.Update)
	}
	if commands.AvailableCommands[0].Input == nil || commands.AvailableCommands[0].Input.Unstructured == nil {
		t.Fatalf("command input = %#v", commands.AvailableCommands[0].Input)
	}
	if commands.AvailableCommands[0].Name != "compact" || commands.AvailableCommands[1].Name != "think" {
		t.Fatalf("commands = %#v", commands.AvailableCommands)
	}
	if commands.AvailableCommands[1].Description != "plan work" {
		t.Fatalf("skill command = %#v", commands.AvailableCommands[1])
	}
	if second.Update.SessionInfoUpdate == nil || second.Update.SessionInfoUpdate.UpdatedAt == nil {
		t.Fatalf("session info update = %#v", second.Update)
	}
}

func TestAvailableCommandsSkipsInvalidAndReservedSkills(t *testing.T) {
	a := NewAgent(&fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "compact", Description: "reserved"},
			{Name: "bad/name", Description: "invalid"},
			{Name: "write", Description: "write prose"},
		},
	})
	a.setSession("sess", testCWD(t), "test-model", "high", nil)

	commands := a.availableCommands(context.Background(), testCWD(t))
	if got := commandNames(commands); !reflect.DeepEqual(got, []string{"compact", "write"}) {
		t.Fatalf("commands = %#v", got)
	}
}

func TestNewSessionDoesNotFailWhenPostResponseUpdateFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	called := make(chan struct{}, 3)
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		called <- struct{}{}
		return errors.New("send failed because receiver is gone")
	}

	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: testCWD(t)})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if resp.SessionId == "" {
		t.Fatalf("session id = %q", resp.SessionId)
	}
	receiveSignal(t, called)
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

func receiveSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func commandNames(commands []acpsdk.AvailableCommand) []string {
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.Name)
	}
	return names
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

func TestPromptIgnoresSessionMetadataUpdateFailure(t *testing.T) {
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work", Title: "hello", UpdatedAt: time.Now()},
		},
	}
	rt.run = func(_ context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		return atlasruntime.TurnResult{
			SessionID:     opts.SessionID,
			Content:       "done",
			Usage:         model.Usage{TotalTokens: 15},
			ContextWindow: 100,
		}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		return errors.New("send failed because receiver is gone")
	}

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn || resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Fatalf("response = %#v", resp)
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
	cwd := testCWD(t)
	a.setSession("sess", cwd, "test-model", "high", nil)
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
	if !terminalCreateMatches(client.create, "pwd", cwd) {
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
	cwd := testCWD(t)
	a.setSession("sess", cwd, "test-model", "high", nil)
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
	if !terminalCreateMatches(client.create, "pwd", cwd) {
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
		call := model.ToolCall{ID: "call_1", Name: "read_file", Arguments: `{"path":"README.md","line":2,"limit":3}`}
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolStarted, Step: 1, ToolCall: call})
		result, err := opts.ToolRunner(ctx, call, func(context.Context, model.ToolCall) (tool.RunResult, error) {
			return tool.RunResult{}, fmt.Errorf("fallback should not run")
		})
		opts.Observer(agentpkg.Event{Type: agentpkg.EventToolFinished, Step: 1, ToolCall: call, ToolResult: result.Content, ToolMetadata: result.Metadata, ToolError: err != nil})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: result.Content}, err
	}
	a := NewAgent(rt)
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true}}
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "local\n")
	a.setSession("sess", cwd, "test-model", "high", nil)
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
	if client.read.Path != path || client.read.Line == nil || *client.read.Line != 2 || client.read.Limit == nil || *client.read.Limit != 3 {
		t.Fatalf("read request = %#v", client.read)
	}
	finish := updates[1].Update.ToolCallUpdate
	if finish == nil || finish.RawOutput != "line 2\nline 3\n" || len(finish.Locations) != 1 || finish.Locations[0].Path != path || finish.Locations[0].Line == nil || *finish.Locations[0].Line != 2 {
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
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "local\n")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "read_file",
		Arguments: `{"path":"README.md","line":2,"limit":3}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		args, err := tool.ParseReadFileArgs(call.Arguments)
		if err != nil || args.Path != path || args.Line != 2 || args.Limit != 3 {
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
	if got.Content != "local" || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != path || got.Metadata.Locations[0].Line != 2 {
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
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	a.setSession("sess", cwd, "test-model", "high", nil)
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
	if client.write.Path != path || client.write.Content != "new\n" {
		t.Fatalf("write request = %#v", client.write)
	}
	finish := updates[1].Update.ToolCallUpdate
	if finish == nil || len(finish.Locations) != 1 || finish.Locations[0].Path != path {
		t.Fatalf("tool finish = %#v", updates[1].Update)
	}
	if len(finish.Content) != 1 || finish.Content[0].Diff == nil || finish.Content[0].Diff.Path != path || finish.Content[0].Diff.OldText == nil || *finish.Content[0].Diff.OldText != "old\n" || finish.Content[0].Diff.NewText != "new\n" {
		t.Fatalf("tool diff = %#v", finish.Content)
	}
}

func TestToolRunnerFallsBackWhenClientWriteFileFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.fileClient = &fakeFileClient{writeErr: errors.New("resource not found")}
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "write_file",
		Arguments: `{"path":"README.md","content":"new\n"}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		args, err := tool.ParseWriteFileArgs(call.Arguments)
		if err != nil || args.Path != path || args.Content == nil || *args.Content != "new\n" {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "wrote " + path}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "wrote "+path || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != path || got.Metadata.Diff == nil || got.Metadata.Diff.NewText != "new\n" {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerUsesClientFileSystemForEditFileWhenSupported(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	client := &fakeFileClient{readContent: "old\n"}
	a.fileClient = client
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","old_text":"old","new_text":"new"}`,
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
	if got.Content != "replaced 1 block in "+path {
		t.Fatalf("content = %q", got.Content)
	}
	if client.read.Path != path || client.write.Path != path || client.write.Content != "new\n" {
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
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","old_text":"old","new_text":"new"}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		args, err := tool.ParseEditFileArgs(call.Arguments)
		if err != nil || args.Path != path || args.OldText != "old" || args.NewText == nil || *args.NewText != "new" {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "replaced 1 block in " + path}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "replaced 1 block in "+path || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != path {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerFallsBackWhenClientEditFileWriteFails(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}}
	a.fileClient = &fakeFileClient{readContent: "old\n", writeErr: errors.New("resource not found")}
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "edit_file",
		Arguments: `{"path":"README.md","old_text":"old","new_text":"new"}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		args, err := tool.ParseEditFileArgs(call.Arguments)
		if err != nil || args.Path != path || args.OldText != "old" || args.NewText == nil || *args.NewText != "new" {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "replaced 1 block in " + path}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "replaced 1 block in "+path || len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != path {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerAddsMetadataForApplyPatch(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	patch := strings.Join([]string{
		"--- a/README.md",
		"+++ b/README.md",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "apply_patch",
		Arguments: `{"patch":` + strconv.Quote(patch) + `}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return tool.RunResult{Content: "applied patch"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if len(got.Metadata.Locations) != 1 || got.Metadata.Locations[0].Path != path {
		t.Fatalf("locations = %#v", got.Metadata.Locations)
	}
	if got.Metadata.Diff == nil || got.Metadata.Diff.OldText == nil || *got.Metadata.Diff.OldText != "old\n" || got.Metadata.Diff.NewText != "new\n" {
		t.Fatalf("diff = %#v", got.Metadata.Diff)
	}
}

func TestToolRunnerAddsDiffForApplyPatchDelete(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	writeTestTextFile(t, path, "old\n")
	runner := a.toolRunner("sess", cwd)
	patch := strings.Join([]string{
		"--- a/README.md",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-old",
		"",
	}, "\n")

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "apply_patch",
		Arguments: `{"patch":` + strconv.Quote(patch) + `}`,
	}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return tool.RunResult{Content: "applied patch"}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if got.Metadata.Diff == nil || got.Metadata.Diff.OldText == nil || *got.Metadata.Diff.OldText != "old\n" || got.Metadata.Diff.NewText != "" {
		t.Fatalf("diff = %#v", got.Metadata.Diff)
	}
}

func TestToolRunnerFallsBackForEmptyClientFileSystemWrite(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.clientCapabilities = acpsdk.ClientCapabilities{Fs: acpsdk.FileSystemCapabilities{WriteTextFile: true}}
	a.fileClient = &fakeFileClient{}
	cwd := testCWD(t)
	path := filepath.Join(cwd, "README.md")
	runner := a.toolRunner("sess", cwd)
	fallbackCalled := false

	got, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "write_file",
		Arguments: `{"path":"README.md","content":""}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		fallbackCalled = true
		args, err := tool.ParseWriteFileArgs(call.Arguments)
		if err != nil || args.Path != path {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "wrote " + path}, nil
	})

	if err != nil {
		t.Fatalf("ToolRunner() error = %v", err)
	}
	if !fallbackCalled {
		t.Fatal("fallback was not called")
	}
	if got.Content != "wrote "+path || got.Metadata.Diff == nil || got.Metadata.Diff.NewText != "" {
		t.Fatalf("result = %#v", got)
	}
}

func TestToolRunnerDoesNotAskClientFileSystemForNonTextPaths(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		path         string
		arguments    string
		capabilities acpsdk.FileSystemCapabilities
		setup        func(t *testing.T, path string)
	}{
		{
			name:         "read directory",
			toolName:     "read_file",
			path:         "docs",
			arguments:    `{"path":"docs"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true},
			setup:        mkdirTestDir,
		},
		{
			name:         "read binary",
			toolName:     "read_file",
			path:         "image.bin",
			arguments:    `{"path":"image.bin"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true},
			setup:        writeTestBinaryFile,
		},
		{
			name:         "write directory",
			toolName:     "write_file",
			path:         "docs",
			arguments:    `{"path":"docs","content":"new\n"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			setup:        mkdirTestDir,
		},
		{
			name:         "write binary",
			toolName:     "write_file",
			path:         "image.bin",
			arguments:    `{"path":"image.bin","content":"new\n"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			setup:        writeTestBinaryFile,
		},
		{
			name:         "edit directory",
			toolName:     "edit_file",
			path:         "docs",
			arguments:    `{"path":"docs","old_text":"old","new_text":"new"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			setup:        mkdirTestDir,
		},
		{
			name:         "edit binary",
			toolName:     "edit_file",
			path:         "image.bin",
			arguments:    `{"path":"image.bin","old_text":"old","new_text":"new"}`,
			capabilities: acpsdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
			setup:        writeTestBinaryFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAgent(&fakeRuntime{})
			a.clientCapabilities = acpsdk.ClientCapabilities{Fs: tt.capabilities}
			client := &fakeFileClient{readContent: "client"}
			a.fileClient = client
			cwd := testCWD(t)
			tt.setup(t, filepath.Join(cwd, tt.path))
			runner := a.toolRunner("sess", cwd)
			fallbackCalled := false

			got, err := runner(context.Background(), model.ToolCall{
				ID:        "call_1",
				Name:      tt.toolName,
				Arguments: tt.arguments,
			}, func(context.Context, model.ToolCall) (tool.RunResult, error) {
				fallbackCalled = true
				return tool.RunResult{Content: "local"}, nil
			})

			if err != nil {
				t.Fatalf("ToolRunner() error = %v", err)
			}
			if !fallbackCalled {
				t.Fatal("fallback was not called")
			}
			if client.read.Path != "" || client.write.Path != "" {
				t.Fatalf("client fs should not run: read=%#v write=%#v", client.read, client.write)
			}
			if got.Content != "local" {
				t.Fatalf("result = %#v", got)
			}
		})
	}
}

func TestSearchResultLocationsUsesFileParentForSingleFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := grepResultLocations(path, "note.txt:1:needle")

	if len(got) != 1 || got[0].Path != path || got[0].Line != 1 {
		t.Fatalf("locations = %#v", got)
	}
}

func TestToolRunnerUsesCWDForGlobWithoutPath(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cwd := testCWD(t)
	runner := a.toolRunner("sess", cwd)

	_, err := runner(context.Background(), model.ToolCall{
		ID:        "call_1",
		Name:      "glob",
		Arguments: `{"pattern":"*.go"}`,
	}, func(_ context.Context, call model.ToolCall) (tool.RunResult, error) {
		args, err := tool.ParseGlobArgs(call.Arguments)
		if err != nil || args.Path != cwd || args.Pattern != "*.go" {
			t.Fatalf("arguments = %s", call.Arguments)
		}
		return tool.RunResult{Content: "main.go"}, nil
	})

	if err != nil {
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

func TestPromptSkillCommandRunsTurnWithSelectedSkill(t *testing.T) {
	rt := &fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
		},
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "other-model", "max", nil)

	resp, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt: []acpsdk.ContentBlock{
			acpsdk.TextBlock("/think design this"),
			acpsdk.TextBlock("extra context"),
		},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if rt.compactOptions.SessionID != "" {
		t.Fatalf("compact should not run: %#v", rt.compactOptions)
	}
	if rt.runOptions.Prompt != "/think design this\n\nextra context" {
		t.Fatalf("prompt = %q", rt.runOptions.Prompt)
	}
	if !reflect.DeepEqual(rt.runOptions.Skills, []string{"think"}) {
		t.Fatalf("skills = %#v", rt.runOptions.Skills)
	}
	if len(rt.runOptions.Parts) != 2 || model.TextFromParts(rt.runOptions.Parts) != rt.runOptions.Prompt {
		t.Fatalf("parts = %#v, prompt = %q", rt.runOptions.Parts, rt.runOptions.Prompt)
	}
}

func TestPromptSkillCommandMidMessage(t *testing.T) {
	rt := &fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
		},
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("帮我用 /think 分析一下这个设计")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if !reflect.DeepEqual(rt.runOptions.Skills, []string{"think"}) {
		t.Fatalf("skills = %#v, want [think]", rt.runOptions.Skills)
	}
}

func TestPromptSkillCommandMultipleSkills(t *testing.T) {
	rt := &fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
			{Name: "write", Description: "write docs"},
		},
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("/think 然后 /write 文档")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if !reflect.DeepEqual(rt.runOptions.Skills, []string{"think", "write"}) {
		t.Fatalf("skills = %#v, want [think write]", rt.runOptions.Skills)
	}
}

func TestPromptSkillCommandDeduplicates(t *testing.T) {
	rt := &fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
		},
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	_, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("/think /think /think")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if !reflect.DeepEqual(rt.runOptions.Skills, []string{"think"}) {
		t.Fatalf("skills = %#v, want [think]", rt.runOptions.Skills)
	}
}

func TestPromptSkillCommandNoSpacePrefix(t *testing.T) {
	rt := &fakeRuntime{
		skillSummaries: []atlasruntime.SkillSummary{
			{Name: "think", Description: "plan work"},
		},
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	// "/think" attached to preceding text without space should not match
	_, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("帮我用/think分析")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if len(rt.runOptions.Skills) != 0 {
		t.Fatalf("skills = %#v, want empty", rt.runOptions.Skills)
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

func TestPromptAcceptsImageContent(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "", nil)

	if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
		SessionId: "sess",
		Prompt: []acpsdk.ContentBlock{
			acpsdk.TextBlock("describe"),
			acpsdk.ImageBlock("aGVsbG8=", "image/png"),
		},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if rt.runOptions.Prompt != "describe" {
		t.Fatalf("prompt = %q", rt.runOptions.Prompt)
	}
	if len(rt.runOptions.Parts) != 2 || rt.runOptions.Parts[1].Type != model.ContentPartImage || rt.runOptions.Parts[1].DataURL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("parts = %#v", rt.runOptions.Parts)
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
			"sess": {ID: "sess", CWD: cwd, UpdatedAt: now, LastTotalTokens: 42},
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
	if third.Update.UsageUpdate == nil {
		t.Fatalf("usage update = %#v", third.Update)
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

func TestResumeSessionDoesNotFailWhenPostResponseUpdateFails(t *testing.T) {
	cwd := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd, UpdatedAt: time.Now(), LastTotalTokens: 42},
		},
	}
	a := NewAgent(rt)
	called := make(chan struct{}, 3)
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		called <- struct{}{}
		return errors.New("send failed because receiver is gone")
	}

	resp, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       cwd,
	})
	if err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
	}
	if got := currentModelValue(resp.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	receiveSignal(t, called)
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
	updates := make(chan acpsdk.SessionNotification, 7)
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates <- update
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
	first := receiveSessionUpdate(t, updates)
	second := receiveSessionUpdate(t, updates)
	third := receiveSessionUpdate(t, updates)
	fourth := receiveSessionUpdate(t, updates)
	fifth := receiveSessionUpdate(t, updates)
	sixth := receiveSessionUpdate(t, updates)
	seventh := receiveSessionUpdate(t, updates)
	if first.SessionId != "sess" || first.Update.UserMessageChunk == nil || first.Update.UserMessageChunk.Content.Text.Text != "hi" {
		t.Fatalf("user update = %#v", first)
	}
	if second.Update.AgentThoughtChunk == nil || second.Update.AgentThoughtChunk.Content.Text.Text != "thinking" {
		t.Fatalf("thought update = %#v", second.Update)
	}
	if third.Update.AgentMessageChunk == nil || third.Update.AgentMessageChunk.Content.Text.Text != "I will check" {
		t.Fatalf("agent update = %#v", third.Update)
	}
	start := fourth.Update.ToolCall
	if start == nil || start.ToolCallId != "call_1" || start.Title != "Run: just check" || start.Kind != acpsdk.ToolKindExecute || start.Status != acpsdk.ToolCallStatusInProgress {
		t.Fatalf("tool start = %#v", fourth.Update)
	}
	if got := start.RawInput.(map[string]any)["command"]; got != "just check" {
		t.Fatalf("raw input = %#v", start.RawInput)
	}
	finish := fifth.Update.ToolCallUpdate
	if finish == nil || finish.ToolCallId != "call_1" || finish.Status == nil || *finish.Status != acpsdk.ToolCallStatusCompleted || finish.RawOutput != "ok" {
		t.Fatalf("tool finish = %#v", fifth.Update)
	}
	if len(finish.Locations) != 1 || finish.Locations[0].Path != filepath.Join(cwd, "README.md") || finish.Locations[0].Line == nil || *finish.Locations[0].Line != 4 {
		t.Fatalf("tool locations = %#v", finish.Locations)
	}
	if len(finish.Content) != 1 || finish.Content[0].Diff == nil || finish.Content[0].Diff.NewText != "new" {
		t.Fatalf("tool content = %#v", finish.Content)
	}
	if sixth.Update.AgentMessageChunk == nil || sixth.Update.AgentMessageChunk.Content.Text.Text != "done" {
		t.Fatalf("final update = %#v", sixth.Update)
	}
	if seventh.Update.AvailableCommandsUpdate == nil || len(seventh.Update.AvailableCommandsUpdate.AvailableCommands) != 1 || seventh.Update.AvailableCommandsUpdate.AvailableCommands[0].Name != "compact" {
		t.Fatalf("commands update = %#v", seventh.Update)
	}
}

func TestLoadSessionDoesNotFailWhenMetadataUpdateFails(t *testing.T) {
	cwd := testCWD(t)
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: cwd, UpdatedAt: time.Now(), LastTotalTokens: 42},
		},
		showTranscripts: map[string]*transcript.Transcript{
			"sess": transcript.New(),
		},
	}
	a := NewAgent(rt)
	called := make(chan struct{}, 3)
	a.sendUpdate = func(context.Context, acpsdk.SessionNotification) error {
		called <- struct{}{}
		return errors.New("send failed because receiver is gone")
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
	receiveSignal(t, called)
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

func TestSetSessionConfigOptionUpdatesConfig(t *testing.T) {
	tests := []struct {
		name       string
		configID   acpsdk.SessionConfigId
		value      string
		checkState func(t *testing.T, state *sessionState)
		checkRun   func(t *testing.T, opts atlasruntime.TurnOptions)
	}{
		{
			name:     "model",
			configID: modelSessionConfigID(),
			value:    "other-model",
			checkState: func(t *testing.T, state *sessionState) {
				if state.model != "other-model" {
					t.Fatalf("session model = %q", state.model)
				}
				if state.reasoningEffort != "high" {
					t.Fatalf("session reasoning effort = %q", state.reasoningEffort)
				}
			},
			checkRun: func(t *testing.T, opts atlasruntime.TurnOptions) {
				if opts.Model != "other-model" {
					t.Fatalf("turn model = %q", opts.Model)
				}
				if opts.ReasoningEffort != "high" {
					t.Fatalf("turn reasoning effort = %q", opts.ReasoningEffort)
				}
			},
		},
		{
			name:     "reasoning_effort",
			configID: reasoningEffortSessionConfigID(),
			value:    "max",
			checkState: func(t *testing.T, state *sessionState) {
				if state.reasoningEffort != "max" {
					t.Fatalf("session reasoning effort = %q", state.reasoningEffort)
				}
			},
			checkRun: func(t *testing.T, opts atlasruntime.TurnOptions) {
				if opts.ReasoningEffort != "max" {
					t.Fatalf("turn reasoning effort = %q", opts.ReasoningEffort)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRuntime{}
			a := NewAgent(rt)
			a.setSession("sess", "/tmp/work", "test-model", "max", nil)

			resp, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
				ValueId: &acpsdk.SetSessionConfigOptionValueId{
					SessionId: "sess",
					ConfigId:  tt.configID,
					Value:     acpsdk.SessionConfigValueId(tt.value),
				},
			})
			if err != nil {
				t.Fatalf("SetSessionConfigOption() error = %v", err)
			}
			if got := currentConfigValue(resp.ConfigOptions, tt.configID); got != tt.value {
				t.Fatalf("current config value = %q, want %q", got, tt.value)
			}
			if tt.configID == modelSessionConfigID() {
				if got := currentReasoningEffortValue(resp.ConfigOptions); got != "high" {
					t.Fatalf("current reasoning effort = %q", got)
				}
				assertReasoningEffortOptions(t, resp.ConfigOptions, []string{"high"})
			}
			state, ok := a.getSession("sess")
			if !ok {
				t.Fatal("session missing")
			}
			tt.checkState(t, state)
			if _, err := a.Prompt(context.Background(), acpsdk.PromptRequest{
				SessionId: "sess",
				Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock("hi")},
			}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			tt.checkRun(t, rt.runOptions)
		})
	}
}

func TestSetSessionConfigOptionRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		configID acpsdk.SessionConfigId
		value    string
		wantErr  string
	}{
		{name: "invalid reasoning effort", configID: reasoningEffortSessionConfigID(), value: "medium", wantErr: "not supported"},
		{name: "invalid model", configID: modelSessionConfigID(), value: "missing-model", wantErr: "not configured"},
		{name: "unsupported option", configID: "mode", value: "other-model", wantErr: "unsupported session config option"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAgent(&fakeRuntime{})
			a.setSession("sess", "/tmp/work", "test-model", "", nil)

			_, err := a.SetSessionConfigOption(context.Background(), acpsdk.SetSessionConfigOptionRequest{
				ValueId: &acpsdk.SetSessionConfigOptionValueId{
					SessionId: "sess",
					ConfigId:  tt.configID,
					Value:     acpsdk.SessionConfigValueId(tt.value),
				},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SetSessionConfigOption() error = %v", err)
			}
		})
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
	skillSummaries  []atlasruntime.SkillSummary
	skillCWDs       []string
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
		Default: "test-model",
		Models: []atlasruntime.ModelOption{
			{
				Value:         "test-model",
				Name:          "Test Model",
				ContextWindow: 1000000,
				MaxTokens:     384000, InputFormats: []string{"text", "image"},
				ReasoningEfforts: []atlasruntime.ReasoningEffortOption{
					{Value: "high", Name: "High"},
					{Value: "max", Name: "Max"},
				},
			},
			{
				Value:         "other-model",
				Name:          "Other Model",
				Description:   "alternate",
				ContextWindow: 1000000,
				MaxTokens:     128000, InputFormats: []string{"text"},
				ReasoningEfforts: []atlasruntime.ReasoningEffortOption{
					{Value: "high", Name: "High"},
				},
			},
		},
	}, nil
}

func (f *fakeRuntime) SkillSummaries(_ context.Context, cwd string) ([]atlasruntime.SkillSummary, error) {
	f.skillCWDs = append(f.skillCWDs, cwd)
	return append([]atlasruntime.SkillSummary(nil), f.skillSummaries...), nil
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

func assertReasoningEffortOptions(t *testing.T, options []acpsdk.SessionConfigOption, want []string) {
	t.Helper()
	for _, option := range options {
		if option.Select == nil || option.Select.Id != reasoningEffortSessionConfigID() {
			continue
		}
		if option.Select.Options.Ungrouped == nil {
			t.Fatalf("reasoning effort options = %#v", option.Select.Options)
		}
		got := make([]string, 0, len(*option.Select.Options.Ungrouped))
		for _, item := range *option.Select.Options.Ungrouped {
			got = append(got, string(item.Value))
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("reasoning effort options = %#v, want %#v", got, want)
		}
		return
	}
	t.Fatalf("reasoning effort config option missing")
}

func currentConfigValue(options []acpsdk.SessionConfigOption, id acpsdk.SessionConfigId) string {
	for _, option := range options {
		if option.Select != nil && option.Select.Id == id {
			return string(option.Select.CurrentValue)
		}
	}
	return ""
}

func terminalCreateMatches(req acpsdk.CreateTerminalRequest, command, cwd string) bool {
	spec := tool.DefaultShell()
	wantArgs := append([]string(nil), spec.Args...)
	wantArgs = append(wantArgs, command)
	return req.Command == spec.Command && reflect.DeepEqual(req.Args, wantArgs) && req.Cwd != nil && *req.Cwd == cwd
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

// writeTestTextFile writes a test text file.
func writeTestTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

// writeTestBinaryFile writes a test binary file containing NUL bytes.
func writeTestBinaryFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

// mkdirTestDir creates a test directory.
func mkdirTestDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
}
