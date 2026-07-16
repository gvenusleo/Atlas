package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/tool"
)

// turnDoneMsg signals that RunTurn has returned.
type turnDoneMsg struct {
	result runtime.TurnResult
	err    error
}

// turnUpdateMsg serializes agent events and turn completion through one channel.
type turnUpdateMsg struct {
	event *agent.Event
	done  *turnDoneMsg
}

// sessionLoadedMsg contains the persisted messages requested at startup.
type sessionLoadedMsg struct {
	messages      []model.Message
	contextTokens int
	err           error
}

// modelStatusLoadedMsg contains the default model information shown in the footer.
type modelStatusLoadedMsg struct {
	modelName       string
	reasoningEffort string
	contextWindow   int
	err             error
}

// chatMessage represents a single rendered message block in the conversation.
type chatMessage struct {
	role      string          // "user" | "assistant"
	content   strings.Builder // accumulated text (streamed for assistant)
	toolCalls []toolCallView  // tool calls within this assistant message
	err       error
	cancelled bool
}

// toolCallView holds the display state of a single tool call.
type toolCallView struct {
	title  string // tool.DisplayTitle result
	result string
	err    bool
	done   bool
}

// newUserMessage creates a chatMessage for a user prompt.
func newUserMessage(text string) *chatMessage {
	m := &chatMessage{role: "user"}
	m.content.WriteString(ansi.Strip(text))
	return m
}

// newAssistantMessage creates an empty chatMessage ready for streaming.
func newAssistantMessage() *chatMessage {
	return &chatMessage{role: "assistant"}
}

// handleEvent updates the message state based on an agent.Event.
func (m *chatMessage) handleEvent(e agent.Event) {
	switch e.Type {
	case agent.EventModelDelta:
		m.content.WriteString(ansi.Strip(e.Content))
	case agent.EventToolStarted:
		m.toolCalls = append(m.toolCalls, toolCallView{
			title: ansi.Strip(tool.DisplayTitle(e.ToolCall)),
		})
	case agent.EventToolFinished:
		if len(m.toolCalls) > 0 {
			tc := &m.toolCalls[len(m.toolCalls)-1]
			tc.result = ansi.Strip(e.ToolResult)
			tc.err = e.ToolError
			tc.done = true
		}
	}
}

// render produces the styled string for this message block.
func (m *chatMessage) render(width int, hasDarkBackground bool) string {
	switch m.role {
	case "user":
		content := renderIndented(m.content.String(), max(width-2, 1), "› ", messageStyle)
		return userMessageStyle(hasDarkBackground).
			Width(width).
			Render(content)
	case "assistant":
		var parts []string
		if m.content.Len() > 0 {
			parts = append(parts, renderIndented(m.content.String(), width, "• ", messageStyle))
		}
		for _, tc := range m.toolCalls {
			parts = append(parts, renderToolCall(tc, width))
		}
		if m.cancelled {
			parts = append(parts, renderIndented("Cancelled", width, "• ", mutedStyle))
		} else if m.err != nil {
			parts = append(parts, renderIndented(ansi.Strip(m.err.Error()), width, "• ", errorStyle))
		}
		return strings.Join(parts, "\n\n")
	}
	return m.content.String()
}

// renderToolCall renders a single tool call as a styled line.
func renderToolCall(tc toolCallView, width int) string {
	if width <= 0 {
		return ""
	}
	style := toolStyle
	if tc.err {
		style = errorStyle
	} else if tc.done {
		style = assistantStyle
	}
	line := renderIndented(ansi.Strip(tc.title), width, "• ", style)
	if tc.done && tc.result != "" && !tc.err {
		firstLine := tc.result
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		if width > 2 {
			line += "\n" + renderIndented(firstLine, width, "  ", mutedStyle)
		}
	}
	return line
}

func renderIndented(content string, width int, firstPrefix string, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width <= 2 {
		return style.Render(ansi.Truncate(firstPrefix, width, ""))
	}

	wrapped := ansi.Hardwrap(ansi.Strip(content), width-2, true)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		prefix := "  "
		if i == 0 {
			prefix = firstPrefix
		}
		lines[i] = style.Render(prefix + lines[i])
	}
	return strings.Join(lines, "\n")
}

// messagesFromTranscript converts persisted model messages into TUI blocks.
func messagesFromTranscript(messages []model.Message) []*chatMessage {
	var rendered []*chatMessage
	type toolRef struct {
		message *chatMessage
		index   int
	}
	toolRefs := make(map[string]toolRef)

	for _, message := range messages {
		switch message.Role {
		case model.RoleUser:
			text := model.TextFromParts(model.MessageParts(message))
			if text != "" {
				rendered = append(rendered, newUserMessage(text))
			}
		case model.RoleAssistant:
			chat := newAssistantMessage()
			chat.content.WriteString(ansi.Strip(message.Content))
			for _, call := range message.ToolCalls {
				chat.toolCalls = append(chat.toolCalls, toolCallView{
					title: ansi.Strip(tool.DisplayTitle(call)),
				})
				toolRefs[call.ID] = toolRef{message: chat, index: len(chat.toolCalls) - 1}
			}
			if chat.content.Len() > 0 || len(chat.toolCalls) > 0 {
				rendered = append(rendered, chat)
			}
		case model.RoleTool:
			ref, ok := toolRefs[message.ToolCallID]
			if !ok {
				continue
			}
			call := &ref.message.toolCalls[ref.index]
			call.result = ansi.Strip(message.Content)
			call.done = true
		}
	}
	return rendered
}
