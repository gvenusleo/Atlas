package tui

import (
	"fmt"
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

// compactDoneMsg signals that manual context compaction has returned.
type compactDoneMsg struct {
	result runtime.CompactResult
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

// modelStatusLoadedMsg contains configured models and the initial footer selection.
type modelStatusLoadedMsg struct {
	models          []runtime.ModelOption
	modelValue      string
	modelName       string
	reasoningEffort string
	contextWindow   int
	err             error
}

// skillSummariesLoadedMsg contains skills available as TUI slash commands.
type skillSummariesLoadedMsg struct {
	cwd       string
	summaries []runtime.SkillSummary
	err       error
}

// chatMessage represents a single rendered message block in the conversation.
type chatMessage struct {
	role          string          // "user" | "assistant" | "notice"
	content       strings.Builder // accumulated text (streamed for assistant)
	toolCalls     []toolCallView  // tool calls within this assistant message
	err           error
	cancelled     bool
	noticeError   bool
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
	call     model.ToolCall
	result   string
	metadata model.ToolMetadata
	err      bool
	done     bool
}

const (
	toolInputContinuationRows = 2
	toolOutputRows            = 5
	directShellOutputRows     = 50
)

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

func newNoticeMessage(text string, failed bool) *chatMessage {
	m := &chatMessage{role: "notice", noticeError: failed}
	m.content.WriteString(ansi.Strip(text))
	return m
}

// handleEvent updates the message state based on an agent.Event.
func (m *chatMessage) handleEvent(e agent.Event) {
	switch e.Type {
	case agent.EventModelDelta:
		m.content.WriteString(ansi.Strip(e.Content))
	case agent.EventToolStarted:
		m.toolCalls = append(m.toolCalls, toolCallView{
			call: e.ToolCall,
		})
	case agent.EventToolFinished:
		if tc := m.findToolCall(e.ToolCall.ID); tc != nil {
			tc.result = ansi.Strip(e.ToolResult)
			tc.metadata = e.ToolMetadata
			tc.err = e.ToolError || e.ToolMetadata.Error
			tc.done = true
		}
	}
}

// findToolCall matches completion events by ID and falls back to the latest unfinished call.
func (m *chatMessage) findToolCall(id string) *toolCallView {
	for i := len(m.toolCalls) - 1; i >= 0; i-- {
		call := &m.toolCalls[i]
		if id != "" && call.call.ID == id {
			return call
		}
		if id == "" && !call.done {
			return call
		}
	}
	return nil
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
	case "notice":
		style := mutedStyle
		if m.noticeError {
			style = errorStyle
		}
		return renderIndented(m.content.String(), width, "• ", style)
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

// renderToolCall renders a compact semantic summary and a bounded result preview.
func renderToolCall(tc toolCallView, width int) string {
	if width <= 0 {
		return ""
	}
	statusStyle := toolStyle
	if tc.err {
		statusStyle = errorStyle
	} else if tc.done {
		statusStyle = assistantStyle
	}
	action, input := toolCallSummary(tc)
	lines := renderToolHeader(action, input, width, statusStyle.Bold(true))

	if tc.done && shouldRenderToolResult(tc) {
		result := strings.Trim(ansi.Strip(tc.result), "\r\n")
		if result == "" {
			if tc.err {
				result = "Tool failed"
			} else if tc.call.Name == "run_shell" {
				result = "(no output)"
			}
		}
		if result != "" {
			limit := toolOutputRows
			if tc.metadata.DirectShell {
				limit = directShellOutputRows
			}
			lines = append(lines, renderToolOutput(result, width, limit)...)
		}
	}
	return strings.Join(lines, "\n")
}

// toolCallSummary returns the lifecycle verb and the most useful input preview.
func toolCallSummary(tc toolCallView) (string, string) {
	detail := tool.DisplayDetail(tc.call)
	if detail == "" {
		detail = strings.TrimSpace(tc.call.Arguments)
	}

	switch tc.call.Name {
	case "run_shell":
		return toolLifecycleVerb(tc, "Running", "Ran", "Failed"), detail
	case "web_search":
		action := toolLifecycleVerb(tc, "Searching the web", "Searched the web", "Failed to search the web")
		if detail != "" {
			action += " for"
		}
		return action, detail
	case "web_fetch":
		return toolLifecycleVerb(tc, "Fetching", "Fetched", "Failed to fetch"), detail
	case "load_skill":
		return toolLifecycleVerb(tc, "Loading skill", "Loaded skill", "Failed to load skill"), detail
	case "todo_write":
		return toolLifecycleVerb(tc, "Updating", "Updated", "Failed to update"), detail
	default:
		name := tc.call.Name
		if name == "" {
			name = "tool"
		}
		if detail != "" {
			name += " " + detail
		}
		return toolLifecycleVerb(tc, "Calling", "Called", "Failed"), name
	}
}

func toolLifecycleVerb(tc toolCallView, running, completed, failed string) string {
	if tc.err {
		return failed
	}
	if tc.done {
		return completed
	}
	return running
}

func shouldRenderToolResult(tc toolCallView) bool {
	if tc.err {
		return true
	}
	switch tc.call.Name {
	case "web_search", "web_fetch", "load_skill", "todo_write":
		return false
	default:
		return true
	}
}

// renderToolHeader wraps the input before limiting it to two continuation rows.
func renderToolHeader(action, input string, width int, statusStyle lipgloss.Style) []string {
	if width <= 4 {
		return []string{statusStyle.Render(ansi.Truncate("• "+action, width, ""))}
	}

	content := statusStyle.Render(action)
	if input != "" {
		content += " " + messageStyle.Render(ansi.Strip(input))
	}
	wrapped := strings.Split(ansi.Hardwrap(content, width-4, true), "\n")
	visible := min(len(wrapped), 1+toolInputContinuationRows)
	lines := make([]string, 0, visible+1)
	for i := range visible {
		prefix := mutedStyle.Render("  │ ")
		if i == 0 {
			prefix = statusStyle.Render("• ")
		}
		lines = append(lines, prefix+wrapped[i])
	}
	if omitted := len(wrapped) - visible; omitted > 0 {
		hint := ansi.Truncate(fmt.Sprintf("… +%d lines", omitted), width-4, "")
		lines = append(lines, mutedStyle.Render("  │ "+hint))
	}
	return lines
}

// renderToolOutput keeps the beginning and end within the visible row budget.
func renderToolOutput(result string, width, limit int) []string {
	if width <= 4 || limit <= 0 {
		return nil
	}
	contentWidth := width - 4
	wrapped := strings.Split(ansi.Hardwrap(result, contentWidth, true), "\n")
	if len(wrapped) > limit {
		available := limit - 1
		head := available / 2
		tail := available - head
		omitted := len(wrapped) - head - tail
		hint := ansi.Truncate(fmt.Sprintf("… +%d lines", omitted), contentWidth, "")
		trimmed := make([]string, 0, limit)
		trimmed = append(trimmed, wrapped[:head]...)
		trimmed = append(trimmed, hint)
		trimmed = append(trimmed, wrapped[len(wrapped)-tail:]...)
		wrapped = trimmed
	}

	lines := make([]string, len(wrapped))
	for i, line := range wrapped {
		prefix := "    "
		if i == 0 {
			prefix = "  └ "
		}
		lines[i] = mutedStyle.Render(prefix + line)
	}
	return lines
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
					call: call,
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
			call.metadata = message.ToolMetadata
			call.err = message.ToolMetadata.Error
			call.done = true
		}
	}
	return rendered
}
