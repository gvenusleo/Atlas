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
	caps := resp.AgentCapabilities.SessionCapabilities
	if caps.Close == nil || caps.Delete == nil || caps.List == nil || caps.Resume == nil {
		t.Fatalf("session capabilities = %#v", caps)
	}
	if resp.AgentCapabilities.LoadSession {
		t.Fatal("LoadSession capability should be disabled")
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
}

func TestPromptRunsRuntimeAndStreamsUpdates(t *testing.T) {
	rt := &fakeRuntime{}
	rt.run = func(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
		rt.runOptions = opts
		opts.Observer(agentpkg.Event{Type: agentpkg.EventModelDelta, Content: "hello"})
		opts.Observer(agentpkg.Event{
			Type: agentpkg.EventToolStarted,
			Step: 1,
			ToolCall: model.ToolCall{
				ID:        "call_1",
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			},
		})
		opts.Observer(agentpkg.Event{
			Type:       agentpkg.EventToolFinished,
			Step:       1,
			ToolCall:   model.ToolCall{ID: "call_1", Name: "read_file"},
			ToolResult: "content",
		})
		return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: "done"}, nil
	}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work")
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
	if rt.runOptions.SessionID != "sess" || rt.runOptions.Prompt != "hi" || rt.runOptions.CWD != "/tmp/work" {
		t.Fatalf("turn options = %#v", rt.runOptions)
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0].Update.AgentMessageChunk == nil || updates[0].Update.AgentMessageChunk.Content.Text.Text != "hello" {
		t.Fatalf("first update = %#v", updates[0].Update)
	}
	start := updates[1].Update.ToolCall
	if start == nil || start.ToolCallId != "call_1" || start.Kind != acpsdk.ToolKindRead || start.Status != acpsdk.ToolCallStatusInProgress {
		t.Fatalf("tool start = %#v", updates[1].Update)
	}
	if got := start.RawInput.(map[string]any)["path"]; got != "README.md" {
		t.Fatalf("raw input = %#v", start.RawInput)
	}
	finish := updates[2].Update.ToolCallUpdate
	if finish == nil || finish.ToolCallId != "call_1" || finish.Status == nil || *finish.Status != acpsdk.ToolCallStatusCompleted {
		t.Fatalf("tool finish = %#v", updates[2].Update)
	}
}

func TestPromptResourceLinkText(t *testing.T) {
	rt := &fakeRuntime{}
	a := NewAgent(rt)
	a.setSession("sess", "/tmp/work")

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
	a.setSession("sess", "/tmp/work")

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
	a.setSession("sess", "/tmp/work")

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

	if _, err := a.ResumeSession(context.Background(), acpsdk.ResumeSessionRequest{
		SessionId: "sess",
		Cwd:       "/tmp/work",
	}); err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
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

	runOptions     atlasruntime.TurnOptions
	listedCWDs     []string
	deleted        []string
	sessions       []session.Session
	sessionsForCWD []session.Session
	showSessions   map[string]session.Session
	showErr        error
}

func (f *fakeRuntime) RunTurn(ctx context.Context, opts atlasruntime.TurnOptions) (atlasruntime.TurnResult, error) {
	f.runOptions = opts
	if f.run != nil {
		return f.run(ctx, opts)
	}
	return atlasruntime.TurnResult{SessionID: opts.SessionID, Content: "ok"}, nil
}

func (f *fakeRuntime) ShowSession(_ context.Context, sessionID string) (session.Session, *transcript.Transcript, error) {
	if f.showErr != nil {
		return session.Session{}, nil, f.showErr
	}
	if sess, ok := f.showSessions[sessionID]; ok {
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
