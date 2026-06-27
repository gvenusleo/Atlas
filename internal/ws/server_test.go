package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/transcript"
)

// fakeRuntime 实现 ws.Runtime 接口用于测试。
type fakeRuntime struct {
	mu sync.Mutex

	modelOptions runtime.ModelOptions
	sessions     []session.Session
	transcript   *transcript.Transcript
	nextCursor   string

	turnResult runtime.TurnResult
	turnError  error

	// 记录最后一次 RunTurn 的参数
	lastTurnOpts runtime.TurnOptions
	listedCWD    string
	listedCursor string
	listedLimit  int
}

func (f *fakeRuntime) RunTurn(ctx context.Context, opts runtime.TurnOptions) (runtime.TurnResult, error) {
	f.mu.Lock()
	f.lastTurnOpts = opts
	f.mu.Unlock()

	// 模拟事件推送
	if opts.Observer != nil {
		opts.Observer(agent.Event{Type: agent.EventTurnStarted, Step: 0})
		opts.Observer(agent.Event{Type: agent.EventModelDelta, Step: 1, Content: "Hello!"})
		opts.Observer(agent.Event{Type: agent.EventTurnFinished, Step: 1})
	}

	if f.turnError != nil {
		return runtime.TurnResult{}, f.turnError
	}
	return f.turnResult, nil
}

func (f *fakeRuntime) CompactSession(ctx context.Context, opts runtime.CompactOptions) (runtime.CompactResult, error) {
	return runtime.CompactResult{SessionID: opts.SessionID, ContextWindow: 1000000}, nil
}

func (f *fakeRuntime) ModelOptions(ctx context.Context) (runtime.ModelOptions, error) {
	return f.modelOptions, nil
}

func (f *fakeRuntime) SkillSummaries(ctx context.Context, cwd string) ([]runtime.SkillSummary, error) {
	return []runtime.SkillSummary{{Name: "think", Description: "Plan before coding"}}, nil
}

func (f *fakeRuntime) ShowSession(ctx context.Context, id string) (session.Session, *transcript.Transcript, error) {
	for _, s := range f.sessions {
		if s.ID == id {
			return s, f.transcript, nil
		}
	}
	return session.Session{}, nil, fmt.Errorf("session %q not found", id)
}

func (f *fakeRuntime) ListSessionsPage(ctx context.Context, cursor string, limit int) (session.ListPage, error) {
	f.mu.Lock()
	f.listedCWD = ""
	f.listedCursor = cursor
	f.listedLimit = limit
	f.mu.Unlock()
	return session.ListPage{Sessions: f.sessions, NextCursor: f.nextCursor}, nil
}

func (f *fakeRuntime) ListSessionsForCWDPage(ctx context.Context, cwd, cursor string, limit int) (session.ListPage, error) {
	f.mu.Lock()
	f.listedCWD = cwd
	f.listedCursor = cursor
	f.listedLimit = limit
	f.mu.Unlock()

	var filtered []session.Session
	for _, s := range f.sessions {
		if s.CWD == cwd {
			filtered = append(filtered, s)
		}
	}
	return session.ListPage{Sessions: filtered, NextCursor: f.nextCursor}, nil
}

func (f *fakeRuntime) DeleteSessionIfExists(ctx context.Context, id string) error {
	return nil
}

func (f *fakeRuntime) RunMemoryWorker(ctx context.Context) error {
	return nil
}

// startTestServer 启动一个测试 WebSocket 服务并返回地址。
func startTestServer(t *testing.T, rt Runtime) (*Server, string) {
	t.Helper()
	srv, err := NewServer(ServerOptions{
		Runtime: rt,
		Host:    "127.0.0.1",
		Port:    0,
	})
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.handleWebSocket)
	httpSrv := &http.Server{Handler: mux}

	go httpSrv.Serve(ln)

	t.Cleanup(func() {
		httpSrv.Close()
	})

	return srv, ln.Addr().String()
}

