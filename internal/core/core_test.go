package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// recorder 是记下收到的更新的客户端。
type recorder struct {
	mu      sync.Mutex
	updates []protocol.SessionUpdate
}

func (r *recorder) SessionUpdate(n protocol.SessionNotification) {
	r.mu.Lock()
	r.updates = append(r.updates, n.Update)
	r.mu.Unlock()
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
	return slices.ContainsFunc(r.updates, func(u protocol.SessionUpdate) bool {
		return u.SessionUpdate() == kind
	})
}

// resume 让 a 打开会话 id。
func resume(t *testing.T, a protocol.Agent, id string) {
	t.Helper()
	_, err := a.ResumeSession(t.Context(), protocol.ResumeSessionRequest{SessionID: id})
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

func say(t *testing.T, a protocol.Agent, id, text string) error {
	t.Helper()
	_, err := a.Prompt(t.Context(), protocol.PromptRequest{
		SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text(text)}})
	return err
}

// 新会话先只在内存里，第一条消息保存时写库并以它命名；客户端只从更新里看到对话，
// 回放的历史与实时收到的是同样的内容。
func TestNewSessionIsSavedWithFirstPrompt(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	client := &recorder{}
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
	if err := say(t, a, resp.SessionID, "你好"); err != nil {
		t.Fatal(err)
	}

	if got := client.texts(); !slices.Equal(got, []string{"用户:你好", "助手:好"}) {
		t.Errorf("收到的对话 = %v", got)
	}
	if !client.has("session_info_update") || !client.has("_atlas_state") {
		t.Error("没有收到标题或运行状态的更新")
	}
	list, err = a.ListSessions(t.Context(), protocol.ListSessionsRequest{})
	if err != nil || len(list.Sessions) != 1 || list.Sessions[0].Title != "你好" {
		t.Fatalf("列表 = %+v（%v），期望一个名为“你好”的会话", list.Sessions, err)
	}
	history, err := a.History(t.Context(),
		protocol.HistoryRequest{SessionID: resp.SessionID, Limit: 10})
	replayed := &recorder{updates: history.Updates}
	if err != nil || !slices.Equal(replayed.texts(), client.texts()) {
		t.Errorf("回放 = %v（%v），期望与实时收到的一样", replayed.texts(), err)
	}
}

// 两个进程打开同一个库：各自加载会话。一方运行到一半时另一方写过，这一方的保存冲突，
// 这一轮以错误结束；下一轮开始前从库里重新加载，通知客户端重新读历史，带着对方的消息继续。
func TestTwoProcessesShareSession(t *testing.T) {
	path := home(t, settingsJSON)
	c1, c2 := open(t, path), open(t, path)
	client1 := &recorder{}
	a1, a2 := c1.Connect(client1), c2.Connect(&recorder{})
	resp, err := a1.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	stub(t, c1, id, reply("一", nil))
	if err := say(t, a1, id, "甲"); err != nil {
		t.Fatal(err)
	}
	resume(t, a2, id)
	stub(t, c2, id, reply("二", nil))

	stub(t, c1, id, func(ctx context.Context, req agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		if err := say(t, a2, id, "乙"); err != nil { // 另一个进程在这一轮中途写了这个会话
			t.Error(err)
		}
		return reply("三", nil)(ctx, req, emit)
	})
	if err := say(t, a1, id, "丙"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("中途被别处写过的一轮 = %v，期望 ErrConflict", err)
	}

	var seen int
	stub(t, c1, id, reply("四", &seen))
	if err := say(t, a1, id, "丁"); err != nil {
		t.Fatal(err)
	}
	if !client1.has("_atlas_reset") {
		t.Error("重新加载之后没有通知客户端")
	}
	// 上下文：甲、一、丙（保存成功的用户消息）、乙、二，加上这一轮的丁。
	if seen != 6 {
		t.Errorf("重新加载后的请求有 %d 条消息，期望 6 条", seen)
	}
}

// 会话属于打开它的连接：没打开的连接不能对它发 Prompt。同一进程里两个连接打开同一个会话，
// 都收到更新；运行中另一个 Prompt 返回 ErrBusy，取消后这一轮以 cancelled 结束。
// 删除不存在的会话静默成功。
func TestClientsShareSession(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	client1, client2 := &recorder{}, &recorder{}
	a1, a2 := c.Connect(client1), c.Connect(client2)
	resp, err := a1.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	stub(t, c, id, reply("好", nil))
	if err := say(t, a1, id, "你好"); err != nil {
		t.Fatal(err)
	}
	if err := say(t, a2, id, "没打开就发"); !errors.Is(err, protocol.ErrNotFound) {
		t.Fatalf("没打开会话的连接 Prompt = %v，期望 ErrNotFound", err)
	}
	resume(t, a2, id)

	entered := make(chan struct{})
	stub(t, c, id, func(ctx context.Context, _ agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		emit(agent.Delta{Type: agent.DeltaTypeText, Text: "想"})
		close(entered)
		<-ctx.Done()
		return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
	})
	done := make(chan protocol.PromptResponse, 1)
	go func() {
		resp, _ := a1.Prompt(context.Background(), protocol.PromptRequest{
			SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text("再想想")}})
		done <- resp
	}()
	<-entered
	if err := say(t, a2, id, "插一句"); !errors.Is(err, protocol.ErrBusy) {
		t.Fatalf("运行中的 Prompt = %v，期望 ErrBusy", err)
	}
	a2.Cancel(protocol.CancelNotification{SessionID: id})
	select {
	case resp := <-done:
		if resp.StopReason != protocol.StopCancelled {
			t.Errorf("被取消的一轮 = %q，期望 cancelled", resp.StopReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消之后这一轮没有结束")
	}
	want := []string{"用户:你好", "助手:好", "用户:再想想", "助手:想"}
	if got := client2.texts(); !slices.Equal(got, want[2:]) {
		t.Errorf("后打开的连接收到 %v，期望 %v", got, want[2:])
	}
	if got := client1.texts(); !slices.Equal(got, want) {
		t.Errorf("先打开的连接收到 %v，期望 %v", got, want)
	}
	missing := protocol.DeleteSessionRequest{SessionID: "0190f0f0-0000-7000-8000-000000000001"}
	if err := a1.DeleteSession(t.Context(), missing); err != nil {
		t.Errorf("删除不存在的会话 = %v，期望静默成功", err)
	}
}

// 从 settings.json 里删掉会话上次用的模型之后，会话照常打开，退到默认模型并存下来。
func TestResumeFallsBackWhenModelIsGone(t *testing.T) {
	path := home(t, settingsJSON)
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	id := "0190f0f0-0000-7000-8000-000000000000"
	_, err = db.Create(t.Context(), store.Info{ID: id, Cwd: t.TempDir(), Model: "p/gone",
		Thinking: agent.ThinkingHigh}, snapshot("提示词"))
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	c := open(t, path)
	resp, err := c.Connect(&recorder{}).ResumeSession(t.Context(),
		protocol.ResumeSessionRequest{SessionID: id})
	if err != nil {
		t.Fatalf("ResumeSession = %v，期望模型被删也能打开", err)
	}
	if model := resp.ConfigOptions[0]; model.CurrentValue != "p/a" ||
		resp.ConfigOptions[1].CurrentValue != "high" {
		t.Errorf("配置项 = %+v，期望退到默认模型 p/a，保留 high", resp.ConfigOptions)
	}
	if info, err := c.db.Get(t.Context(), id); err != nil || info.Model != "p/a" {
		t.Errorf("会话表里的模型 = %q（%v），期望存下退到的 p/a", info.Model, err)
	}
}
