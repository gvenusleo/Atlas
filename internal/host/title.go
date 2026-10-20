package host

import (
	"context"
	"slices"
	"strings"

	"github.com/gvenusleo/atlas/internal/agent"
)

// titleRunes 是自动生成的会话名称的最大字数。
const titleRunes = 30

// autoName 在会话尚无名称时，用首条用户消息的开头命名；消息已保存后调用。
func (s *Session) autoName(ctx context.Context, message agent.UserMessage) error {
	isUser := func(m agent.Message) bool {
		_, ok := m.(agent.UserMessage)
		return ok
	}
	if s.State().Name != "" || slices.IndexFunc(s.loop.Messages, isUser) != len(s.loop.Messages)-1 {
		return nil
	}
	title := titleOf(message.Content)
	if title == "" {
		return nil
	}
	if err := s.manager.store.Rename(ctx, s.id, title); err != nil {
		return err
	}
	s.mu.Lock()
	s.state.Name = title
	s.mu.Unlock()
	return nil
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
