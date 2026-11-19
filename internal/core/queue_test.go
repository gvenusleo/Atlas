package core

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
)

func openPromptState(t *testing.T, client protocol.Service, id string) protocol.SessionState {
	t.Helper()
	resp, err := client.OpenSession(t.Context(), protocol.OpenSessionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	return resp.State
}

func enqueue(t *testing.T, client protocol.Service, id string, runID uint64,
	delivery protocol.PromptDelivery, text string,
) error {
	t.Helper()
	_, err := client.SendPrompt(t.Context(), protocol.SendPromptRequest{
		SessionID: id, RunID: runID, Delivery: delivery,
		Prompt: []protocol.ContentBlock{protocol.Text(text)},
	})
	return err
}

func awaitPromptEvent[T any](t *testing.T, events <-chan T) T {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("等待模型或工具到达指定位置超时")
		var zero T
		return zero
	}
}

// 工具执行期间交错提交两类用户消息，模型先收到完整工具结果及本轮消息。
// 普通回复期间的插入仍留在同一轮，后续消息各自启动新一轮。
func TestPromptQueueDelivery(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	requests, toolStarted, toolRelease, textRelease, nextRelease :=
		stubQueueDelivery(t, c, id)
	if err := say(t, client, id, "开场"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, toolStarted)
	other := c.Connect(newRecorder())
	runID := openPromptState(t, other, id).Run.RunID
	queueDeliveryFixture(t, client, other, id, runID)
	close(toolRelease)
	assertModelMessages(t, requests, "开场")
	assertModelMessages(t, requests, "开场", "工具结果", "B", "D 已编辑")
	if err := enqueue(t, other, id, runID, protocol.PromptInTurn, "E"); err != nil {
		t.Fatal(err)
	}
	close(textRelease)
	assertModelMessages(t, requests, "开场", "工具结果", "B", "D 已编辑", "E")
	assertModelMessages(t, requests, "开场", "工具结果", "B", "D 已编辑", "E", "A")
	err = enqueue(t, client, id, runID, protocol.PromptInTurn, "迟到")
	if !errors.Is(err, protocol.ErrTurnChanged) {
		t.Fatalf("交接后仍受理上一轮的消息：%v", err)
	}
	close(nextRelease)
	assertModelMessages(t, requests, "开场", "工具结果", "B", "D 已编辑", "E", "A", "C")
	if end := r.end(t); end.Error != "" || end.StopReason != protocol.StopEndTurn {
		t.Fatalf("轮次结束错误：%+v", end)
	}
	assertQueueHandoffs(t, r, runID)
	history, err := client.ListUserMessages(t.Context(), protocol.ListUserMessagesRequest{
		SessionID: id,
	})
	if err != nil || len(history.Messages) != 6 {
		t.Fatalf("历史缺少消息或重复保存：%+v，%v", history, err)
	}
}

func stubQueueDelivery(t *testing.T, c *Core, id string) (
	requests chan []string, toolStarted, toolRelease, textRelease, nextRelease chan struct{},
) {
	t.Helper()
	s, _ := c.lookup(id)
	toolStarted, toolRelease = make(chan struct{}), make(chan struct{})
	s.tools = []agent.Tool{{Name: "read", Execute: func(ctx context.Context, _ string,
		_ jsontext.Value,
	) (agent.ToolResult, error) {
		close(toolStarted)
		select {
		case <-toolRelease:
			return agent.ToolResult{Content: []agent.Content{agent.TextContent{Text: "工具结果"}}}, nil
		case <-ctx.Done():
			return agent.ToolResult{}, ctx.Err()
		}
	}}}
	requests = make(chan []string, 8)
	textRelease, nextRelease = make(chan struct{}), make(chan struct{})
	step := 0
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest,
		_ func(agent.Delta),
	) agent.AssistantMessage {
		var seen []string
		for _, m := range req.Messages {
			switch m := m.(type) {
			case agent.UserMessage:
				seen = append(seen, agent.TextOf(m.Content))
			case agent.ToolResultMessage:
				seen = append(seen, agent.TextOf(m.Content))
			}
		}
		requests <- seen
		step++
		switch step {
		case 1:
			return agent.AssistantMessage{StopReason: agent.StopReasonToolUse,
				Content: []agent.Content{agent.ToolCall{ID: "call", Name: "read"}}}
		case 2:
			select {
			case <-textRelease:
			case <-ctx.Done():
			}
		case 4:
			select {
			case <-nextRelease:
			case <-ctx.Done():
			}
		}
		return reply("完成", nil)(ctx, req, nil)
	})
	return requests, toolStarted, toolRelease, textRelease, nextRelease
}