// dialWS 连接测试 WebSocket 服务。
func dialWS(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/ws", addr), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

// sendMsg 发送一条 JSON 消息。
func sendMsg(t *testing.T, conn *websocket.Conn, msg ClientMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

// recvMsg 读取一条 JSON 消息。
func recvMsg(t *testing.T, conn *websocket.Conn) ServerMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var msg ServerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v, data: %s", err, data)
	}
	return msg
}

func TestModelOptions(t *testing.T) {
	rt := &fakeRuntime{
		modelOptions: runtime.ModelOptions{
			Default: "test-model",
			Models: []runtime.ModelOption{
				{Value: "test-model", Name: "Test Model", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{"text"}},
			},
		},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgModelOptions})
	msg := recvMsg(t, conn)

	if msg.Type != MsgModelOptionsResp {
		t.Fatalf("type = %q, want %q", msg.Type, MsgModelOptionsResp)
	}
	if msg.Default != "test-model" {
		t.Fatalf("default = %q", msg.Default)
	}
	if len(msg.Models) != 1 || msg.Models[0].Value != "test-model" {
		t.Fatalf("models = %#v", msg.Models)
	}
}

func TestPromptEvents(t *testing.T) {
	rt := &fakeRuntime{
		turnResult: runtime.TurnResult{SessionID: "test-session", Content: "Hello!"},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{
		Type:      MsgPrompt,
		SessionID: "test-session",
		CWD:       "/tmp",
		Content:   "hello",
	})

	// 应收到 turn_started, model_delta, turn_finished
	msg1 := recvMsg(t, conn)
	if msg1.Type != MsgEvent || msg1.Event != EventTurnStarted {
		t.Fatalf("msg1 = %#v", msg1)
	}
	// 所有事件都应携带 session_id
	if msg1.SessionID != "test-session" {
		t.Fatalf("turn_started session_id = %q, want %q", msg1.SessionID, "test-session")
	}

	msg2 := recvMsg(t, conn)
	if msg2.Type != MsgEvent || msg2.Event != EventModelDelta || msg2.Content != "Hello!" {
		t.Fatalf("msg2 = %#v", msg2)
	}
	if msg2.SessionID != "test-session" {
		t.Fatalf("model_delta session_id = %q, want %q", msg2.SessionID, "test-session")
	}

	msg3 := recvMsg(t, conn)
	if msg3.Type != MsgEvent || msg3.Event != EventTurnFinished {
		t.Fatalf("msg3 = %#v", msg3)
	}
	if msg3.SessionID != "test-session" {
		t.Fatalf("turn_finished session_id = %q, want %q", msg3.SessionID, "test-session")
	}

	// 验证 RunTurn 参数
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTurnOpts.SessionID != "test-session" {
		t.Fatalf("session ID = %q", rt.lastTurnOpts.SessionID)
	}
	if rt.lastTurnOpts.CWD != "/tmp" {
		t.Fatalf("cwd = %q", rt.lastTurnOpts.CWD)
	}
}

func TestPromptNewSession(t *testing.T) {
	// 客户端不传 session_id，runtime 生成新 ID 并通过 turn_finished 回传
	rt := &fakeRuntime{
		turnResult: runtime.TurnResult{SessionID: "new-session-abc"},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{
		Type:    MsgPrompt,
		CWD:     "/tmp",
		Content: "hello",
	})

	recvMsg(t, conn)        // turn_started
	recvMsg(t, conn)        // model_delta
	msg := recvMsg(t, conn) // turn_finished

	if msg.Type != MsgEvent || msg.Event != EventTurnFinished {
		t.Fatalf("msg = %#v", msg)
	}
	if msg.SessionID != "new-session-abc" {
		t.Fatalf("session_id = %q, want %q", msg.SessionID, "new-session-abc")
	}

	// 后续 prompt 传回 session_id，应复用同一 session
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "new-session-abc", Content: "world"})
	recvMsg(t, conn) // turn_started
	recvMsg(t, conn) // model_delta
	recvMsg(t, conn) // turn_finished

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTurnOpts.SessionID != "new-session-abc" {
		t.Fatalf("second prompt session_id = %q, want %q", rt.lastTurnOpts.SessionID, "new-session-abc")
	}
}

