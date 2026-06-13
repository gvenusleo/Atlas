package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/transcript"
)

func TestServerRunsTurnWithTypingAndSavesSession(t *testing.T) {
	var requestMu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requests = append(requests, r.URL.Path)
		requestMu.Unlock()
		switch r.URL.Path {
		case "/ilink/bot/getconfig":
			_, _ = w.Write([]byte(`{"typing_ticket":"ticket-1"}`))
		case "/ilink/bot/sendtyping":
			w.WriteHeader(http.StatusOK)
		case "/ilink/bot/sendmessage":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "hello")); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	waitFor(t, func() bool {
		requestMu.Lock()
		defer requestMu.Unlock()
		return containsPath(requests, "/ilink/bot/sendmessage")
	})

	prompt, cwd := rt.lastRun()
	if prompt != "hello" {
		t.Fatalf("prompt = %q", prompt)
	}
	if cwd != srv.cfg.DefaultCWD {
		t.Fatalf("cwd = %q", cwd)
	}
	requestMu.Lock()
	recordedRequests := append([]string(nil), requests...)
	requestMu.Unlock()
	if !containsPath(recordedRequests, "/ilink/bot/sendtyping") || !containsPath(recordedRequests, "/ilink/bot/sendmessage") {
		t.Fatalf("requests = %#v", recordedRequests)
	}
	state, err := srv.store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if state.Senders["user-1"].SessionID != "session-1" {
		t.Fatalf("sender state = %#v", state.Senders["user-1"])
	}
}

func TestServerRunDoesNotFetchTypingTicketAtStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/getupdates":
			cancel()
			_, _ = w.Write([]byte(`{}`))
		case "/ilink/bot/getconfig":
			t.Fatalf("unexpected startup getconfig request")
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	srv, _ := newTestServer(t, server.URL)
	if err := srv.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestServerRunsTurnWhenTypingTicketFetchFails(t *testing.T) {
	var output bytes.Buffer
	var requestMu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requests = append(requests, r.URL.Path)
		requestMu.Unlock()
		switch r.URL.Path {
		case "/ilink/bot/getconfig":
			_, _ = w.Write([]byte(`{"ret":1,"errmsg":"GetTypingTicket rpc failed"}`))
		case "/ilink/bot/sendmessage":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	srv.output = &output
	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "hello")); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	waitFor(t, func() bool {
		requestMu.Lock()
		defer requestMu.Unlock()
		return containsPath(requests, "/ilink/bot/sendmessage")
	})
	if !rt.wasRun() {
		t.Fatal("RunTurn was not called")
	}
	if strings.Contains(output.String(), "getconfig failed") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestServerSendsToolUpdatesToWeixin(t *testing.T) {
	var eventMu sync.Mutex
	var replies []string
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/getconfig":
			_, _ = w.Write([]byte(`{"typing_ticket":"ticket-1"}`))
		case "/ilink/bot/sendtyping":
			var req sendTypingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			eventMu.Lock()
			events = append(events, "typing:"+fmt.Sprint(req.Status))
			eventMu.Unlock()
		case "/ilink/bot/sendmessage":
			var req sendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			eventMu.Lock()
			replies = append(replies, req.Message.Items[0].TextItem.Text)
			events = append(events, "message:"+req.Message.Items[0].TextItem.Text)
			eventMu.Unlock()
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	rt.events = []agent.Event{{
		Type: agent.EventToolStarted,
		ToolCall: model.ToolCall{
			Name:      "run_shell",
			Arguments: `{"command":"just check"}`,
		},
	}}
	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "hello")); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	waitFor(t, func() bool {
		eventMu.Lock()
		defer eventMu.Unlock()
		return len(events) >= 5
	})

	eventMu.Lock()
	got := append([]string(nil), replies...)
	gotEvents := append([]string(nil), events...)
	eventMu.Unlock()
	if got[0] != "Run: just check" || got[1] != "reply" {
		t.Fatalf("replies = %#v", got)
	}
	wantEvents := []string{
		"typing:1",
		"message:Run: just check",
		"typing:1",
		"message:reply",
		"typing:2",
	}
	if strings.Join(gotEvents, "\n") != strings.Join(wantEvents, "\n") {
		t.Fatalf("events = %#v", gotEvents)
	}
}

func TestServerRejectsOtherSender(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	if err := srv.HandleMessage(context.Background(), textMessage("other", "hello")); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if rt.wasRun() {
		t.Fatal("RunTurn was called")
	}
}

func TestServerIgnoresUnknownMessageType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	msg := textMessage("user-1", "hello")
	msg.MessageType = 0
	if err := srv.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if rt.wasRun() {
		t.Fatal("RunTurn was called")
	}
}