func queueDeliveryFixture(t *testing.T, client, other protocol.Service, id string, runID uint64) {
	t.Helper()
	for i, text := range []string{"A", "B", "C", "D", "将删除"} {
		delivery := protocol.PromptNextTurn
		if i%2 == 1 {
			delivery = protocol.PromptInTurn
		}
		if err := enqueue(t, client, id, runID, delivery, text); err != nil {
			t.Fatal(err)
		}
	}
	items := openPromptState(t, other, id).Queue.Items
	if len(items) != 5 || items[0].Prompt[0].Text != "B" || items[1].Prompt[0].Text != "D" {
		t.Fatalf("队列显示顺序错误：%+v", items)
	}
	changeQueuedFixture(t, other, id, items[1], items[4])
}

func changeQueuedFixture(
	t *testing.T, client protocol.Service, id string, edit, remove protocol.QueuedPrompt,
) {
	t.Helper()
	update := protocol.UpdateQueuedPromptRequest{SessionID: id, ID: edit.ID, Version: edit.Version,
		Prompt: []protocol.ContentBlock{protocol.Text("D 已编辑")}}
	if err := client.UpdateQueuedPrompt(t.Context(), update); err != nil {
		t.Fatal(err)
	}
	err := client.UpdateQueuedPrompt(t.Context(), update)
	if !errors.Is(err, protocol.ErrQueuedPromptChanged) {
		t.Fatalf("过时的编辑覆盖了新版本：%v", err)
	}
	if err := client.DeleteQueuedPrompt(t.Context(), protocol.DeleteQueuedPromptRequest{
		SessionID: id, ID: remove.ID, Version: remove.Version,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertModelMessages(t *testing.T, requests <-chan []string, want ...string) {
	t.Helper()
	if got := awaitPromptEvent(t, requests); !slices.Equal(got, want) {
		t.Fatalf("模型收到的消息 = %q，期望 %q", got, want)
	}
}

func assertQueueHandoffs(t *testing.T, r *recorder, initial uint64) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var starts, handoffs, stops int
	current := initial
	for _, u := range r.updates {
		s, ok := u.(protocol.RunStateUpdate)
		if !ok {
			continue
		}
		if s.Running && s.PreviousRunID == 0 {
			starts++
			continue
		}
		if s.PreviousRunID != current || s.StopReason != protocol.StopEndTurn {
			t.Fatalf("轮次关联或停止原因错误：%+v", s)
		}
		if s.Running {
			current = s.RunID
			handoffs++
		} else {
			stops++
		}
	}
	if starts != 1 || handoffs != 2 || stops != 1 {
		t.Fatalf("运行状态应有一次开始、两次交接、一次结束，收到 %d、%d、%d", starts, handoffs, stops)
	}
}

type handoffRecorder struct {
	*recorder
	reserved chan struct{}
	release  chan struct{}
}

func (r *handoffRecorder) Receive(n protocol.Notification) {
	r.recorder.Receive(n)
	if u, ok := n.Update.(protocol.PromptQueueUpdate); ok &&
		len(u.Items) == 1 && u.Items[0].Sending {
		close(r.reserved)
		<-r.release
	}
}

// 预留下轮时必须同步公布运行标识，旧轮的停止请求不能作用于新轮。
func TestPromptQueueCancelDuringHandoff(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := &handoffRecorder{recorder: newRecorder(),
		reserved: make(chan struct{}), release: make(chan struct{})}
	releaseHandoff := sync.OnceFunc(func() { close(r.release) })
	t.Cleanup(releaseHandoff)
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	started, finish := make(chan struct{}), make(chan struct{})
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest,
		emit func(agent.Delta),
	) agent.AssistantMessage {
		close(started)
		select {
		case <-finish:
		case <-ctx.Done():
		}
		return reply("完成", nil)(ctx, req, emit)
	})
	if err := say(t, client, id, "开场"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, started)
	first := openPromptState(t, client, id).Run.RunID
	if err := enqueue(t, client, id, first, protocol.PromptNextTurn, "之后"); err != nil {
		t.Fatal(err)
	}
	close(finish)
	awaitPromptEvent(t, r.reserved)
	assertHandoffCancellation(t, client, r, id, first, releaseHandoff)
}

