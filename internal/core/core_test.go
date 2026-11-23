package core

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

func TestBrokenResourcesDoNotBlockSessions(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	cwd := t.TempDir()
	entry, err := encode(agent.UserMessage{Content: []agent.Content{agent.TextContent{Text: "历史"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.Create(t.Context(), store.Info{ID: "saved", Cwd: cwd}, entry); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{
		filepath.Join(os.Getenv("HOME"), ".atlas", "AGENTS.md"):     "\xff",
		filepath.Join(cwd, ".atlas", "skills", "demo", "SKILL.md"):  "格式错误",
		filepath.Join(cwd, ".agents", "skills", "demo", "SKILL.md"): "---\nname: demo\ndescription: 测试技能\n---\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(cwd, "missing"), filepath.Join(cwd, ".atlas", "skills", "broken")); err != nil {
		t.Fatal(err)
	}
	client := c.Connect(newRecorder())
	for _, kind := range []protocol.SessionType{protocol.SessionInteractive, protocol.SessionScheduled} {
		req := protocol.NewSessionRequest{Cwd: cwd, Type: kind}
		if kind == protocol.SessionScheduled {
			req.OwnerID = "task"
		}
		created, err := client.NewSession(t.Context(), req)
		if err != nil || !slices.ContainsFunc(created.State.Commands, func(cmd protocol.Command) bool {
			return cmd.Name == "demo"
		}) {
			t.Fatalf("新建 %s 会话未加载有效技能：%+v，%v", kind, created.State.Commands, err)
		}
		if len(created.State.Diagnostics) != 3 {
			t.Fatalf("应提示三项失败，收到：%q", created.State.Diagnostics)
		}
	}
	if _, err := client.OpenSession(t.Context(), protocol.OpenSessionRequest{SessionID: "saved"}); err != nil {
		t.Fatalf("打开已有会话失败：%v", err)
	}
}

const settingsJSON = `{"model": "p/a", "providers": {"p": {"api": "openai-responses",
	"base_url": "http://x", "models": {"a": {
	"reasoning_options": [{"type": "effort", "values": ["low", "high"]}]}, "b": {}}}}}`

// home 把 HOME 指到临时目录，写入 settings.json 与 p 的 API key，返回数据库路径。
func home(t *testing.T, settings string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	atlas := filepath.Join(dir, ".atlas")
	if err := os.MkdirAll(atlas, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"settings.json": settings, "auth.json": `{"p":"key"}`,
	} {
		if err := os.WriteFile(filepath.Join(atlas, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "atlas.db")
}

// open 在 path 上打开一个核心，就像一个进程。
func open(t *testing.T, path string) *Core {
	t.Helper()
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	c := New(db)
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// recorder 是记下收到的更新的订阅者。
type recorder struct {
	mu      sync.Mutex
	updates []protocol.Update
	ends    chan protocol.RunStateUpdate // 每一轮结束时的 run_state
}

func newRecorder() *recorder {
	return &recorder{ends: make(chan protocol.RunStateUpdate, 8)}
}

func (r *recorder) Receive(n protocol.Notification) {
	r.mu.Lock()
	r.updates = append(r.updates, n.Update)
	r.mu.Unlock()
	if s, ok := n.Update.(protocol.RunStateUpdate); ok && !s.Running {
		r.ends <- s
	}
}

// end 等一轮结束，返回结束时的 run_state。
func (r *recorder) end(t *testing.T) protocol.RunStateUpdate {
	t.Helper()
	select {
	case s := <-r.ends:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("一轮没有在期限内结束")
		return protocol.RunStateUpdate{}
	}
}

// texts 是收到的回复文字与用户消息文字，按顺序。
func (r *recorder) texts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	saved := map[string]bool{}
	for _, u := range r.updates {
		if message, ok := u.(protocol.AssistantMessageSaved); ok {
			saved[message.MessageID] = true
		}
	}
	var out []string
	for _, u := range r.updates {
		switch u := u.(type) {
		case protocol.UserMessageChunk:
			out = append(out, "用户:"+u.Content.Text)
		case protocol.AgentMessageChunk:
			if !saved[u.MessageID] {
				out = append(out, "助手:"+u.Content.Text)
			}
		case protocol.AssistantMessageSaved:
			for _, update := range u.Updates {
				if chunk, ok := update.(protocol.AgentMessageChunk); ok {
					out = append(out, "助手:"+chunk.Content.Text)
				}
			}
		}
	}
	return out
}

func (r *recorder) has(kind string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.updates, func(u protocol.Update) bool {
		return u.Type() == kind
	})
}

// openSession 让 a 打开会话 id。
func openSession(t *testing.T, a protocol.Service, id string) {
	t.Helper()
	_, err := a.OpenSession(t.Context(), protocol.OpenSessionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
}

// stub 让会话用 stream 代替真的模型调用。
func stub(t *testing.T, c *Core, id string, stream agent.StreamFn) {
	t.Helper()
	s, err := c.lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.model.Stream = stream
	s.mu.Unlock()
}

// reply 回复固定的文字，并记下请求里的消息数。
func reply(text string, seen *int) agent.StreamFn {
	return func(
		_ context.Context, req agent.StreamRequest, _ func(agent.Delta),
	) agent.AssistantMessage {
		if seen != nil {
			*seen = len(req.Messages)
		}
		return agent.AssistantMessage{StopReason: agent.StopReasonStop,
			Content: []agent.Content{agent.TextContent{Text: text}}}
	}
}

// say 发一条用户消息，返回核心是否受理。
func say(t *testing.T, a protocol.Service, id, text string) error {
	t.Helper()
	_, err := a.SendPrompt(t.Context(), protocol.SendPromptRequest{
		SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text(text)}})
	return err
}

// sayAndWait 发一条用户消息，等这一轮结束。
func sayAndWait(
	t *testing.T, a protocol.Service, r *recorder, id, text string,
) protocol.RunStateUpdate {
	t.Helper()
	if err := say(t, a, id, text); err != nil {
		t.Fatal(err)
	}
	return r.end(t)
}

// 运行中保存配置后，当前回复的重试仍用旧模型，同一轮的下一次请求使用新配置。
func TestConfigChangeDuringTurn(t *testing.T) {
	path, requests := configChangeProvider(t)
	c := open(t, path)
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	attempts, release := stubConfigChange(t, c, id)
	if err := say(t, client, id, "开场"); err != nil {
		t.Fatal(err)
	}
	assertModelAttempt(t, attempts, "p/a", agent.ThinkingLow)
	set := func(option, value string) []protocol.ConfigOption {
		t.Helper()
		resp, err := client.SetConfigOption(t.Context(), protocol.SetConfigOptionRequest{
			SessionID: id, ConfigID: option, Value: value})
		if err != nil {
			t.Fatal(err)
		}
		return resp.ConfigOptions
	}
	options := set(protocol.ConfigModel, "q/a")
	if !slices.ContainsFunc(options, func(o protocol.ConfigOption) bool {
		return o.ID == protocol.ConfigThinking && o.CurrentValue == "medium"
	}) {
		t.Fatalf("新模型的思考深度错误：%+v", options)
	}
	set(protocol.ConfigThinking, "high")
	if _, err := client.SetConfigOption(t.Context(), protocol.SetConfigOptionRequest{
		SessionID: id, ConfigID: protocol.ConfigThinking, Value: "low",
	}); err == nil {
		t.Fatal("接受了新模型不支持的思考深度")
	}
	close(release)
	assertModelAttempt(t, attempts, "p/a", agent.ThinkingLow)
	assertSwitchedRequest(t, awaitPromptEvent(t, requests), false)
	if state := r.end(t); state.Error != "" || state.StopReason != protocol.StopEndTurn {
		t.Fatalf("切换后的同一轮没有完成：%+v", state)
	}
	other := newRecorder()
	reopened := open(t, path).Connect(other)
	openSession(t, reopened, id)
	if state := sayAndWait(t, reopened, other, id, "继续"); state.Error != "" {
		t.Fatalf("重新打开会话后发送失败：%+v", state)
	}
	assertSwitchedRequest(t, awaitPromptEvent(t, requests), true)
}

func configChangeProvider(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case requests <- body:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		const done = `{"type":"response.completed","response":{"status":"completed","output":[` +
			`{"type":"reasoning","id":"rs_new","summary":[]}]}}`
		fmt.Fprint(w, "data: "+done+"\n\n")
	}))
	t.Cleanup(server.Close)
	settings := `{"model":"p/a","providers":{"p":{"api":"openai-responses",
		"base_url":"http://x","models":{"a":{"reasoning_options":[{"type":"effort",
		"values":["low"]}]}}},"q":{"api":"openai-responses","base_url":` +
		strconv.Quote(server.URL) + `,"models":{"a":{"reasoning_options":[{"type":"effort",
		"values":["medium","high"]}]}}}}}`
	path := home(t, settings)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".atlas", "auth.json"),
		[]byte(`{"p":"key","q":"key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, requests
}

func stubConfigChange(t *testing.T, c *Core, id string) (
	attempts chan agent.StreamRequest, release chan struct{},
) {
	t.Helper()
	attempts = make(chan agent.StreamRequest, 4)
	release = make(chan struct{})
	first := true
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest,
		_ func(agent.Delta),
	) agent.AssistantMessage {
		select {
		case attempts <- req:
		case <-ctx.Done():
			return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
		}
		if first {
			first = false
			select {
			case <-release:
			case <-ctx.Done():
				return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
			}
			return agent.AssistantMessage{StopReason: agent.StopReasonError,
				ErrorKind: agent.ErrorKindRetryable, ErrorMessage: "暂时失败"}
		}
		return agent.AssistantMessage{StopReason: agent.StopReasonToolUse,
			Content: []agent.Content{
				agent.ThinkingContent{Thinking: "原始思考",
					Signature: `{"type":"reasoning","id":"rs_old","summary":[]}`},
				agent.TextContent{Text: "工具调用前的正文"},
				agent.ToolCall{ID: "call_1", Name: "missing", Arguments: []byte(`{}`)},
			}}
	})
	return attempts, release
}

func assertModelAttempt(
	t *testing.T, attempts <-chan agent.StreamRequest, want string, depth agent.ThinkingLevel,
) {
	t.Helper()
	if req := awaitPromptEvent(t, attempts); req.Model != want || req.ThinkingLevel != depth {
		t.Fatalf("模型 = %q，思考深度 = %q，期望 %q、%q", req.Model, req.ThinkingLevel, want, depth)
	}
}

func assertSwitchedRequest(t *testing.T, body []byte, wantReasoning bool) {
	t.Helper()
	var req struct {
		Model     string `json:"model"`
		Reasoning struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		Input []struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			ID     string `json:"id"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	var calls, results int
	var reasoning bool
	for _, item := range req.Input {
		switch {
		case item.Type == "reasoning":
			if item.ID != "rs_new" {
				t.Fatalf("跨提供方请求携带了旧的推理项：%q", item.ID)
			}
			reasoning = true
		case item.Type == "function_call" && item.CallID == "call_1":
			calls++
		case item.Type == "function_call_output" && item.CallID == "call_1":
			results++
		}
	}
	if req.Model != "a" || req.Reasoning.Effort != "high" || calls != 1 || results != 1 ||
		reasoning != wantReasoning || !strings.Contains(string(body), "工具调用前的正文") {
		t.Fatalf("切换后的请求缺少模型配置、正文或工具记录：%s", body)
	}
}

// 新会话先只在内存里，第一条用户消息保存时写库并以它作标题；客户端只从更新里看到对话，
// 回放的历史与实时收到的是同样的内容。
func TestNewSessionIsSavedWithFirstPrompt(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	client := newRecorder()
	a := c.Connect(client)
	resp, err := a.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := a.ListSessions(t.Context(), protocol.ListSessionsRequest{})
	if len(list.Sessions) != 0 {
		t.Fatalf("发消息之前列表 = %v，期望为空", list.Sessions)
	}
	stub(t, c, resp.SessionID, reply("好", nil))
	end := sayAndWait(t, a, client, resp.SessionID, "你好")
	if end.StopReason != protocol.StopEndTurn {
		t.Fatalf("结束时 run_state = %+v，期望 end_turn", end)
	}

	if got := client.texts(); !slices.Equal(got, []string{"用户:你好", "助手:好"}) {
		t.Errorf("收到的对话 = %v", got)
	}
	if !client.has("session_info_update") || !client.has("run_state") {
		t.Error("没有收到标题或运行状态的更新")
	}
	list, err = a.ListSessions(t.Context(), protocol.ListSessionsRequest{})
	if err != nil || len(list.Sessions) != 1 || list.Sessions[0].Title != "你好" {
		t.Fatalf("列表 = %+v（%v），期望一个标题为“你好”的会话", list.Sessions, err)
	}
	history, err := a.ListHistory(t.Context(),
		protocol.ListHistoryRequest{SessionID: resp.SessionID, Limit: 10})
	replayed := &recorder{updates: history.Updates}
	if err != nil || !slices.Equal(replayed.texts(), client.texts()) {
		t.Errorf("回放 = %v（%v），期望与实时收到的一样", replayed.texts(), err)
	}
}

// 两个进程打开同一个库，同时写同一个会话是不支持的用法：后写入的一方这一轮以错误结束，
// 会话在它的进程内失效；重新打开之后带着对方的消息继续。
func TestConflictInvalidatesSession(t *testing.T) {
	path := home(t, settingsJSON)
	c1, c2 := open(t, path), open(t, path)
	r1, r2 := newRecorder(), newRecorder()
	a1, a2 := c1.Connect(r1), c2.Connect(r2)
	resp, err := a1.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	stub(t, c1, id, reply("一", nil))
	sayAndWait(t, a1, r1, id, "甲")
	openSession(t, a2, id)
	stub(t, c2, id, reply("二", nil))
	sayAndWait(t, a2, r2, id, "乙") // 另一个进程写过这个会话

	end := sayAndWait(t, a1, r1, id, "丙")
	if !strings.Contains(end.Error, protocol.ErrConflict.Error()) {
		t.Fatalf("过时的一方这一轮 = %+v，期望以写入冲突结束", end)
	}
	if err := say(t, a1, id, "丁"); !errors.Is(err, protocol.ErrNotFound) {
		t.Fatalf("失效的会话再发消息 = %v，期望 ErrNotFound", err)
	}

	openSession(t, a1, id)
	var seen int
	stub(t, c1, id, reply("四", &seen))
	sayAndWait(t, a1, r1, id, "戊")
	// 上下文：甲、一、乙、二，加上这一轮的戊。
	if seen != 5 {
		t.Errorf("重新打开后的请求有 %d 条消息，期望 5 条", seen)
	}
}

// 会话属于订阅它的连接：没订阅的连接不能对它发消息。同一进程里两个连接订阅同一个会话，
// 都收到更新；运行中再发消息返回 ErrBusy，取消后这一轮以 cancelled 结束。
func TestClientsShareSession(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r1, r2 := newRecorder(), newRecorder()
	a1, a2 := c.Connect(r1), c.Connect(r2)
	resp, err := a1.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	stub(t, c, id, reply("好", nil))
	sayAndWait(t, a1, r1, id, "你好")
	if err := say(t, a2, id, "没打开就发"); !errors.Is(err, protocol.ErrNotFound) {
		t.Fatalf("没订阅会话的连接发消息 = %v，期望 ErrNotFound", err)
	}
	openSession(t, a2, id)

	entered := make(chan struct{})
	stub(t, c, id, func(ctx context.Context, _ agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		emit(agent.Delta{Type: agent.DeltaTypeText, Text: "想"})
		close(entered)
		<-ctx.Done()
		return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
	})
	if err := say(t, a1, id, "再想想"); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := say(t, a2, id, "插一句"); !errors.Is(err, protocol.ErrBusy) {
		t.Fatalf("运行中再发消息 = %v，期望 ErrBusy", err)
	}
	if err := a2.CancelTurn(t.Context(), protocol.CancelTurnRequest{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if end := r1.end(t); end.StopReason != protocol.StopCancelled {
		t.Errorf("被取消的一轮 = %+v，期望 cancelled", end)
	}
	r2.end(t) // 后订阅的连接也收到结束
	want := []string{"用户:你好", "助手:好", "用户:再想想", "助手:想"}
	if got := r2.texts(); !slices.Equal(got, want[2:]) {
		t.Errorf("后打开的连接收到 %v，期望 %v", got, want[2:])
	}
	if got := r1.texts(); !slices.Equal(got, want) {
		t.Errorf("先打开的连接收到 %v，期望 %v", got, want)
	}
}

// 一轮归会话所有，与连接无关：所有订阅者都关闭会话也不会中止这一轮，它跑完之后会话才卸载。
func TestCloseSessionDoesNotStopTurn(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	a := c.Connect(r)
	resp, err := a.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	entered, release := make(chan struct{}), make(chan struct{})
	stub(t, c, id, func(_ context.Context, _ agent.StreamRequest, _ func(agent.Delta),
	) agent.AssistantMessage {
		close(entered)
		<-release
		return agent.AssistantMessage{StopReason: agent.StopReasonStop,
			Content: []agent.Content{agent.TextContent{Text: "好"}}}
	})
	if err := say(t, a, id, "你好"); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := a.CloseSession(t.Context(), protocol.CloseSessionRequest{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.lookup(id); err != nil {
		t.Fatalf("关闭时这一轮还在运行，会话 lookup = %v，期望仍然加载着", err)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := c.lookup(id); errors.Is(err, protocol.ErrNotFound) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("这一轮结束后会话没有卸载")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 这一轮跑完了：消息都已入库。
	history, err := a.ListHistory(t.Context(),
		protocol.ListHistoryRequest{SessionID: id, Limit: 10})
	if err != nil || len(history.Updates) == 0 {
		t.Errorf("关闭后读历史 = %d 条（%v），期望这一轮的消息已入库", len(history.Updates), err)
	}
}

// 图片外置到 blob 里，读出时还原；压缩之后上下文的起点前移，重新打开只带摘要和保留的消息，
// 之前的消息不再进入上下文。
func TestImagesAndCompactionAcrossReopen(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	a := c.Connect(r)
	resp, err := a.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	started, release := make(chan struct{}), make(chan struct{})
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest, emit func(agent.Delta)) agent.AssistantMessage {
		if len(req.Messages) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return reply("好", nil)(ctx, req, emit)
	})
	if err := say(t, a, id, "较早的任务"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, started)
	runID := openPromptState(t, a, id).Run.RunID
	if err := enqueue(t, a, id, runID, protocol.PromptInTurn, "同一轮的补充"); err != nil {
		t.Fatal(err)
	}
	close(release)
	r.end(t)
	if err := a.CloseSession(t.Context(), protocol.CloseSessionRequest{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	openSession(t, a, id)
	stub(t, c, id, reply("好", nil))
	if err := say(t, a, id, "/compact"); !errors.Is(err, protocol.ErrNothingToCompact) {
		t.Fatalf("只有一轮时压缩返回 %v，期望没有可压缩的内容", err)
	}
	image := protocol.Image("AQID", "image/png")
	if _, err := a.SendPrompt(t.Context(), protocol.SendPromptRequest{
		SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text("看图"), image}}); err != nil {
		t.Fatal(err)
	}
	r.end(t)

	history, err := a.ListHistory(t.Context(),
		protocol.ListHistoryRequest{SessionID: id, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var got protocol.ContentBlock
	for _, u := range history.Updates {
		chunk, ok := u.(protocol.UserMessageChunk)
		if ok && chunk.Content.Type == protocol.ContentImage {
			got = chunk.Content
		}
	}
	if got != image {
		t.Errorf("读回的图片 = %+v，期望还原成 %+v", got, image)
	}

	if err := say(t, a, id, "/compact"); err != nil {
		t.Fatal(err)
	}
	if end := r.end(t); end.Error != "" {
		t.Fatalf("压缩 = %+v，期望成功", end)
	}
	if err := a.CloseSession(t.Context(), protocol.CloseSessionRequest{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	info, err := c.db.Get(t.Context(), id)
	if err != nil || info.ContextFrom <= 1 {
		t.Fatalf("压缩后上下文起点 = %+v（%v），期望前移", info, err)
	}

	openSession(t, a, id)
	stub(t, c, id, func(
		_ context.Context, req agent.StreamRequest, _ func(agent.Delta),
	) agent.AssistantMessage {
		if len(req.Messages) != 4 {
			t.Errorf("重开后有 %d 条消息，期望摘要、最近一轮和新的用户消息", len(req.Messages))
			return agent.AssistantMessage{StopReason: agent.StopReasonStop}
		}
		user, ok := req.Messages[1].(agent.UserMessage)
		if !ok || agent.TextOf(user.Content) != "看图\n[图片]" || len(user.Content) != 2 {
			t.Errorf("重开后未保留最近一轮的用户原文和图片：%#v", req.Messages[1])
		} else if img, ok := user.Content[1].(agent.ImageContent); !ok || img.Data != image.Data {
			t.Error("重开后未还原最近一轮的图片内容")
		}
		return agent.AssistantMessage{StopReason: agent.StopReasonStop}
	})
	sayAndWait(t, a, r, id, "继续")
	history, err = a.ListHistory(t.Context(),
		protocol.ListHistoryRequest{SessionID: id, Limit: 10})
	if err != nil || !slices.ContainsFunc(history.Updates, func(u protocol.Update) bool {
		chunk, ok := u.(protocol.UserMessageChunk)
		return ok && chunk.Content.Text == "较早的任务"
	}) {
		t.Fatalf("压缩后未读回已摘要的历史：%v", err)
	}
}

// 取消超限恢复时，最近一次成功压缩和待发消息仍可用。
func TestCancelOverflowRecoveryPreservesSession(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	stub(t, c, id, reply("摘要", nil))
	sayAndWait(t, client, r, id, "已完成的任务")
	sayAndWait(t, client, r, id, strings.Repeat("近", 2000))
	if end := sayAndWait(t, client, r, id, "/compact"); end.Error != "" {
		t.Fatalf("手动压缩失败：%+v", end)
	}
	before, err := c.db.Get(t.Context(), id)
	if err != nil || before.ContextFrom <= 1 {
		t.Fatalf("未保存手动压缩：%+v，%v", before, err)
	}
	assertFailedReplayPreservesContext(t, c, client, r, id, before.ContextFrom)
	started := make(chan struct{})
	var summaries int
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest, _ func(agent.Delta)) agent.AssistantMessage {
		if len(req.Tools) == 0 {
			summaries++
			if summaries == 2 {
				close(started)
				<-ctx.Done()
				return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
			}
		}
		return agent.AssistantMessage{StopReason: agent.StopReasonError,
			ErrorKind: agent.ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
	})
	if err := say(t, client, id, "继续"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, started)
	runID := openPromptState(t, client, id).Run.RunID
	if err := enqueue(t, client, id, runID, protocol.PromptNextTurn, "待发"); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelTurn(t.Context(), protocol.CancelTurnRequest{SessionID: id, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if end := r.end(t); end.StopReason != protocol.StopCancelled {
		t.Fatalf("恢复取消后的状态：%+v", end)
	}
	queue := openPromptState(t, client, id).Queue
	if !queue.Paused || len(queue.Items) != 1 || queue.Items[0].Prompt[0].Text != "待发" {
		t.Fatalf("恢复取消后应保留待发消息：%+v", queue)
	}
	after, err := c.db.Get(t.Context(), id)
	if err != nil || after.ContextFrom != before.ContextFrom {
		t.Fatalf("恢复取消后应保留前次压缩的上下文起点：%+v，%v", after, err)
	}
}

// 重发再次超限时，已生成的临时摘要不能改变持久化的上下文起点。
func assertFailedReplayPreservesContext(t *testing.T, c *Core, client protocol.Service,
	r *recorder, id string, contextFrom int64,
) {
	t.Helper()
	var replies int
	stub(t, c, id, func(_ context.Context, req agent.StreamRequest, _ func(agent.Delta)) agent.AssistantMessage {
		if len(req.Tools) == 0 {
			return agent.AssistantMessage{StopReason: agent.StopReasonStop,
				Content: []agent.Content{agent.TextContent{Text: "临时摘要"}}}
		}
		replies++
		return agent.AssistantMessage{StopReason: agent.StopReasonError,
			ErrorKind: agent.ErrorKindContextOverflow, ErrorMessage: "上下文不足"}
	})
	if end := sayAndWait(t, client, r, id, "无法继续"); end.Error == "" || replies != 2 {
		t.Fatalf("重发应超限且结束，请求次数 %d，状态 %+v", replies, end)
	}
	after, err := c.db.Get(t.Context(), id)
	if err != nil || after.ContextFrom != contextFrom {
		t.Fatalf("重发超限后应保留前次压缩的上下文起点：%+v，%v", after, err)
	}
}

// 一轮用完模型请求次数的上限时以 max_turn_requests 结束，说明原因的通知写进了历史，
// 重新读出来仍在。
func TestTurnStopsAtRequestLimit(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	client := newRecorder()
	a := c.Connect(client)
	resp, err := a.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	stub(t, c, resp.SessionID, func(
		context.Context, agent.StreamRequest, func(agent.Delta),
	) agent.AssistantMessage {
		requests++ // 调用不存在的工具：得到错误结果，模型接着再调用
		return agent.AssistantMessage{StopReason: agent.StopReasonToolUse,
			Content: []agent.Content{agent.ToolCall{ID: strconv.Itoa(requests), Name: "none"}}}
	})
	end := sayAndWait(t, a, client, resp.SessionID, "开始")
	if end.StopReason != protocol.StopMaxTurnRequests || requests != maxRequests {
		t.Fatalf("结束时 run_state = %+v，请求 %d 次，期望 max_turn_requests、%d 次",
			end, requests, maxRequests)
	}
	history, err := a.ListHistory(t.Context(),
		protocol.ListHistoryRequest{SessionID: resp.SessionID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	last := history.Updates[len(history.Updates)-1]
	if n, ok := last.(protocol.Notice); !ok || !strings.Contains(n.Title, "上限") {
		t.Errorf("历史的最后一条 = %#v，期望说明上限的通知", last)
	}
}

// 类型与属主不合法的建会话请求被拒绝：任务会话没有属主、普通会话有属主、不认识的类型。
func TestNewSessionRejectsBadTypeAndOwner(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	a := c.Connect(newRecorder())
	for _, bad := range []protocol.NewSessionRequest{
		{Cwd: t.TempDir(), Type: protocol.SessionScheduled},
		{Cwd: t.TempDir(), OwnerID: "t1"},
		{Cwd: t.TempDir(), Type: "other"},
	} {
		if _, err := a.NewSession(t.Context(), bad); err == nil {
			t.Errorf("NewSession(%+v) 应当被拒绝", bad)
		}
	}
}

// 更新最早的会话会把它移到游标之前。目录删除需要一次拿到全部标识，才能固定确认范围。
func TestDirectorySessionSnapshot(t *testing.T) {
	ctx := t.Context()
	c := open(t, filepath.Join(t.TempDir(), "sessions.db"))
	a := c.Connect(newRecorder())
	cwd := t.TempDir()
	for i := range listPage + 1 {
		if _, err := c.db.Create(ctx, store.Info{ID: strconv.Itoa(i), Cwd: cwd}); err != nil {
			t.Fatal(err)
		}
	}
	for _, info := range []store.Info{
		{ID: "other", Cwd: t.TempDir()},
		{ID: "task", Cwd: cwd, Type: string(protocol.SessionScheduled), OwnerID: "task"},
	} {
		if _, err := c.db.Create(ctx, info); err != nil {
			t.Fatal(err)
		}
	}
	req := protocol.ListSessionsRequest{Cwd: cwd}
	page, err := a.ListSessions(ctx, req)
	if err != nil || len(page.Sessions) != listPage || page.NextCursor == "" {
		t.Fatalf("目录第一页包含 %d 个会话，游标 %q：%v", len(page.Sessions), page.NextCursor, err)
	}
	req.Cursor = page.NextCursor
	tail, err := a.ListSessions(ctx, req)
	if err != nil || len(tail.Sessions) != 1 {
		t.Fatalf("目录第二页包含 %d 个会话：%v", len(tail.Sessions), err)
	}
	last := tail.Sessions[0].SessionID
	if err := c.db.Rename(ctx, last, "新标题"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.ListSessions(ctx, protocol.ListSessionsRequest{Cwd: cwd, All: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool)
	for _, info := range snapshot.Sessions {
		ids[info.SessionID] = true
	}
	if !ids[last] || ids["task"] || ids["other"] {
		t.Fatalf("目录快照中会话标识与筛选范围不符：%v", ids)
	}
	if snapshot.NextCursor != "" || len(ids) != listPage+1 {
		t.Fatalf("目录快照包含 %d 个会话，游标 %q", len(ids), snapshot.NextCursor)
	}
}

// 任务会话带着类型与属主保存：默认的列表（侧栏用）只有普通会话，按类型与属主才列得出任务会话，
// 重新打开核心之后仍然如此。
func TestScheduledSessionsAreListedSeparately(t *testing.T) {
	path := home(t, settingsJSON)
	c := open(t, path)
	client := newRecorder()
	a := c.Connect(client)
	newSession := func(sessionType protocol.SessionType, owner string) string {
		t.Helper()
		resp, err := a.NewSession(t.Context(), protocol.NewSessionRequest{
			Cwd: t.TempDir(), Type: sessionType, OwnerID: owner})
		if err != nil {
			t.Fatal(err)
		}
		stub(t, c, resp.SessionID, reply("好", nil))
		sayAndWait(t, a, client, resp.SessionID, "做事")
		return resp.SessionID
	}
	chat := newSession("", "")
	first := newSession(protocol.SessionScheduled, "t1")
	newSession(protocol.SessionScheduled, "t1")
	newSession(protocol.SessionScheduled, "t2")

	count := func(a protocol.Service, req protocol.ListSessionsRequest) []protocol.SessionInfo {
		t.Helper()
		resp, err := a.ListSessions(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.Sessions
	}
	check := func(a protocol.Service) {
		t.Helper()
		got := count(a, protocol.ListSessionsRequest{})
		if len(got) != 1 || got[0].SessionID != chat {
			t.Errorf("默认的列表 = %+v，期望只有普通会话", got)
		}
		scheduled := protocol.ListSessionsRequest{Type: protocol.SessionScheduled}
		if got := count(a, scheduled); len(got) != 3 {
			t.Errorf("所有任务会话 = %d 个，期望 3 个", len(got))
		}
		scheduled.OwnerID = "t1"
		got = count(a, scheduled)
		if len(got) != 2 || got[0].OwnerID != "t1" || got[0].Type != protocol.SessionScheduled {
			t.Errorf("t1 的任务会话 = %+v，期望 2 个，带着类型与属主", got)
		}
	}
	check(a)

	// 重新打开：类型与属主存在库里；任务会话再打开、再保存，也不会变。
	c2 := open(t, path)
	client2 := newRecorder()
	a2 := c2.Connect(client2)
	check(a2)
	openSession(t, a2, first)
	stub(t, c2, first, reply("再来", nil))
	sayAndWait(t, a2, client2, first, "又一条")
	check(a2)
}
