package core

import (
	"errors"
	"slices"
	"strings"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
)

func checkQueuedPrompt(blocks []protocol.ContentBlock) error {
	switch strings.TrimSpace(agent.TextOf(userMessage(blocks).Content)) {
	case "":
		return errors.New("请输入消息或添加图片")
	case "/compact":
		return errors.New("请在会话空闲时发送 /compact")
	}
	return nil
}

func (s *session) enqueuePrompt(req protocol.SendPromptRequest) error {
	if err := checkQueuedPrompt(req.Prompt); err != nil {
		return err
	}
	s.mu.Lock()
	if s.runState.Invalidated {
		s.mu.Unlock()
		return protocol.ErrNotFound
	}
	if s.busy == nil || s.busy.turn == nil || s.busy.turn.id != req.RunID ||
		!s.busy.turn.accepting || s.busy.turn.ctx.Err() != nil {
		s.mu.Unlock()
		return protocol.ErrTurnChanged
	}
	if req.Delivery == protocol.PromptInTurn && s.busy.turn.kind == protocol.RunCompaction {
		s.mu.Unlock()
		return errors.New("手动压缩期间请排到下一轮")
	}
	id := s.core.versions.Add(1)
	item := protocol.QueuedPrompt{ID: id, Version: id, Delivery: req.Delivery,
		Prompt: slices.Clone(req.Prompt)}
	if req.Delivery == protocol.PromptInTurn {
		s.queue.Items = slices.Insert(s.queue.Items, s.followUpIndex(), item)
	} else {
		s.queue.Items = append(s.queue.Items, item)
	}
	update := s.queueChanged()
	s.mu.Unlock()
	s.broadcast(update)
	return nil
}

func (s *session) changeQueuedPrompt(id, version uint64, prompt *[]protocol.ContentBlock) error {
	if prompt != nil {
		if err := checkQueuedPrompt(*prompt); err != nil {
			return err
		}
	}
	s.mu.Lock()
	if s.runState.Invalidated {
		s.mu.Unlock()
		return protocol.ErrNotFound
	}
	i := s.queuedPromptIndex(id)
	if i < 0 || s.queue.Items[i].Sending || s.queue.Items[i].Version != version {
		s.mu.Unlock()
		return protocol.ErrQueuedPromptChanged
	}
	if prompt == nil {
		s.queue.Items = slices.Delete(s.queue.Items, i, i+1)
	} else {
		s.queue.Items[i].Prompt = slices.Clone(*prompt)
		s.queue.Items[i].Version = s.core.versions.Add(1)
	}
	update := s.queueChanged()
	s.mu.Unlock()
	s.broadcast(update)
	return nil
}

func (s *session) resumePromptQueue() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.mu.Lock()
	if s.runState.Invalidated {
		s.mu.Unlock()
		return protocol.ErrNotFound
	}
	if s.busy != nil {
		s.mu.Unlock()
		return protocol.ErrBusy
	}
	if len(s.queue.Items) == 0 {
		s.mu.Unlock()
		return nil
	}
	s.queue.Paused, s.queue.Error = false, ""
	t := s.reserveQueuedTurn()
	run := s.activateTurn(t, protocol.RunStateUpdate{})
	update := s.queueChanged()
	s.mu.Unlock()
	s.broadcast(run, update)
	s.core.working.Go(func() { s.runTurns(t, agent.Input{}, agent.UserMessage{}) })
	return nil
}

// takeTurnPrompts 与提交共用一把锁。确认没有本轮消息后，停止接收任何针对本轮的提交。
func (s *session) takeTurnPrompts(t *turn, ending, atLimit bool) ([]protocol.QueuedPrompt, bool) {
	s.mu.Lock()
	n := s.followUpIndex()
	if t.ctx.Err() != nil || atLimit || (ending && n == 0) {
		t.accepting = false
		s.mu.Unlock()
		return nil, n > 0
	}
	if n == 0 {
		s.mu.Unlock()
		return nil, false
	}
	for i := range n {
		s.queue.Items[i].Sending = true
	}
	items := slices.Clone(s.queue.Items[:n])
	update := s.queueChanged()
	s.mu.Unlock()
	s.broadcast(update)
	return items, true
}

// promptSaved 只在消息已入库后移除队列项。
func (s *session) promptSaved(id uint64) protocol.PromptQueueUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.queuedPromptIndex(id)
	s.queue.Items = slices.Delete(s.queue.Items, i, i+1)
	return s.queueChanged()
}

// pausePromptQueue 在会话锁内收回未写入的项。队列已有的顺序也就是继续发送时的顺序。
func (s *session) pausePromptQueue(reason string) protocol.PromptQueueUpdate {
	for i := range s.queue.Items {
		item := &s.queue.Items[i]
		item.Delivery = protocol.PromptNextTurn
		item.Sending = false
	}
	s.queue.Paused, s.queue.Error = true, reason
	return s.queueChanged()
}

// reserveQueuedTurn 在会话锁内领取队首并占用会话，预检与实际运行在锁外完成。
func (s *session) reserveQueuedTurn() *turn {
	item := &s.queue.Items[0]
	item.Sending = true
	t := s.reserveTurn(protocol.RunConversation)
	t.queued = *item
	return t
}

func (s *session) followUpIndex() int {
	i := slices.IndexFunc(s.queue.Items, func(item protocol.QueuedPrompt) bool {
		return item.Delivery == protocol.PromptNextTurn
	})
	if i < 0 {
		return len(s.queue.Items)
	}
	return i
}

func (s *session) queuedPromptIndex(id uint64) int {
	return slices.IndexFunc(s.queue.Items, func(item protocol.QueuedPrompt) bool {
		return item.ID == id
	})
}

// queueSnapshot 与 queueChanged 由持有会话锁的调用方使用。
func (s *session) queueSnapshot() protocol.PromptQueue {
	queue := s.queue
	queue.Items = slices.Clone(queue.Items)
	return queue
}

func (s *session) queueChanged() protocol.PromptQueueUpdate {
	if len(s.queue.Items) == 0 {
		s.queue.Paused, s.queue.Error = false, ""
	}
	s.queue.Version = s.core.versions.Add(1)
	return protocol.PromptQueueUpdate{PromptQueue: s.queueSnapshot()}
}