func TestServerCWDCommandSwitchesDirectoryAndStartsNewSession(t *testing.T) {
	var replies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/sendmessage" {
			var req sendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			replies = append(replies, req.Message.Items[0].TextItem.Text)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	srv, _ := newTestServer(t, server.URL)
	oldState, err := srv.store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	oldState.Senders["user-1"] = SenderState{CWD: srv.cfg.DefaultCWD, SessionID: "old-session"}
	if err := srv.store.saveState(oldState); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	nextCWD := t.TempDir()

	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "/cwd "+nextCWD)); err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}

	state, err := srv.store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	got := state.Senders["user-1"]
	if got.CWD != nextCWD || got.SessionID != "" {
		t.Fatalf("sender state = %#v", got)
	}
	if len(replies) == 0 || !strings.Contains(replies[0], "next message will start a new conversation") {
		t.Fatalf("replies = %#v", replies)
	}
}

func TestServerSessionsAndResumeCommands(t *testing.T) {
	var replies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/sendmessage" {
			var req sendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			replies = append(replies, req.Message.Items[0].TextItem.Text)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	srv, rt := newTestServer(t, server.URL)
	rt.setSessions([]session.Session{{
		ID:        "session-1",
		Title:     "hello",
		CWD:       srv.cfg.DefaultCWD,
		UpdatedAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	}})

	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "/sessions")); err != nil {
		t.Fatalf("sessions HandleMessage() error = %v", err)
	}
	if len(replies) != 1 || !strings.Contains(replies[0], "session-1") {
		t.Fatalf("replies = %#v", replies)
	}

	if err := srv.HandleMessage(context.Background(), textMessage("user-1", "/resume session-1")); err != nil {
		t.Fatalf("resume HandleMessage() error = %v", err)
	}
	state, err := srv.store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if state.Senders["user-1"].SessionID != "session-1" {
		t.Fatalf("sender state = %#v", state.Senders["user-1"])
	}
}

type fakeRuntime struct {
	mu        sync.Mutex
	runCalled bool
	prompt    string
	cwd       string
	events    []agent.Event
	sessions  []session.Session
}

func (f *fakeRuntime) RunTurn(_ context.Context, opts runtime.TurnOptions) (runtime.TurnResult, error) {
	f.mu.Lock()
	f.runCalled = true
	f.prompt = opts.Prompt
	f.cwd = opts.CWD
	events := append([]agent.Event(nil), f.events...)
	f.mu.Unlock()
	if opts.Observer != nil {
		for _, event := range events {
			opts.Observer(event)
		}
	}
	return runtime.TurnResult{SessionID: "session-1", Content: "reply"}, nil
}

func (f *fakeRuntime) CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error) {
	return runtime.CompactResult{Compacted: true, CompactCount: 2, KeepCount: 2}, nil
}

func (f *fakeRuntime) ListSessions(context.Context, int) ([]session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]session.Session(nil), f.sessions...), nil
}

func (f *fakeRuntime) ListSessionsForCWD(_ context.Context, cwd string, _ int) ([]session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var filtered []session.Session
	for _, item := range f.sessions {
		if item.CWD == cwd {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (f *fakeRuntime) ShowSession(_ context.Context, id string) (session.Session, *transcript.Transcript, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.sessions {
		if item.ID == id {
			return item, transcript.New(), nil
		}
	}
	return session.Session{}, nil, context.Canceled
}

func (f *fakeRuntime) wasRun() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runCalled
}

func (f *fakeRuntime) lastRun() (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prompt, f.cwd
}

func (f *fakeRuntime) setSessions(sessions []session.Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append([]session.Session(nil), sessions...)
}

func newTestServer(t *testing.T, baseURL string) (*Server, *fakeRuntime) {
	t.Helper()

	store, err := NewStore(filepath.Join(t.TempDir(), "weixin"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	client, err := NewClient(ClientOptions{BaseURL: baseURL, Token: "token-1"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	rt := &fakeRuntime{}
	srv, err := NewServer(ServerOptions{
		Runtime: rt,
		Store:   store,
		Client:  client,
		Account: Account{ID: "account-1", UserID: "user-1", Token: "token-1", BaseURL: baseURL},
		Config:  config.WeixinConfig{DefaultCWD: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv, rt
}

func textMessage(from, text string) WeixinMessage {
	return WeixinMessage{
		FromUserID:  from,
		MessageType: messageTypeUser,
		Items: []MessageItem{{
			Type:     messageItemTypeText,
			TextItem: &TextItem{Text: text},
		}},
		ContextToken: "ctx",
		RunID:        "run",
	}
}

func containsPath(paths []string, target string) bool {
	for _, item := range paths {
		if item == target {
			return true
		}
	}
	return false
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
