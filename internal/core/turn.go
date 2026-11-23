package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
)

// saveTimeout 限制单条消息的保存耗时；保存不受取消影响。
const saveTimeout = 5 * time.Second

// maxRequests 是一轮最多请求模型的次数：模型反复调用失败的工具时，由它拦住失控的循环。
const maxRequests = 200

// titleRunes 是自动生成的会话标题的最大字数。
const titleRunes = 30

type turn struct {
	id        uint64
	kind      protocol.RunKind
	since     time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	accepting bool // 由会话锁保护
	queued    protocol.QueuedPrompt
}

func (s *session) sendPrompt(req protocol.SendPromptRequest) error {
	switch req.Delivery {
	case protocol.PromptInTurn, protocol.PromptNextTurn:
		return s.enqueuePrompt(req)
	case "":
		return s.startTurn(req.Prompt)
	default:
		return fmt.Errorf("不认识的消息投递方式 %q", req.Delivery)
	}
}

func (s *session) startTurn(blocks []protocol.ContentBlock) error {
	kind := protocol.RunConversation
	if strings.TrimSpace(agent.TextOf(userMessage(blocks).Content)) == "/compact" {
		kind = protocol.RunCompaction
	}
	s.mu.Lock()
	switch {
	case s.runState.Invalidated:
		s.mu.Unlock()
		return protocol.ErrNotFound
	case s.busy != nil:
		s.mu.Unlock()
		return protocol.ErrBusy
	}
	t := s.reserveTurn(kind)
	s.mu.Unlock()
	in, user, err := s.prepareTurn(t, blocks)
	if err != nil {
		t.cancel()
		s.mu.Lock()
		s.busy = nil
		s.mu.Unlock()
		s.core.unloadIfIdle(s)
		return err
	}
	// 客户端收到受理响应后即可发下一条消息，此前须公布本轮的标识。
	s.publishTurnStart(t)
	s.core.working.Go(func() { s.runTurns(t, in, user) })
	return nil
}

// reserveTurn 在会话锁内完成占用。运行随会话而非请求取消，交接时可直接换成下一轮。
func (s *session) reserveTurn(kind protocol.RunKind) *turn {
	ctx, cancel := context.WithCancel(s.core.ctx)
	t := &turn{id: s.core.versions.Add(1), kind: kind, since: time.Now(), ctx: ctx, cancel: cancel}
	s.busy = &busy{turn: t}
	return t
}

func (s *session) prepareTurn(
	t *turn, blocks []protocol.ContentBlock,
) (agent.Input, agent.UserMessage, error) {
	if err := checkCwd(s.cwd); err != nil {
		return agent.Input{}, agent.UserMessage{}, err
	}
	if err := s.ensureModel(t.ctx); err != nil {
		return agent.Input{}, agent.UserMessage{}, err
	}
	s.mu.Lock()
	in := agent.Input{Model: s.model, Thinking: s.thinking,
		SystemPrompt: s.systemPrompt, Tools: s.tools, Messages: s.messages,
		MaxRequests: maxRequests, ReadModel: s.readModel}
	s.mu.Unlock()
	if t.kind == protocol.RunCompaction {
		if !agent.Compactable(in.Messages, in.Model.ContextWindow) {
			return agent.Input{}, agent.UserMessage{}, protocol.ErrNothingToCompact
		}
		return in, agent.UserMessage{}, t.ctx.Err()
	}
	checkImages := t.queued.ID != 0 && !in.Model.Images
	user, err := s.prepareUserPrompt(blocks, checkImages)
	if err == nil {
		err = t.ctx.Err()
	}
	return in, user, err
}

func (s *session) prepareUserPrompt(
	blocks []protocol.ContentBlock, checkImages bool,
) (agent.UserMessage, error) {
	if checkImages {
		for _, block := range blocks {
			if block.Type == protocol.ContentImage {
				return agent.UserMessage{}, errors.New("当前模型不接受图片")
			}
		}
	}
	user := userMessage(blocks)
	return withSkills(user, skills.Invoked(s.skills, agent.TextOf(user.Content)))
}

// runTurns 串行交接已预留的轮次。下一轮的预检失败时，队首仍留在队列中。
func (s *session) runTurns(t *turn, in agent.Input, user agent.UserMessage) {
	defer s.core.unloadIfIdle(s)
	for t != nil {
		if t.queued.ID != 0 {
			var err error
			in, user, err = s.prepareTurn(t, t.queued.Prompt)
			if err != nil {
				s.finishTurn(t, turnResult(t, nil, false, err, nil))
				return
			}
		}
		t = s.finishTurn(t, s.runTurn(t, in, user))
	}
}

func (s *session) publishTurnStart(t *turn) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.mu.Lock()
	state := s.activateTurn(t, protocol.RunStateUpdate{})
	s.mu.Unlock()
	s.broadcast(state)
}

