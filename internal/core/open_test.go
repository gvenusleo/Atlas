package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

func TestOpenDuringReplyReturnsHistoryBoundaryAndFinalText(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	first := c.Connect(newRecorder())
	created, err := first.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := created.SessionID
	prefix, resume := make(chan struct{}), make(chan struct{})
	stub(t, c, id, func(ctx context.Context, _ agent.StreamRequest, emit func(agent.Delta)) agent.AssistantMessage {
		emit(agent.Delta{Type: agent.DeltaTypeText, Text: "开头"})
		close(prefix)
		select {
		case <-ctx.Done():
			return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
		case <-resume:
		}
		emit(agent.Delta{Type: agent.DeltaTypeText, Text: "结尾"})
		return agent.AssistantMessage{StopReason: agent.StopReasonStop,
			Content: []agent.Content{agent.TextContent{Text: "开头结尾"}}}
	})
	if err := say(t, first, id, "问题"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, prefix)
	recorder := newRecorder()
	second := c.Connect(recorder)
	opened, err := second.OpenSession(t.Context(), protocol.OpenSessionRequest{SessionID: id})
	if err != nil || opened.LastSeq != 2 || !opened.State.Run.Running || opened.State.Epoch == 0 {
		t.Fatalf("订阅边界错误：%+v，%v", opened, err)
	}
	close(resume)
	if end := recorder.end(t); end.Error != "" {
		t.Fatalf("会话未正常结束：%+v", end)
	}
	history, err := second.ListHistory(t.Context(), protocol.ListHistoryRequest{
		SessionID: id, Before: opened.LastSeq + 1, Limit: 20,
	})
	if err != nil || len(history.Updates) != 1 {
		t.Fatalf("边界内的历史只能有用户消息：%+v，%v", history, err)
	}
	if text := strings.Join(recorder.texts(), ""); text != "助手:开头结尾" {
		t.Fatalf("后续订阅未收到完整定稿：%q", text)
	}
}

func TestSessionLifecycleWaitsOnlyForSameID(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	first := c.Connect(newRecorder())
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	blocked, err := c.lockSession(ctx, "loading")
	if err != nil {
		t.Fatal(err)
	}
	defer blocked()
	info := store.Info{ID: "available", Cwd: t.TempDir()}
	if _, err := c.db.Create(ctx, info); err != nil {
		t.Fatal(err)
	}
	results := make(chan protocol.OpenSessionResponse, 2)
	failures := make(chan error, 2)
	for range 2 {
		go func() {
			client := c.Connect(newRecorder())
			resp, err := client.OpenSession(ctx, protocol.OpenSessionRequest{SessionID: info.ID})
			if err == nil {
				err = client.CloseSession(ctx, protocol.CloseSessionRequest{SessionID: info.ID})
			}
			results <- resp
			failures <- err
		}()
	}
	for range 2 {
		if resp, err := awaitPromptEvent(t, results), awaitPromptEvent(t, failures); err != nil || resp.LastSeq != 1 {
			t.Fatalf("其他会话被阻塞或重复写入快照：%+v，%v", resp, err)
		}
	}
	waiting, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	_, err = first.OpenSession(waiting, protocol.OpenSessionRequest{SessionID: "loading"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("同一会话的打开应等到超时：%v", err)
	}
}