func TestShowSession(t *testing.T) {
	trans := transcript.New()
	// 用户消息同时设置 Content 和 Parts
	trans.Append(model.Message{Role: "user", Content: "hello", Parts: []model.ContentPart{{Type: model.ContentPartText, Text: "hello"}}})
	// assistant 消息只设置 Content，没有 Parts（与 agent 循环实际行为一致）
	trans.Append(model.Message{Role: "assistant", Content: "hi there"})

	rt := &fakeRuntime{
		sessions: []session.Session{
			{ID: "s1", Title: "Session 1", CWD: "/tmp"},
		},
		transcript: trans,
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgShowSession, SessionID: "s1"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSessionDetail {
		t.Fatalf("type = %q", msg.Type)
	}
	if msg.Session == nil || msg.Session.ID != "s1" {
		t.Fatalf("session = %#v", msg.Session)
	}
	if len(msg.Messages) != 2 {
		t.Fatalf("messages = %d", len(msg.Messages))
	}
	if msg.Messages[0].Role != "user" || msg.Messages[0].Content != "hello" {
		t.Fatalf("msg 0 = %#v", msg.Messages[0])
	}
	// assistant 消息只有 Content 没有 Parts，必须通过 MessageParts fallback 才能取到
	if msg.Messages[1].Role != "assistant" || msg.Messages[1].Content != "hi there" {
		t.Fatalf("msg 1 = %#v", msg.Messages[1])
	}
}

func TestListSessions(t *testing.T) {
	rt := &fakeRuntime{
		sessions: []session.Session{
			{ID: "s1", Title: "Session 1", CWD: "/tmp", LastTotalTokens: 100},
			{ID: "s2", Title: "Session 2", CWD: "/home", LastTotalTokens: 200},
		},
		nextCursor: "next-page",
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgListSessions})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSessions {
		t.Fatalf("type = %q", msg.Type)
	}
	if len(msg.Sessions) != 2 {
		t.Fatalf("sessions = %#v", msg.Sessions)
	}
	if msg.Sessions[0].ID != "s1" {
		t.Fatalf("first session = %#v", msg.Sessions[0])
	}
	if msg.NextCursor != "next-page" {
		t.Fatalf("next cursor = %q", msg.NextCursor)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.listedCursor != "" || rt.listedLimit != 50 {
		t.Fatalf("listed cursor/limit = %q/%d", rt.listedCursor, rt.listedLimit)
	}
}

func TestListSessionsSupportsCursor(t *testing.T) {
	rt := &fakeRuntime{
		sessions: []session.Session{
			{ID: "s1", Title: "Session 1", CWD: "/tmp"},
		},
		nextCursor: "page-2",
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgListSessions, Cursor: "page-1", Limit: 1})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSessions {
		t.Fatalf("type = %q", msg.Type)
	}
	if msg.NextCursor != "page-2" {
		t.Fatalf("next cursor = %q", msg.NextCursor)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.listedCursor != "page-1" || rt.listedLimit != 1 {
		t.Fatalf("listed cursor/limit = %q/%d", rt.listedCursor, rt.listedLimit)
	}
}

func TestListSessionsForCWD(t *testing.T) {
	rt := &fakeRuntime{
		sessions: []session.Session{
			{ID: "s1", Title: "Session 1", CWD: "/tmp"},
			{ID: "s2", Title: "Session 2", CWD: "/home"},
		},
		nextCursor: "cwd-page-2",
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgListSessions, CWD: "/tmp", Cursor: "cwd-page-1", Limit: 1})
	msg := recvMsg(t, conn)

	if len(msg.Sessions) != 1 || msg.Sessions[0].ID != "s1" {
		t.Fatalf("sessions = %#v", msg.Sessions)
	}
	if msg.NextCursor != "cwd-page-2" {
		t.Fatalf("next cursor = %q", msg.NextCursor)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.listedCWD != "/tmp" || rt.listedCursor != "cwd-page-1" || rt.listedLimit != 1 {
		t.Fatalf("listed cwd/cursor/limit = %q/%q/%d", rt.listedCWD, rt.listedCursor, rt.listedLimit)
	}
}

func TestDeleteSession(t *testing.T) {
	rt := &fakeRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgDeleteSession, SessionID: "s1"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSessionDeleted || msg.SessionID != "s1" {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestCompactSession(t *testing.T) {
	rt := &fakeRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgCompactSession, SessionID: "s1"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSessionCompacted || msg.SessionID != "s1" {
		t.Fatalf("msg = %#v", msg)
	}
	if msg.ContextWindow != 1000000 {
		t.Fatalf("context_window = %d", msg.ContextWindow)
	}
}

func TestCancelTurn(t *testing.T) {
	// 使用一个会阻塞的 fakeRuntime
	rt := &blockingRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	// 发送 prompt（带 session_id）
	sendMsg(t, conn, ClientMessage{
		Type:      MsgPrompt,
		SessionID: "s1",
		Content:   "long task",
	})

	// 等待 turn 开始
	msg1 := recvMsg(t, conn)
	if msg1.Event != EventTurnStarted {
		t.Fatalf("msg1 = %#v", msg1)
	}

	// 发送 cancel（必须带 session_id）
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "s1"})

	// 应收到 turn_finished (cancelled)
	msg2 := recvMsg(t, conn)
	if msg2.Type != MsgEvent || msg2.Event != EventTurnFinished {
		t.Fatalf("msg2 = %#v", msg2)
	}
}

func TestCancelRequiresSessionID(t *testing.T) {
	rt := &fakeRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgCancel})
	msg := recvMsg(t, conn)
	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v", msg)
	}
	if !strings.Contains(msg.Error, "session_id is required") {
		t.Fatalf("error = %q", msg.Error)
	}
}

