package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
)

// saveTimeout 限制单条消息的保存耗时；保存不受取消影响。
const saveTimeout = 5 * time.Second

// titleRunes 是自动生成的会话名称的最大字数。
const titleRunes = 30

// drive 是一轮里要跑的：一轮对话或一次压缩。
type drive func(ctx context.Context, in agent.Input, emit func(agent.Event)) []agent.Message

// send 跑一轮。/compact 压缩上下文；/技能名 开头时在原文后附上技能全文。
func (s *session) send(ctx context.Context, blocks []protocol.ContentBlock) (
	protocol.StopReason, error,
) {
	user := userMessage(blocks)
	if strings.TrimSpace(agent.TextOf(user.Content)) == "/compact" {
		s.mu.Lock()
		compactable := agent.Compactable(s.messages)
		s.mu.Unlock()
		if !compactable {
			return "", protocol.ErrNothingToCompact
		}
		return s.run(ctx, agent.Compact)
	}
	s.mu.Lock()
	list := s.skills
	s.mu.Unlock()
	user, err := withSkill(user, list)
	if err != nil {
		return "", err
	}
	return s.run(ctx, func(
		ctx context.Context, in agent.Input, emit func(agent.Event),
	) []agent.Message {
		return agent.Run(ctx, in, user, emit)
	})
}

// run 独占会话跑一轮：先与库对齐，再运行 drive，每条定稿的消息先写库、再并进上下文、最后推给
// 客户端。写库失败时这一轮停止，之后的事件都丢掉；会话本身仍然可用。
func (s *session) run(ctx context.Context, drive drive) (protocol.StopReason, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(s.core.ctx, cancel)()
	end, err := s.begin(cancel)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	since := s.turn.since
	s.mu.Unlock()
	s.broadcast(protocol.StateUpdate{Running: true, Since: since})
	defer func() {
		end()
		s.broadcast(protocol.StateUpdate{})
	}()

	if err := s.ready(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	in := agent.Input{Model: s.model, Thinking: s.thinking, Prompt: s.prompt, Tools: s.tools,
		Messages: s.messages}
	s.mu.Unlock()

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
	switch {
	case saveErr != nil:
		return "", saveErr
	case ctx.Err() != nil:
		return protocol.StopCancelled, nil
	}
	return stopReason(messages)
}

// ready 让会话可以运行：没有可用的模型时再选一次（配置可能已经改好了）；
// 别的进程写过这个会话时，从库里重新加载上下文，并通知客户端重新读历史。
func (s *session) ready(ctx context.Context) error {
	s.mu.Lock()
	ref, thinking, ok, saved, seq := s.model.Ref, s.thinking, s.model.Stream != nil, s.saved, s.seq
	s.mu.Unlock()
	if !ok {
		settings, err := config.Load()
		if err != nil {
			return err
		}
		if ref, thinking, err = pickModel(settings, ref, thinking); err != nil {
			return err
		}
		model, err := modelConfig(settings, ref, thinking)
		if err != nil {
			return err
		}
		if saved {
			if err := s.core.db.SetModel(ctx, s.id, ref, thinking); err != nil {
				return err
			}
		}
		s.mu.Lock()
		s.model, s.thinking = model, thinking
		s.mu.Unlock()
		s.broadcast(protocol.ConfigOptionUpdate{
			ConfigOptions: configOptions(settings, ref, thinking)})
	}
	if !saved {
		return nil
	}
	info, err := s.core.db.Get(ctx, s.id)
	if err != nil || info.LastSeq == seq {
		return err
	}
	info, records, err := s.core.db.Log(ctx, s.id)
	if err != nil {
		return err
	}
	messages, _ := replay(records)
	s.mu.Lock()
	s.messages, s.seq = messages, info.LastSeq
	usage := s.usage()
	s.mu.Unlock()
	s.broadcast(protocol.ResetUpdate{}, protocol.UsageUpdate{Usage: usage})
	return nil
}

// save 写库并把消息并进上下文，返回它的序号。新会话的第一条消息（总是用户消息）连同提示词快照
// 一起建会话，名称取这条消息的开头。保存不受这一轮取消的影响，但有独立的超时。
func (s *session) save(ctx context.Context, m agent.Message) (int64, error) {
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer stop()
	s.mu.Lock()
	saved, seq := s.saved, s.seq
	info := store.Info{ID: s.id, Cwd: s.cwd, Model: s.model.Ref, Thinking: s.thinking}
	prompt := s.prompt
	s.mu.Unlock()
	var err error
	if saved {
		seq, err = s.core.db.Append(ctx, s.id, seq, m)
	} else {
		if user, ok := m.(agent.UserMessage); ok {
			info.Name = titleOf(user.Content)
		}
		info, err = s.core.db.Create(ctx, info, snapshot(prompt), m)
		seq = info.LastSeq
	}
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.saved, s.seq = true, seq
	s.messages = agent.Fold(s.messages, m)
	usage := s.usage()
	s.mu.Unlock()
	if !saved {
		s.broadcast(protocol.SessionInfoUpdate{Title: info.Name})
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
		case "text":
			m.Content = append(m.Content, agent.TextContent{Text: b.Text})
		case "image":
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
