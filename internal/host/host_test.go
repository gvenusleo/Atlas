package host

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/session"
)

// newSession 创建临时数据库和一个已持久化的会话，并以 stream 作为模型调用。
func newSession(t *testing.T, stream agent.StreamFn) (*Manager, *Session) {
	t.Helper()
	m, err := Open(t.Context(), filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })

	id := uuid.NewV7().String()
	info, err := m.store.Create(t.Context(), session.CreateOptions{
		ID: id, Cwd: t.TempDir(), ModelRef: "p/m", SystemPrompt: "提示词",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{manager: m, id: id, seq: info.LastSeq, state: State{ID: id}}
	s.loop = agent.LoopContext{SystemPrompt: "提示词", Model: "m", Stream: stream}
	m.sessions[id] = s
	return m, s
}

// reply 以固定的助手回复替代模型调用。
func reply(context.Context, agent.StreamRequest, func(agent.Delta)) agent.AssistantMessage {
	return agent.AssistantMessage{
		Content:    []agent.Content{agent.TextContent{Text: "好"}},
		Model:      "m",
		StopReason: agent.StopReasonStop,
		Timestamp:  time.Now(),
	}
}

func user() agent.UserMessage {
	return agent.UserMessage{Content: []agent.Content{agent.TextContent{Text: "你好"}}}
}

func TestRunIsExclusiveAndPersists(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	m, s := newSession(t, func(ctx context.Context, req agent.StreamRequest,
		emit func(agent.Delta)) agent.AssistantMessage {
		close(entered)
		<-release
		return reply(ctx, req, emit)
	})
	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context(), user(), nil)
		done <- err
	}()
	<-entered

	if st := s.State(); !st.Busy || !st.Running {
		t.Fatalf("运行中状态不正确: %+v", st)
	}
	if _, err := s.Run(t.Context(), user(), nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("并发 Run = %v，期望 ErrBusy", err)
	}
	if err := m.Delete(t.Context(), s.id); !errors.Is(err, ErrBusy) {
		t.Fatalf("运行中 Delete = %v，期望 ErrBusy", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.WaitIdle(t.Context()); err != nil {
		t.Fatal(err)
	}
	records, err := m.History(t.Context(), s.id, 0, 10)
	if err != nil || len(records) != 3 { // 提示词快照、用户消息、助手消息
		t.Fatalf("历史 = %d 条, err = %v", len(records), err)
	}
}

func TestCancelKeepsSessionUsable(t *testing.T) {
	entered := make(chan struct{})
	_, s := newSession(t, func(ctx context.Context, _ agent.StreamRequest,
		_ func(agent.Delta)) agent.AssistantMessage {
		close(entered)
		<-ctx.Done()
		return agent.AssistantMessage{
			Model: "m", StopReason: agent.StopReasonCanceled, Timestamp: time.Now(),
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context(), user(), nil)
		done <- err
	}()
	<-entered
	s.Cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v，期望 context.Canceled", err)
	}
	if s.State().NeedsReload {
		t.Fatal("取消不应使会话失效")
	}
}

func TestConflictRequiresReload(t *testing.T) {
	m, s := newSession(t, reply)
	if _, err := s.Run(t.Context(), user(), nil); err != nil {
		t.Fatal(err)
	}
	// 模拟另一进程在本实例之后追加了消息。
	notice := agent.SystemMessage{
		Type: agent.SystemMessageTypeNotice, Content: "通知", Timestamp: time.Now(),
	}
	if _, err := m.store.AppendMessage(t.Context(), s.id, s.seq, notice); err != nil {
		t.Fatal(err)
	}

	_, err := s.Run(t.Context(), user(), nil)
	if !errors.Is(err, ErrConflict) || !errors.Is(err, ErrNeedsReload) {
		t.Fatalf("Run = %v，期望 ErrConflict 与 ErrNeedsReload", err)
	}
	if _, err := s.Run(t.Context(), user(), nil); !errors.Is(err, ErrNeedsReload) {
		t.Fatalf("失效后 Run = %v，期望 ErrNeedsReload", err)
	}
}

func TestDeleteAndClose(t *testing.T) {
	m, s := newSession(t, reply)
	if err := m.Delete(t.Context(), s.id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(t.Context(), user(), nil); !errors.Is(err, ErrDeleted) {
		t.Fatalf("删除后 Run = %v，期望 ErrDeleted", err)
	}
	if _, err := m.Get(t.Context(), s.id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 Get = %v，期望 ErrNotFound", err)
	}

	for range 2 { // Close 可重复调用
		if err := m.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.List(t.Context(), "", nil, 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("关闭后 List = %v，期望 ErrClosed", err)
	}
}