func TestPromptError(t *testing.T) {
	// 非 context 取消的错误路径：应收到 error 事件（error_flag=true），不发 turn_finished
	rt := &fakeRuntime{
		turnResult: runtime.TurnResult{SessionID: "s1"},
		turnError:  fmt.Errorf("model unavailable"),
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "s1", Content: "hello"})

	// turn_started + model_delta（fakeRuntime 在返回错误前推送了事件）
	recvMsg(t, conn)
	recvMsg(t, conn)

	msg := recvMsg(t, conn)
	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v, want event/error", msg)
	}
	if !msg.HasError {
		t.Fatalf("error_flag = false, want true")
	}
	if !strings.Contains(msg.Error, "model unavailable") {
		t.Fatalf("error = %q", msg.Error)
	}
	if msg.SessionID != "s1" {
		t.Fatalf("session_id = %q, want s1", msg.SessionID)
	}
}

func TestConcurrentPromptSameSessionRejected(t *testing.T) {
	// 同一 session 的第二个 prompt 应被拒绝
	rt := &blockingRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	// 发送第一个 prompt（会阻塞）
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "s1", Content: "long task"})

	// 等待 turn 开始
	recvMsg(t, conn) // turn_started

	// 发送第二个 prompt 到同一 session，应收到错误
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "s1", Content: "second"})
	msg := recvMsg(t, conn)
	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v, want event/error", msg)
	}
	if !strings.Contains(msg.Error, "already running") {
		t.Fatalf("error = %q", msg.Error)
	}

	// 取消第一个 turn，清理
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "s1"})
	recvMsg(t, conn) // turn_finished (cancelled)
}

// blockingRuntime 在 RunTurn 中阻塞直到 context 被取消。
type blockingRuntime struct{}

func (b *blockingRuntime) RunTurn(ctx context.Context, opts runtime.TurnOptions) (runtime.TurnResult, error) {
	if opts.Observer != nil {
		opts.Observer(agent.Event{Type: agent.EventTurnStarted, Step: 0})
	}
	<-ctx.Done()
	return runtime.TurnResult{}, ctx.Err()
}

