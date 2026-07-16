package tui

import (
	"strings"

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
	messages []model.Message
	err      error
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
func (m *chatMessage) render(width int) string {
	switch m.role {
	case "user":
		return userStyle.Render("▸ ") + m.content.String()
	case "assistant":
		var parts []string
		if m.content.Len() > 0 {
			parts = append(parts, assistantStyle.Render("◂ ")+m.content.String())
		}
		for _, tc := range m.toolCalls {
			parts = append(parts, renderToolCall(tc, width))
		}
		if m.cancelled {
			parts = append(parts, mutedStyle.Render("◂ Cancelled"))
		} else if m.err != nil {
			parts = append(parts, errorStyle.Render("✗ "+ansi.Strip(m.err.Error())))
		}
		return strings.Join(parts, "\n")
	}
	return m.content.String()
}

// renderToolCall renders a single tool call as a styled line.
func renderToolCall(tc toolCallView, width int) string {
	if width <= 0 {
		return ""
	}
	prefix := "⚡ "
	style := toolStyle
	if tc.err {
		prefix = "✗ "
		style = errorStyle
	} else if tc.done {
		prefix = "✓ "
		style = assistantStyle
	}
	line := style.Render(ansi.Truncate(prefix+ansi.Strip(tc.title), width, "…"))
	if tc.done && tc.result != "" && !tc.err {
		firstLine := tc.result
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		resultWidth := max(width-2, 0)
		if resultWidth > 0 {
			line += "\n" + mutedStyle.Render("  "+ansi.Truncate(firstLine, resultWidth, "…"))
		}
	}
	return line
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