// activateTurn 的调用方持有会话锁。预留下轮时同时更新状态，取消与提交看到同一个目标。
func (s *session) activateTurn(t *turn, previous protocol.RunStateUpdate) protocol.RunStateUpdate {
	t.accepting = true
	s.runState = protocol.RunStateUpdate{Version: s.core.versions.Add(1), Running: true,
		RunID: t.id, Kind: t.kind, Since: t.since,
		PreviousRunID: previous.PreviousRunID, StopReason: previous.StopReason}
	return s.runState
}

func (s *session) finishTurn(t *turn, state protocol.RunStateUpdate) *turn {
	if state.Invalidated {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.mu.Lock()
	if t.ctx.Err() != nil && state.Error == "" {
		state.StopReason = protocol.StopCancelled
	}
	t.accepting = false
	t.cancel()
	normal := state.Error == "" && state.StopReason == protocol.StopEndTurn
	var queue *protocol.PromptQueueUpdate
	if len(s.queue.Items) != 0 {
		if !normal {
			queue = new(s.pausePromptQueue(state.Error))
		} else if !s.queue.Paused {
			next := s.reserveQueuedTurn()
			run := s.activateTurn(next, state)
			update := s.queueChanged()
			s.mu.Unlock()
			s.broadcast(run, update)
			return next
		}
	}
	s.busy = nil
	state.Version = s.core.versions.Add(1)
	s.runState = state
	s.mu.Unlock()
	if state.Invalidated {
		s.core.forget(s)
	}
	if queue != nil {
		s.broadcast(*queue)
	}
	s.broadcast(state)
	return nil
}

// runTurn 的队列项在对应的用户消息写库后才移出。失败时取消循环，剩余项由 finishTurn 恢复。
func (s *session) runTurn(
	t *turn, in agent.Input, user agent.UserMessage,
) protocol.RunStateUpdate {
	var runErr error
	var pendingIDs []uint64
	if t.queued.ID != 0 {
		pendingIDs = append(pendingIDs, t.queued.ID)
	}
	in.PollPrompts = func(acceptsImages, ending, atLimit bool) ([]agent.UserMessage, bool) {
		items, waiting := s.takeTurnPrompts(t, ending, atLimit)
		var prompts []agent.UserMessage
		for _, item := range items {
			prompt, err := s.prepareUserPrompt(item.Prompt, !acceptsImages)
			if err != nil {
				runErr = err
				t.cancel()
				return nil, waiting
			}
			pendingIDs = append(pendingIDs, item.ID)
			prompts = append(prompts, prompt)
		}
		return prompts, waiting
	}
	limited := false
	tr := &translator{s: s}
	emit := func(e agent.Event) {
		if runErr != nil {
			return
		}
		limited = limited || e.Type == agent.EventLimit
		var seq int64
		var queue *protocol.PromptQueueUpdate
		if e.Type == agent.EventMessageEnd {
			if seq, runErr = s.save(t.ctx, e.Message); runErr != nil {
				t.cancel()
				return
			}
			if _, ok := e.Message.(agent.UserMessage); ok && len(pendingIDs) != 0 {
				queue = new(s.promptSaved(pendingIDs[0]))
				pendingIDs = pendingIDs[1:]
			}
		}
		s.broadcastRecord(seq, tr.event(e, seq)...)
		if queue != nil {
			s.broadcast(*queue)
		}
	}
	messages, crashed := s.guarded(func() []agent.Message {
		defer func() { s.broadcast(tr.finish()...) }()
		if t.ctx.Err() != nil && t.queued.ID != 0 {
			return nil
		}
		if t.kind == protocol.RunCompaction {
			return agent.Compact(t.ctx, in, emit)
		}
		return agent.Run(t.ctx, in, user, emit)
	})
	return turnResult(t, messages, limited, runErr, crashed)
}

func turnResult(
	t *turn, messages []agent.Message, limited bool, runErr error, crashed any,
) protocol.RunStateUpdate {
	state := protocol.RunStateUpdate{PreviousRunID: t.id}
	switch {
	case crashed != nil:
		state.Error, state.Invalidated = fmt.Sprintf("内部错误：%v", crashed), true
	case errors.Is(runErr, context.Canceled):
		state.StopReason = protocol.StopCancelled
	case runErr != nil:
		state.Error, state.Invalidated = runErr.Error(), invalidSessionError(runErr)
	case t.ctx.Err() != nil:
		state.StopReason = protocol.StopCancelled
	case limited:
		state.StopReason = protocol.StopMaxTurnRequests
	default:
		if t.kind == protocol.RunCompaction {
			for _, message := range messages {
				if m, ok := message.(agent.SystemMessage); ok &&
					m.Type == agent.SystemMessageTypeNotice {
					state.Error = m.Content
					return state
				}
			}
		}
		reason, err := stopReason(messages)
		state.StopReason = reason
		if err != nil {
			state.Error = err.Error()
		}
	}
	return state
}

func invalidSessionError(err error) bool {
	return errors.Is(err, protocol.ErrConflict) || errors.Is(err, protocol.ErrNotFound)
}

// guarded 运行一轮：事件的转换与保存、订阅者的 Receive、模型适配里的 panic 不让整个进程
// 退出，记下现场后作为 crashed 返回。工具的 panic 在 agent 里已经收敛为错误结果。
func (s *session) guarded(run func() []agent.Message) (messages []agent.Message, crashed any) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("会话 %s 的一轮 panic: %v\n%s", s.id, r, debug.Stack())
			crashed = r
		}
	}()
	return run(), nil
}