func (b *blockingRuntime) CompactSession(ctx context.Context, opts runtime.CompactOptions) (runtime.CompactResult, error) {
	return runtime.CompactResult{}, nil
}

func (b *blockingRuntime) ModelOptions(ctx context.Context) (runtime.ModelOptions, error) {
	return runtime.ModelOptions{}, nil
}

func (b *blockingRuntime) SkillSummaries(ctx context.Context, cwd string) ([]runtime.SkillSummary, error) {
	return nil, nil
}

func (b *blockingRuntime) ShowSession(ctx context.Context, id string) (session.Session, *transcript.Transcript, error) {
	return session.Session{}, nil, nil
}

func (b *blockingRuntime) ListSessionsPage(ctx context.Context, cursor string, limit int) (session.ListPage, error) {
	return session.ListPage{}, nil
}

func (b *blockingRuntime) ListSessionsForCWDPage(ctx context.Context, cwd, cursor string, limit int) (session.ListPage, error) {
	return session.ListPage{}, nil
}

func (b *blockingRuntime) DeleteSessionIfExists(ctx context.Context, id string) error {
	return nil
}

func (b *blockingRuntime) RunMemoryWorker(ctx context.Context) error {
	return nil
}

func TestSetModel(t *testing.T) {
	rt := &fakeRuntime{
		modelOptions: runtime.ModelOptions{
			Models: []runtime.ModelOption{{Value: "gpt-5", Name: "GPT-5"}},
		},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgSetModel, SessionID: "s1", Model: "gpt-5"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgModelSet || msg.Model != "gpt-5" {
		t.Fatalf("msg = %#v", msg)
	}

	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "s1", Content: "hello"})
	recvMsg(t, conn) // turn_started
	recvMsg(t, conn) // model_delta
	recvMsg(t, conn) // turn_finished

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTurnOpts.Model != "gpt-5" {
		t.Fatalf("turn model = %q", rt.lastTurnOpts.Model)
	}
}

func TestSetModelRejectsUnknownModel(t *testing.T) {
	rt := &fakeRuntime{
		modelOptions: runtime.ModelOptions{
			Models: []runtime.ModelOption{{Value: "gpt-5", Name: "GPT-5"}},
		},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgSetModel, SessionID: "s1", Model: "missing-model"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v", msg)
	}
	if !strings.Contains(msg.Error, "not configured") {
		t.Fatalf("error = %q", msg.Error)
	}
}

func TestSetModelRequiresSessionID(t *testing.T) {
	rt := &fakeRuntime{
		modelOptions: runtime.ModelOptions{
			Models: []runtime.ModelOption{{Value: "gpt-5", Name: "GPT-5"}},
		},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgSetModel, Model: "gpt-5"})
	msg := recvMsg(t, conn)
	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v", msg)
	}
	if !strings.Contains(msg.Error, "session_id is required") {
		t.Fatalf("error = %q", msg.Error)
	}
}

func TestSkillSummaries(t *testing.T) {
	rt := &fakeRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: MsgSkillSummaries, CWD: "/tmp"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgSkills {
		t.Fatalf("type = %q", msg.Type)
	}
	if len(msg.Skills) != 1 || msg.Skills[0].Name != "think" {
		t.Fatalf("skills = %#v", msg.Skills)
	}
}

func TestUnknownMessageType(t *testing.T) {
	rt := &fakeRuntime{}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{Type: "unknown"})
	msg := recvMsg(t, conn)

	if msg.Type != MsgEvent || msg.Event != EventError {
		t.Fatalf("msg = %#v", msg)
	}
	if !strings.Contains(msg.Error, "unknown message type") {
		t.Fatalf("error = %q", msg.Error)
	}
}