func assertHandoffCancellation(t *testing.T, client protocol.Service, r *handoffRecorder,
	id string, first uint64, release func(),
) {
	t.Helper()
	next := openPromptState(t, client, id).Run
	if !next.Running || next.RunID == first || next.PreviousRunID != first ||
		next.StopReason != protocol.StopEndTurn {
		t.Fatalf("交接时没有同步公布下一轮及上一轮结果：%+v", next)
	}
	if err := client.CancelTurn(t.Context(), protocol.CancelTurnRequest{
		SessionID: id, RunID: first,
	}); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(t, client, id, next.RunID, protocol.PromptNextTurn, "再之后"); err != nil {
		t.Fatalf("旧轮的停止请求影响了新轮：%v", err)
	}
	if err := client.CancelTurn(t.Context(), protocol.CancelTurnRequest{
		SessionID: id, RunID: next.RunID,
	}); err != nil {
		t.Fatal(err)
	}
	release()
	end := r.end(t)
	if end.PreviousRunID != next.RunID || end.StopReason != protocol.StopCancelled {
		t.Fatalf("取消的应是交接后的轮次：%+v", end)
	}
	queue := openPromptState(t, client, id).Queue
	if !queue.Paused || len(queue.Items) != 2 || queue.Items[0].Sending {
		t.Fatalf("取消后没有保留预留的队首：%+v", queue)
	}
}

