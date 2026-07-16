package tui

import (
	"image/color"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"charm.land/glamour/v2"
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
	role          string          // "user" | "assistant"
	content       strings.Builder // accumulated text (streamed for assistant)
	toolCalls     []toolCallView  // tool calls within this assistant message
	err           error
	cancelled     bool
	markdownCache markdownRenderCache
}

// markdownRenderCache avoids rendering unchanged historical messages on every delta.
type markdownRenderCache struct {
	contentLength  int
	width          int
	darkBackground bool
	rendered       string
	valid          bool
}

type markdownRendererKey struct {
	width          int
	darkBackground bool
}

type markdownRendererEntry struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
}

var markdownRenderers sync.Map

const (
	markdownParagraphStart  = "\u2060\u2061"
	markdownParagraphEnd    = "\u2061\u2060"
	markdownBlockquoteStart = "\u2062\u2063"
	markdownBlockquoteEnd   = "\u2063\u2062"
)

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
func (m *chatMessage) render(width int, hasDarkBackground bool, terminalBackground color.Color) string {
	switch m.role {
	case "user":
		content := renderIndented(m.content.String(), max(width-2, 1), "› ", messageStyle)
		return userMessageStyle(hasDarkBackground, terminalBackground).
			Width(width).
			Render(content)
	case "assistant":
		var parts []string
		if m.content.Len() > 0 {
			parts = append(parts, m.renderMarkdown(width, hasDarkBackground))
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

// renderMarkdown returns the cached assistant body when its render inputs are unchanged.
func (m *chatMessage) renderMarkdown(width int, hasDarkBackground bool) string {
	cache := &m.markdownCache
	if cache.valid &&
		cache.contentLength == m.content.Len() &&
		cache.width == width &&
		cache.darkBackground == hasDarkBackground {
		return cache.rendered
	}

	rendered := renderAssistantMarkdown(m.content.String(), width, hasDarkBackground)
	*cache = markdownRenderCache{
		contentLength:  m.content.Len(),
		width:          width,
		darkBackground: hasDarkBackground,
		rendered:       rendered,
		valid:          true,
	}
	return rendered
}

// renderAssistantMarkdown renders one assistant body and applies the message gutter.
func renderAssistantMarkdown(content string, width int, hasDarkBackground bool) string {
	if width <= 2 {
		return renderIndented(content, width, "• ", messageStyle)
	}

	wrapWidth := width - 2
	key := markdownRendererKey{width: wrapWidth, darkBackground: hasDarkBackground}
	entryValue, ok := markdownRenderers.Load(key)
	if !ok {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStyles(markdownStyle(hasDarkBackground)),
			glamour.WithWordWrap(wrapWidth),
			glamour.WithTableWrap(true),
		)
		if err != nil {
			return renderIndented(content, width, "• ", messageStyle)
		}
		entryValue, _ = markdownRenderers.LoadOrStore(key, &markdownRendererEntry{renderer: renderer})
	}

	entry := entryValue.(*markdownRendererEntry)
	entry.mu.Lock()
	rendered, err := entry.renderer.Render(content)
	entry.mu.Unlock()
	if err != nil {
		return renderIndented(content, width, "• ", messageStyle)
	}

	rendered = reflowMarkdownBlockquotes(rendered, wrapWidth)
	rendered = reflowMarkdownParagraphs(rendered, wrapWidth)
	lines := strings.Split(rendered, "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	for i := range lines {
		plain := strings.TrimRight(ansi.Strip(lines[i]), " \t")
		lines[i] = ansi.Truncate(lines[i], ansi.StringWidth(plain), "")
		prefix := "  "
		if i == 0 {
			prefix = "• "
		}
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// reflowMarkdownBlockquotes reflows marked quote blocks before restoring their gutter.
func reflowMarkdownBlockquotes(rendered string, width int) string {
	var result strings.Builder
	for {
		before, rest, found := strings.Cut(rendered, markdownBlockquoteStart)
		result.WriteString(before)
		if !found {
			return result.String()
		}

		blockquote, after, found := strings.Cut(rest, markdownBlockquoteEnd)
		if !found {
			result.WriteString(rest)
			return result.String()
		}
		blockquote = reflowMarkdownParagraphs(blockquote, max(width-2, 1))
		lines := strings.Split(strings.Trim(blockquote, "\n"), "\n")
		for i := range lines {
			lines[i] = "│ " + lines[i]
		}
		result.WriteString(strings.Join(lines, "\n"))
		if strings.HasSuffix(blockquote, "\n") {
			result.WriteByte('\n')
		}
		rendered = after
	}
}

// reflowMarkdownParagraphs replaces Glamour's word wrapping only inside
// paragraph markers, leaving tables and code blocks on their native layout path.
func reflowMarkdownParagraphs(rendered string, width int) string {
	var result strings.Builder
	for {
		before, rest, found := strings.Cut(rendered, markdownParagraphStart)
		result.WriteString(before)
		if !found {
			return result.String()
		}

		paragraph, after, found := strings.Cut(rest, markdownParagraphEnd)
		if !found {
			result.WriteString(rest)
			return result.String()
		}
		result.WriteString(reflowMarkdownParagraph(paragraph, width))
		rendered = after
	}
}

// reflowMarkdownParagraph reconstructs one paragraph before hard wrapping it by cell width.
func reflowMarkdownParagraph(paragraph string, width int) string {
	hasTrailingNewline := strings.HasSuffix(paragraph, "\n")
	rawLines := strings.Split(strings.Trim(paragraph, "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		plain := strings.TrimRight(ansi.Strip(line), " \t")
		lines = append(lines, ansi.Truncate(line, ansi.StringWidth(plain), ""))
	}

	var joined strings.Builder
	for i, line := range lines {
		if i > 0 {
			joined.WriteString(markdownLineJoiner(lines[i-1], line))
		}
		joined.WriteString(line)
	}
	wrapped := ansi.Hardwrap(joined.String(), width, true)
	if hasTrailingNewline {
		wrapped += "\n"
	}
	return wrapped
}

// markdownLineJoiner restores whitespace removed by Glamour's word wrapper.
func markdownLineJoiner(previous, next string) string {
	previous = strings.TrimSpace(ansi.Strip(previous))
	next = strings.TrimSpace(ansi.Strip(next))
	if previous == "" || next == "" {
		return ""
	}
	last, _ := utf8.DecodeLastRuneInString(previous)
	first, _ := utf8.DecodeRuneInString(next)
	if (isCJK(last) && isCJK(first)) || last == '-' || unicode.IsPunct(first) {
		return ""
	}
	return " "
}

// isCJK reports whether a rune belongs to a wide East Asian script.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
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
