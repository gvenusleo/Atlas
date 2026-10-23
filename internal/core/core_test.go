package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

const settingsJSON = `{"model": "p/a", "providers": {"p": {"api": "openai-responses",
	"base_url": "http://x", "models": {"a": {"reasoning": true,
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
	var out []string
	for _, u := range r.updates {
		switch u := u.(type) {
		case protocol.UserMessageChunk:
			out = append(out, "用户:"+u.Content.Text)
		case protocol.AgentMessageChunk:
			out = append(out, "助手:"+u.Content.Text)
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
	stub(t, c, id, reply("好", nil))
	image := protocol.Image("AQID", "image/png")
	if _, err := a.SendPrompt(t.Context(), protocol.SendPromptRequest{
		SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text("看图"), image}}); err != nil {
		t.Fatal(err)
	}
	r.end(t)
	sayAndWait(t, a, r, id, "再说一句")

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
	var seen int
	stub(t, c, id, reply("四", &seen))
	sayAndWait(t, a, r, id, "继续")
	// 上下文：摘要、保留的最后一条回复，加上这一轮的“继续”。
	if seen != 3 {
		t.Errorf("压缩后重新打开的请求有 %d 条消息，期望 3 条", seen)
	}
}