// 队首图片通过预检后切换模型，首个请求仍须用通过校验的模型发送原图。
func TestQueuedImageUsesPreparedModel(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	s, err := c.lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	var request agent.StreamRequest
	stream := func(
		_ context.Context, req agent.StreamRequest, _ func(agent.Delta),
	) agent.AssistantMessage {
		request = req
		return agent.AssistantMessage{StopReason: agent.StopReasonStop}
	}
	stub(t, c, id, stream)
	if end := sayAndWait(t, client, r, id, "开场"); end.StopReason != protocol.StopEndTurn {
		t.Fatalf("开场没有正常结束：%+v", end)
	}
	image := protocol.Image("AQID", "image/png")
	s.mu.Lock()
	s.model.Images = true
	s.queue.Items = []protocol.QueuedPrompt{{ID: 1, Version: 1, Delivery: protocol.PromptNextTurn,
		Prompt: []protocol.ContentBlock{image}}}
	queuedTurn := s.reserveQueuedTurn()
	s.activateTurn(queuedTurn, protocol.RunStateUpdate{})
	s.mu.Unlock()
	t.Cleanup(queuedTurn.cancel)
	in, user, err := s.prepareTurn(queuedTurn, queuedTurn.queued.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetConfigOption(t.Context(), protocol.SetConfigOptionRequest{
		SessionID: id, ConfigID: protocol.ConfigModel, Value: "p/b",
	}); err != nil {
		t.Fatal(err)
	}
	stub(t, c, id, stream)
	state := s.runTurn(queuedTurn, in, user)
	s.finishTurn(queuedTurn, state)
	if state.StopReason != protocol.StopEndTurn || request.Model != "p/a" ||
		request.ThinkingLevel != agent.ThinkingLow || len(request.Messages) == 0 {
		t.Fatalf("预检后的首个请求使用了 %q、%q，结束状态：%+v",
			request.Model, request.ThinkingLevel, state)
	}
	last, ok := request.Messages[len(request.Messages)-1].(agent.UserMessage)
	if !ok || !slices.ContainsFunc(last.Content, func(content agent.Content) bool {
		img, ok := content.(agent.ImageContent)
		return ok && img.Data == image.Data
	}) {
		t.Fatal("首个请求没有收到队首图片")
	}
}

// 同批插入中途写入失败时，继续发送从失败处开始。
func TestPromptQueueKeepsUnwrittenMessages(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	started, release := make(chan struct{}, 1), make(chan struct{})
	stub(t, c, id, func(ctx context.Context, req agent.StreamRequest,
		_ func(agent.Delta),
	) agent.AssistantMessage {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return reply("完成", nil)(ctx, req, nil)
	})
	s, _ := c.lookup(id)
	s.mu.Lock()
	s.model.Images = true
	s.mu.Unlock()
	if err := say(t, client, id, "开场"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, started)
	runID := openPromptState(t, client, id).Run.RunID
	if err := enqueue(t, client, id, runID, protocol.PromptInTurn, "B"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendPrompt(t.Context(), protocol.SendPromptRequest{
		SessionID: id, RunID: runID, Delivery: protocol.PromptInTurn,
		Prompt: []protocol.ContentBlock{protocol.Image("!", "image/png")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(t, client, id, runID, protocol.PromptNextTurn, "A"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if end := r.end(t); !strings.Contains(end.Error, "base64") || end.Invalidated {
		t.Fatalf("错误写库后的运行结果：%+v", end)
	}
	queue := openPromptState(t, client, id).Queue
	if !queue.Paused || len(queue.Items) != 2 || queue.Items[0].Sending ||
		queue.Items[0].Prompt[0].Type != protocol.ContentImage ||
		queue.Items[1].Prompt[0].Text != "A" {
		t.Fatalf("写入失败后没有保留未发送消息：%+v", queue)
	}
	assertQueueResumeAfterEdit(t, c, client, r, id, queue.Items[0])
}

func assertQueueResumeAfterEdit(t *testing.T, c *Core, client protocol.Service, r *recorder,
	id string, item protocol.QueuedPrompt,
) {
	t.Helper()
	if err := client.UpdateQueuedPrompt(t.Context(), protocol.UpdateQueuedPromptRequest{
		SessionID: id, ID: item.ID, Version: item.Version,
		Prompt: []protocol.ContentBlock{protocol.Text("D")},
	}); err != nil {
		t.Fatal(err)
	}
	stub(t, c, id, reply("完成", nil))
	resume := protocol.ResumePromptQueueRequest{SessionID: id}
	if err := client.ResumePromptQueue(t.Context(), resume); err != nil {
		t.Fatal(err)
	}
	r.end(t)
	history, err := client.ListUserMessages(t.Context(), protocol.ListUserMessagesRequest{
		SessionID: id,
	})
	var texts []string
	for _, message := range history.Messages {
		texts = append(texts, message.Text)
	}
	if err != nil || !slices.Equal(texts, []string{"开场", "B", "D", "A"}) {
		t.Fatalf("写库失败后继续发送的历史 = %q，%v", texts, err)
	}
}

func TestPromptQueuePausesOnCancel(t *testing.T) {
	c := open(t, home(t, settingsJSON))
	r := newRecorder()
	client := c.Connect(r)
	resp, err := client.NewSession(t.Context(), protocol.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.SessionID
	started := make(chan struct{})
	stub(t, c, id, func(ctx context.Context, _ agent.StreamRequest,
		_ func(agent.Delta),
	) agent.AssistantMessage {
		close(started)
		<-ctx.Done()
		return agent.AssistantMessage{StopReason: agent.StopReasonCanceled}
	})
	if err := say(t, client, id, "开场"); err != nil {
		t.Fatal(err)
	}
	awaitPromptEvent(t, started)
	runID := openPromptState(t, client, id).Run.RunID
	if err := enqueue(t, client, id, runID, protocol.PromptNextTurn, "A"); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(t, client, id, runID, protocol.PromptInTurn, "B"); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelTurn(t.Context(), protocol.CancelTurnRequest{
		SessionID: id,
	}); err != nil {
		t.Fatal(err)
	}
	if end := r.end(t); end.StopReason != protocol.StopCancelled {
		t.Fatalf("停止原因错误：%+v", end)
	}
	if err := client.CloseSession(t.Context(), protocol.CloseSessionRequest{
		SessionID: id,
	}); err != nil {
		t.Fatal(err)
	}
	queue := openPromptState(t, client, id).Queue
	if !queue.Paused || len(queue.Items) != 2 || queue.Items[0].Prompt[0].Text != "B" ||
		queue.Items[0].Sending || queue.Items[0].Delivery != protocol.PromptNextTurn {
		t.Fatalf("暂停时没有按顺序保留消息：%+v", queue)
	}
	stub(t, c, id, reply("完成", nil))
	resume := protocol.ResumePromptQueueRequest{SessionID: id}
	if err := client.ResumePromptQueue(t.Context(), resume); err != nil {
		t.Fatal(err)
	}
	r.end(t)
	want := []string{"用户:开场", "用户:B", "助手:完成", "用户:A", "助手:完成"}
	if got := r.texts(); !slices.Equal(got, want) {
		t.Fatalf("继续后的对话 = %q", got)
	}
	if q := openPromptState(t, client, id).Queue; q.Paused || len(q.Items) != 0 {
		t.Fatalf("发送后没有清空队列：%+v", q)
	}
}