// save 写库并把消息并进上下文，返回它的序号。新会话的第一条消息（总是用户消息）连同系统提示词
// 快照一起建会话，标题取这条消息的开头。保存不受这一轮取消的影响，但有独立的超时。
func (s *session) save(ctx context.Context, m agent.Message) (int64, error) {
	entry, err := encode(m)
	if err != nil {
		return 0, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer stop()
	s.mu.Lock()
	saved, seq := s.saved, s.seq
	info := store.Info{ID: s.id, Cwd: s.cwd, Model: s.model.Ref, Thinking: string(s.thinking),
		Type: string(s.sessionType), OwnerID: s.ownerID}
	systemPrompt := s.systemPrompt
	if sm, ok := m.(agent.SystemMessage); ok && sm.Type == agent.SystemMessageTypeCompaction {
		entry.ContextFrom = s.contextFrom(sm)
	}
	s.mu.Unlock()
	if saved {
		seq, err = s.core.db.Append(ctx, s.id, seq, entry)
	} else {
		var snapshot store.Entry
		if snapshot, err = encode(systemPromptSnapshot(systemPrompt)); err != nil {
			return 0, err
		}
		if user, ok := m.(agent.UserMessage); ok {
			info.Title = titleOf(user.Content)
		}
		info, err = s.core.db.Create(ctx, info, snapshot, entry)
		seq = info.LastSeq
	}
	if err != nil {
		return 0, mapStoreError(err)
	}
	s.mu.Lock()
	s.saved, s.seq = true, seq
	s.fold(m, seq)
	var usage protocol.Usage
	usageChanged := changesUsage(m)
	if usageChanged {
		usage = s.usage()
	}
	s.mu.Unlock()
	if !saved {
		s.broadcast(protocol.SessionInfoUpdate{Title: info.Title})
	}
	if usageChanged {
		s.broadcast(protocol.UsageUpdate{Usage: usage})
	}
	return seq, nil
}

// changesUsage 报告消息会不会改变上下文占用的读数：助手回复带来新的用量，压缩换掉了上下文。
func changesUsage(m agent.Message) bool {
	switch m := m.(type) {
	case agent.AssistantMessage:
		return true
	case agent.SystemMessage:
		return m.Type == agent.SystemMessageTypeCompaction
	}
	return false
}

// stopReason 按最后一条助手消息给出这一轮的结束原因；模型调用失败返回错误。
func stopReason(messages []agent.Message) (protocol.StopReason, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		last, ok := messages[i].(agent.AssistantMessage)
		if !ok {
			continue
		}
		switch last.StopReason {
		case agent.StopReasonError:
			return "", fmt.Errorf("模型调用失败: %s", last.ErrorMessage)
		case agent.StopReasonCanceled:
			return protocol.StopCancelled, nil
		case agent.StopReasonMaxOutputTokens:
			return protocol.StopMaxTokens, nil
		}
		break
	}
	return protocol.StopEndTurn, nil
}

// userMessage 把客户端给的内容转成用户消息。
func userMessage(blocks []protocol.ContentBlock) agent.UserMessage {
	m := agent.UserMessage{Timestamp: time.Now()}
	for _, b := range blocks {
		switch b.Type {
		case protocol.ContentText:
			m.Content = append(m.Content, agent.TextContent{Text: b.Text})
		case protocol.ContentImage:
			m.Content = append(m.Content, agent.ImageContent{Data: b.Data, MIMEType: b.MimeType})
		}
	}
	return m
}

// withSkills 在原文后附上显式调用的技能块，让模型直接得到技能内容。
func withSkills(m agent.UserMessage, list []skills.Skill) (agent.UserMessage, error) {
	content := m.Content
	for _, skill := range list {
		block, err := skills.InvocationBlock(skill)
		if err != nil {
			return m, err
		}
		content = append(content, agent.TextContent{Text: block})
	}
	m.Content = content
	return m, nil
}

// titleOf 取第一段非空文本的前 titleRunes 个字，空白折叠为单个空格；没有文字只有图片时
// 叫“图片”。
func titleOf(content []agent.Content) string {
	hasImage := false
	for _, block := range content {
		switch b := block.(type) {
		case agent.TextContent:
			if title := []rune(strings.Join(strings.Fields(b.Text), " ")); len(title) > 0 {
				return string(title[:min(len(title), titleRunes)])
			}
		case agent.ImageContent:
			hasImage = true
		}
	}
	if hasImage {
		return "图片"
	}
	return ""
}
