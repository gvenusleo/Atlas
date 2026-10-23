package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
)

// saveTimeout 限制单条消息的保存耗时；保存不受取消影响。
const saveTimeout = 5 * time.Second

// titleRunes 是自动生成的会话标题的最大字数。
const titleRunes = 30

// drive 是一轮里要跑的：一轮对话或一次压缩。
type drive func(ctx context.Context, in agent.Input, emit func(agent.Event)) []agent.Message

// sendPrompt 受理一条用户消息，随后这一轮在会话内异步运行。/compact 压缩上下文；
// /技能名 开头时在原文后附上技能全文。
func (s *session) sendPrompt(blocks []protocol.ContentBlock) error {
	user := userMessage(blocks)
	if strings.TrimSpace(agent.TextOf(user.Content)) == "/compact" {
		s.mu.Lock()
		compactable := agent.Compactable(s.messages)
		s.mu.Unlock()
		if !compactable {
			return protocol.ErrNothingToCompact
		}
		return s.start(agent.Compact)
	}
	s.mu.Lock()
	list := s.skills
	s.mu.Unlock()
	user, err := withSkill(user, list)
	if err != nil {
		return err
	}
	return s.start(func(
		ctx context.Context, in agent.Input, emit func(agent.Event),
	) []agent.Message {
		return agent.Run(ctx, in, user, emit)
	})
}

// start 受理一轮：占用会话，检查工作目录与模型，取一份输入快照；通过之后在新的 goroutine 里
// 运行。这一轮归会话所有：用核心的 ctx，与发起它的请求无关。
func (s *session) start(drive drive) error {
	if err := checkCwd(s.cwd); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(s.core.ctx)
	end, err := s.begin(cancel)
	if err != nil {
		cancel()
		return err
	}
	if err := s.ensureModel(ctx); err != nil {
		end()
		cancel()
		return err
	}
	s.mu.Lock()
	in := agent.Input{Model: s.model, Thinking: s.thinking,
		SystemPrompt: s.systemPrompt, Tools: s.tools, Messages: s.messages}
	since := s.busy.since
	s.mu.Unlock()
	// 调用方的请求还没结束，Close 不会在这里错过它。
	s.core.working.Go(func() { s.run(ctx, cancel, end, in, since, drive) })
	return nil
}

// run 运行一轮：每条定稿的消息先写库、再并进上下文、最后推送给订阅者。写库失败时这一轮停止，
// 之后的事件都丢掉；写入冲突还会让会话失效。结束时释放占用，推送停止原因或出错的原因，
// 没有订阅者的会话随即卸载。
func (s *session) run(
	ctx context.Context, cancel context.CancelFunc, end func(),
	in agent.Input, since time.Time, drive drive,
) {
	defer cancel()
	s.broadcast(protocol.RunStateUpdate{Running: true, Since: since})

	var saveErr error
	tr := &translator{s: s}
	messages := drive(ctx, in, func(e agent.Event) {
		if saveErr != nil {
			return
		}
		var seq int64
		if e.Type == agent.EventMessageEnd {
			if seq, saveErr = s.save(ctx, e.Message); saveErr != nil {
				cancel()
				return
			}
		}
		s.broadcast(tr.event(e, seq)...)
	})
	s.broadcast(tr.finish()...)

	var state protocol.RunStateUpdate
	switch {
	case saveErr != nil:
		state.Error = saveErr.Error()
		if errors.Is(saveErr, protocol.ErrConflict) {
			s.core.forget(s) // 内存里的会话已经过时，需要重新打开
		}
	case ctx.Err() != nil:
		state.StopReason = protocol.StopCancelled
	default:
		reason, err := stopReason(messages)
		state.StopReason = reason
		if err != nil {
			state.Error = err.Error()
		}
	}
	end()
	s.broadcast(state)
	s.core.unloadIfIdle(s)
}

// save 写库并把消息并进上下文，返回它的序号。新会话的第一条消息（总是用户消息）连同系统提示词
// 快照一起建会话，标题取这条消息的开头。保存不受这一轮取消的影响，但有独立的超时。
func (s *session) save(ctx context.Context, m agent.Message) (int64, error) {
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer stop()
	entry, err := encode(m)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	saved, seq := s.saved, s.seq
	info := store.Info{ID: s.id, Cwd: s.cwd, Model: s.model.Ref, Thinking: string(s.thinking)}
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
	usage := s.usage()
	s.mu.Unlock()
	if !saved {
		s.broadcast(protocol.SessionInfoUpdate{Title: info.Title})
	}
	if changesUsage(m) {
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

// withSkill 在用户消息以 /技能名 开头时，在原文后附上技能块，让模型直接得到技能的内容；
// 开头不是技能时原样返回。
func withSkill(m agent.UserMessage, list []skills.Skill) (agent.UserMessage, error) {
	skill, ok := skills.Invoked(list, agent.TextOf(m.Content))
	if !ok {
		return m, nil
	}
	block, err := skills.InvocationBlock(skill)
	if err != nil {
		return m, err
	}
	m.Content = append(m.Content, agent.TextContent{Text: block})
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