func TestParseClientMessage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		msg, err := ParseClientMessage([]byte(`{"type":"prompt","content":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		if msg.Type != MsgPrompt || msg.Content != "hello" {
			t.Fatalf("msg = %#v", msg)
		}
	})

	t.Run("missing type", func(t *testing.T) {
		_, err := ParseClientMessage([]byte(`{"content":"hello"}`))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseClientMessage([]byte(`{`))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPromptWithImageParts(t *testing.T) {
	rt := &fakeRuntime{
		turnResult: runtime.TurnResult{SessionID: "s1"},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	sendMsg(t, conn, ClientMessage{
		Type:      MsgPrompt,
		SessionID: "s1",
		Parts: []ContentPart{
			{Type: "text", Text: "what is this?"},
			{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"},
		},
	})

	// 消费事件
	recvMsg(t, conn) // turn_started
	recvMsg(t, conn) // model_delta
	recvMsg(t, conn) // turn_finished

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.lastTurnOpts.Parts) != 2 {
		t.Fatalf("parts = %d", len(rt.lastTurnOpts.Parts))
	}
	if rt.lastTurnOpts.Parts[0].Type != model.ContentPartText {
		t.Fatalf("part 0 type = %q", rt.lastTurnOpts.Parts[0].Type)
	}
	img := rt.lastTurnOpts.Parts[1]
	if img.Type != model.ContentPartImage {
		t.Fatalf("part 1 type = %q", img.Type)
	}
	if img.MimeType != "image/png" {
		t.Fatalf("mime_type = %q", img.MimeType)
	}
	// DataURL 必须是完整 data URL 格式，provider 直接传给 API
	if img.DataURL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("data_url = %q", img.DataURL)
	}
}

// --- 多会话并发测试 ---

// multiSessionRuntime 支持按 session_id 路由的阻塞 runtime。
type multiSessionRuntime struct {
	mu       sync.Mutex
	started  map[string]bool
	modelOpt runtime.ModelOptions
}

func newMultiSessionRuntime() *multiSessionRuntime {
	return &multiSessionRuntime{
		started: make(map[string]bool),
		modelOpt: runtime.ModelOptions{
			Models: []runtime.ModelOption{{Value: "gpt-5", Name: "GPT-5"}},
		},
	}
}

func (m *multiSessionRuntime) RunTurn(ctx context.Context, opts runtime.TurnOptions) (runtime.TurnResult, error) {
	m.mu.Lock()
	m.started[opts.SessionID] = true
	m.mu.Unlock()

	if opts.Observer != nil {
		opts.Observer(agent.Event{Type: agent.EventTurnStarted, Step: 0})
	}
	<-ctx.Done()
	return runtime.TurnResult{}, ctx.Err()
}

func (m *multiSessionRuntime) CompactSession(ctx context.Context, opts runtime.CompactOptions) (runtime.CompactResult, error) {
	return runtime.CompactResult{}, nil
}

func (m *multiSessionRuntime) ModelOptions(ctx context.Context) (runtime.ModelOptions, error) {
	return m.modelOpt, nil
}

func (m *multiSessionRuntime) SkillSummaries(ctx context.Context, cwd string) ([]runtime.SkillSummary, error) {
	return nil, nil
}

func (m *multiSessionRuntime) ShowSession(ctx context.Context, id string) (session.Session, *transcript.Transcript, error) {
	return session.Session{}, nil, nil
}

func (m *multiSessionRuntime) ListSessionsPage(ctx context.Context, cursor string, limit int) (session.ListPage, error) {
	return session.ListPage{}, nil
}

func (m *multiSessionRuntime) ListSessionsForCWDPage(ctx context.Context, cwd, cursor string, limit int) (session.ListPage, error) {
	return session.ListPage{}, nil
}

func (m *multiSessionRuntime) DeleteSessionIfExists(ctx context.Context, id string) error {
	return nil
}

func (m *multiSessionRuntime) RunMemoryWorker(ctx context.Context) error {
	return nil
}

func TestMultiSessionConcurrentTurns(t *testing.T) {
	// 两个不同 session 的 prompt 可以并发执行
	rt := newMultiSessionRuntime()
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	// 同时发送两个 prompt 到不同 session
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "session-a", Content: "task A"})
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "session-b", Content: "task B"})

	// 两个 turn 都应启动（收到两个 turn_started）
	msg1 := recvMsg(t, conn)
	if msg1.Event != EventTurnStarted {
		t.Fatalf("msg1 = %#v, want turn_started", msg1)
	}
	msg2 := recvMsg(t, conn)
	if msg2.Event != EventTurnStarted {
		t.Fatalf("msg2 = %#v, want turn_started", msg2)
	}

	// 两个 session 都应处于运行状态
	gotA, gotB := false, false
	for _, m := range []ServerMessage{msg1, msg2} {
		if m.SessionID == "session-a" {
			gotA = true
		}
		if m.SessionID == "session-b" {
			gotB = true
		}
	}
	if !gotA || !gotB {
		t.Fatalf("expected turn_started for both sessions, got A=%v B=%v", gotA, gotB)
	}

	// 取消两个 turn
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "session-a"})
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "session-b"})

	// 收到两个 turn_finished
	recvMsg(t, conn) // turn_finished for one
	recvMsg(t, conn) // turn_finished for other
}

func TestPerSessionCancel(t *testing.T) {
	// 取消 session A 不影响 session B
	rt := newMultiSessionRuntime()
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	// 启动两个 session 的 turn
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "sess-a", Content: "task A"})
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "sess-b", Content: "task B"})

	// 收到两个 turn_started
	recvMsg(t, conn) // turn_started for one
	recvMsg(t, conn) // turn_started for other

	// 只取消 session A
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "sess-a"})

	// 应收到 session A 的 turn_finished (cancelled)
	msg := recvMsg(t, conn)
	if msg.Event != EventTurnFinished {
		t.Fatalf("msg = %#v, want turn_finished", msg)
	}
	if msg.SessionID != "sess-a" {
		t.Fatalf("session_id = %q, want sess-a", msg.SessionID)
	}

	// session B 仍在运行，取消它以清理
	sendMsg(t, conn, ClientMessage{Type: MsgCancel, SessionID: "sess-b"})
	recvMsg(t, conn) // turn_finished for sess-b
}

func TestPerSessionModel(t *testing.T) {
	// 不同 session 可以使用不同模型
	rt := &fakeRuntime{
		modelOptions: runtime.ModelOptions{
			Models: []runtime.ModelOption{
				{Value: "gpt-5", Name: "GPT-5"},
				{Value: "claude-4", Name: "Claude 4"},
			},
		},
	}
	_, addr := startTestServer(t, rt)
	conn := dialWS(t, addr)

	// session A 用 gpt-5
	sendMsg(t, conn, ClientMessage{Type: MsgSetModel, SessionID: "sess-a", Model: "gpt-5"})
	recvMsg(t, conn) // model_set

	// session B 用 claude-4
	sendMsg(t, conn, ClientMessage{Type: MsgSetModel, SessionID: "sess-b", Model: "claude-4"})
	recvMsg(t, conn) // model_set

	// session A prompt 应使用 gpt-5
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "sess-a", Content: "hello"})
	recvMsg(t, conn) // turn_started
	recvMsg(t, conn) // model_delta
	recvMsg(t, conn) // turn_finished

	rt.mu.Lock()
	if rt.lastTurnOpts.Model != "gpt-5" {
		t.Fatalf("sess-a model = %q, want gpt-5", rt.lastTurnOpts.Model)
	}
	rt.mu.Unlock()

	// session B prompt 应使用 claude-4
	sendMsg(t, conn, ClientMessage{Type: MsgPrompt, SessionID: "sess-b", Content: "hello"})
	recvMsg(t, conn) // turn_started
	recvMsg(t, conn) // model_delta
	recvMsg(t, conn) // turn_finished

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTurnOpts.Model != "claude-4" {
		t.Fatalf("sess-b model = %q, want claude-4", rt.lastTurnOpts.Model)
	}
}
