package acp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	agentpkg "github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	atlasruntime "github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
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

	if _, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: "relative"}); err == nil {
		t.Fatal("NewSession() error = nil")
	}
	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	state, ok := a.getSession(string(resp.SessionId))
	if !ok {
		t.Fatalf("session %q was not recorded", resp.SessionId)
	}
	if state.cwd != "/tmp/work" {
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

func TestNewSessionSendsCompactCommand(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	var updates []acpsdk.SessionNotification
	a.sendUpdate = func(_ context.Context, update acpsdk.SessionNotification) error {
		updates = append(updates, update)
		return nil
	}

	resp, err := a.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].SessionId != resp.SessionId {
		t.Fatalf("session id = %#v", updates[0])
	}
	commands := updates[0].Update.AvailableCommandsUpdate
	if commands == nil || len(commands.AvailableCommands) != 1 || commands.AvailableCommands[0].Name != "compact" {
		t.Fatalf("commands = %#v", updates[0].Update)
	}
	if commands.AvailableCommands[0].Input == nil || commands.AvailableCommands[0].Input.Unstructured == nil {
		t.Fatalf("command input = %#v", commands.AvailableCommands[0].Input)
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
	a.setSession("sess", "/tmp/work", "other-model", "max")
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

func TestPromptCompactCommandRunsCompaction(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "other-model", "max")
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
			a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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

func TestPromptUnsupportedContent(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work"},
		},
		sessionsForCWD: []session.Session{
			{ID: "sess", Title: "hello", CWD: "/tmp/work", UpdatedAt: now},
		},
	}
	a := NewAgent(rt)

	resume, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       "/tmp/work",
	})
	if err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
	}
	if got := currentModelValue(resume.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok || state.cwd != "/tmp/work" {
		t.Fatalf("session state = %#v, %t", state, ok)
	}
	cwd := "/tmp/work"
	list, err := a.ListSessions(context.Background(), acpsdk.ListSessionsRequest{Cwd: &cwd})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].SessionId != "sess" || list.Sessions[0].UpdatedAt == nil {
		t.Fatalf("sessions = %#v", list.Sessions)
	}
	if !reflect.DeepEqual(rt.listedCWDs, []string{"/tmp/work"}) {
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

func TestLoadSessionReplaysTranscript(t *testing.T) {
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
	trans.Append(model.Message{Role: model.RoleTool, Content: "ok", ToolCallID: "call_1"})
	trans.Append(model.Message{Role: model.RoleAssistant, Content: "done"})
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work"},
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
		Cwd:        "/tmp/work",
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got := currentModelValue(resp.ConfigOptions); got != "test-model" {
		t.Fatalf("current model = %q", got)
	}
	state, ok := a.getSession("sess")
	if !ok || state.cwd != "/tmp/work" || state.model != "test-model" || state.reasoningEffort != "high" {
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
	if len(finish.Content) != 1 || finish.Content[0].Content.Content.Text.Text != "ok" {
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
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work"},
		},
	}
	a := NewAgent(rt)

	_, err := a.LoadSession(context.Background(), acpsdk.LoadSessionRequest{
		SessionId:  "sess",
		Cwd:        "/tmp/other",
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
	trans := transcript.New()
	trans.Append(model.Message{Role: model.RoleUser, Content: "hi"})
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work"},
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
		Cwd:        "/tmp/work",
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
	rt := &fakeRuntime{
		showSessions: map[string]session.Session{
			"sess": {ID: "sess", CWD: "/tmp/work"},
		},
	}
	a := NewAgent(rt)

	_, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       "/tmp/other",
	})
	if err == nil || !strings.Contains(err.Error(), "cwd mismatch") {
		t.Fatalf("ResumeSession() error = %v", err)
	}
}

func TestSetSessionConfigOptionUpdatesModel(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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
	a.setSession("sess", "/tmp/work", "test-model", "")

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

func TestListSessionsRejectsCursor(t *testing.T) {
	a := NewAgent(&fakeRuntime{})
	cursor := "next"

	_, err := a.ListSessions(context.Background(), acpsdk.ListSessionsRequest{Cursor: &cursor})
	if err == nil || !strings.Contains(err.Error(), "cursor is not supported") {
		t.Fatalf("ListSessions() error = %v", err)
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
	deleted         []string
	sessions        []session.Session
	sessionsForCWD  []session.Session
	showSessions    map[string]session.Session
	showTranscripts map[string]*transcript.Transcript
	showErr         error
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
	return session.Session{}, nil, errors.New("session not found")
}

func (f *fakeRuntime) ListSessions(context.Context, int) ([]session.Session, error) {
	return f.sessions, nil
}

func (f *fakeRuntime) ListSessionsForCWD(_ context.Context, cwd string, _ int) ([]session.Session, error) {
	f.listedCWDs = append(f.listedCWDs, cwd)
	return f.sessionsForCWD, nil
}

func (f *fakeRuntime) DeleteSessionIfExists(_ context.Context, sessionID string) error {
	f.deleted = append(f.deleted, sessionID)
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
