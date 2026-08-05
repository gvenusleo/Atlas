package tui

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"

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

type conversationRenderMsg struct {
	generation uint64
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
	contentLength int
	width         int
	dark          bool
	palette       colorPalette
	rendered      string
	valid         bool
}

type markdownRendererKey struct {
	width   int
	dark    bool
	palette colorPalette
}

type markdownRendererEntry struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
}

var markdownRenderers sync.Map

// toolCallView holds the display state of a single tool call.
type toolCallView struct {
	call      model.ToolCall
	result    string
	errorText string
	metadata  model.ToolMetadata
	err       bool
	done      bool
}

type conversationBlockKind uint8

const (
	conversationBlockNone conversationBlockKind = iota
	conversationBlockUser
	conversationBlockModel
	conversationBlockTool
	conversationBlockPlan
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
			if e.Err != nil {
				tc.errorText = ansi.Strip(e.Err.Error())
			}
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
func (m *chatMessage) render(width int, theme tuiTheme) string {
	return m.renderWithToolDividers(width, theme, false, conversationBlockNone, false)
}

// renderConversation renders a message within its surrounding conversation boundaries.
func (m *chatMessage) renderConversation(width int, theme tuiTheme, previousKind conversationBlockKind, followedByModelContent bool) string {
	return m.renderWithToolDividers(width, theme, true, previousKind, followedByModelContent)
}

func (m *chatMessage) renderWithToolDividers(width int, theme tuiTheme, showToolDividers bool, previousKind conversationBlockKind, followedByModelContent bool) string {
	switch m.role {
	case "user":
		content := renderIndented(m.content.String(), max(width-2, 1), "› ", theme.text)
		return userMessageStyle(theme).
			Width(width).
			Render(content)
	case "assistant":
		var parts []string
		if m.content.Len() > 0 {
			parts = append(parts, m.renderMarkdown(width, theme))
		}
		if toolBlock := m.renderToolCalls(width, theme, showToolDividers, previousKind, followedByModelContent); toolBlock != "" {
			parts = append(parts, toolBlock)
		}
		if m.cancelled {
			parts = append(parts, renderIndented("Cancelled", width, "• ", theme.muted))
		} else if m.err != nil {
			parts = append(parts, renderIndented(ansi.Strip(m.err.Error()), width, "• ", theme.error))
		}
		return strings.Join(parts, "\n\n")
	case "notice":
		style := theme.muted
		if m.noticeError {
			style = theme.error
		}
		return renderIndented(m.content.String(), width, "• ", style)
	}
	return m.content.String()
}

func (m *chatMessage) visible() bool {
	switch m.role {
	case "assistant":
		return m.content.Len() > 0 || len(m.toolCalls) > 0 || m.cancelled || m.err != nil
	case "user", "notice":
		return m.content.Len() > 0
	default:
		return m.content.Len() > 0
	}
}

func (m *chatMessage) startKind() conversationBlockKind {
	switch m.role {
	case "user":
		return conversationBlockUser
	case "assistant":
		if m.content.Len() > 0 {
			return conversationBlockModel
		}
		if len(m.toolCalls) > 0 {
			return toolCallBlockKind(m.toolCalls[0])
		}
	}
	return conversationBlockModel
}

func (m *chatMessage) endKind() conversationBlockKind {
	if m.role == "assistant" && len(m.toolCalls) > 0 && !m.cancelled && m.err == nil {
		return toolCallBlockKind(m.toolCalls[len(m.toolCalls)-1])
	}
	return m.startKind()
}

// renderMarkdown returns the cached assistant body when its render inputs are unchanged.
func (m *chatMessage) renderMarkdown(width int, theme tuiTheme) string {
	cache := &m.markdownCache
	if cache.valid &&
		cache.contentLength == m.content.Len() &&
		cache.width == width &&
		cache.dark == theme.dark &&
		cache.palette == theme.palette {
		return cache.rendered
	}

	rendered := renderAssistantMarkdown(m.content.String(), width, theme)
	*cache = markdownRenderCache{
		contentLength: m.content.Len(),
		width:         width,
		dark:          theme.dark,
		palette:       theme.palette,
		rendered:      rendered,
		valid:         true,
	}
	return rendered
}

// renderAssistantMarkdown renders one assistant body at the full message width.
func renderAssistantMarkdown(content string, width int, theme tuiTheme) string {
	if width <= 0 {
		return ""
	}

	wrapWidth := width
	key := markdownRendererKey{width: wrapWidth, dark: theme.dark, palette: theme.palette}
	entryValue, ok := markdownRenderers.Load(key)
	if !ok {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStyles(markdownStyle(theme)),
			glamour.WithWordWrap(wrapWidth),
			glamour.WithTableWrap(true),
			glamour.WithChromaFormatter("terminal16m"),
		)
		if err != nil {
			return renderWrapped(content, width, theme.text)
		}
		entryValue, _ = markdownRenderers.LoadOrStore(key, &markdownRendererEntry{renderer: renderer})
	}

	entry := entryValue.(*markdownRendererEntry)
	entry.mu.Lock()
	rendered, err := entry.renderer.Render(content)
	entry.mu.Unlock()
	if err != nil {
		return renderWrapped(content, width, theme.text)
	}

	// Glamour terminates the document with newlines that the conversation supplies itself.
	rendered = strings.TrimRight(rendered, "\n")
	lines := strings.Split(rendered, "\n")
	if rendered == "" {
		return ""
	}
	for i := range lines {
		plain := strings.TrimRight(ansi.Strip(lines[i]), " \t")
		lines[i] = ansi.Truncate(lines[i], ansi.StringWidth(plain), "")
	}
	return strings.Join(lines, "\n")
}

func (m *chatMessage) renderToolCalls(width int, theme tuiTheme, showDividers bool, previousKind conversationBlockKind, followedByModelContent bool) string {
	if len(m.toolCalls) == 0 || width <= 0 {
		return ""
	}
	if m.content.Len() > 0 {
		previousKind = conversationBlockModel
	}

	divider := theme.divider.Render(strings.Repeat("─", width))
	var rendered strings.Builder
	for _, call := range m.toolCalls {
		kind := toolCallBlockKind(call)
		body := renderToolCall(call, width, theme)
		if kind == conversationBlockPlan && !call.err {
			body = renderPlanUpdate(call, width, theme)
		}
		if body == "" {
			continue
		}

		if rendered.Len() == 0 {
			if showDividers && needsToolDivider(previousKind, kind) {
				rendered.WriteString(divider)
				rendered.WriteByte('\n')
			}
		} else if showDividers && needsToolDivider(previousKind, kind) {
			rendered.WriteByte('\n')
			rendered.WriteString(divider)
			rendered.WriteByte('\n')
		} else if kind == conversationBlockPlan {
			rendered.WriteString("\n\n")
		} else {
			rendered.WriteByte('\n')
		}
		rendered.WriteString(body)
		previousKind = kind
	}
	if rendered.Len() > 0 && showDividers && followedByModelContent {
		rendered.WriteByte('\n')
		rendered.WriteString(divider)
	}
	return rendered.String()
}

func needsToolDivider(previous, current conversationBlockKind) bool {
	return previous != conversationBlockNone &&
		previous != conversationBlockUser &&
		previous != current
}

func toolCallBlockKind(call toolCallView) conversationBlockKind {
	if call.call.Name == "update_plan" {
		return conversationBlockPlan
	}
	return conversationBlockTool
}

func renderPlanUpdate(call toolCallView, width int, theme tuiTheme) string {
	entries := call.metadata.Plan
	if len(entries) == 0 {
		parsed, err := tool.ParsePlan(call.call.Arguments)
		if err != nil {
			return renderToolCall(call, width, theme)
		}
		entries = parsed
	}

	title := "Updated Plan"
	if !call.done {
		title = "Updating Plan"
	}
	statusStyle := theme.selected
	allCompleted := len(entries) > 0
	for _, entry := range entries {
		if entry.Status != model.PlanStatusCompleted {
			allCompleted = false
			break
		}
	}
	if allCompleted {
		statusStyle = theme.success
	}
	lines := []string{ansi.Truncate(statusStyle.Render("• ")+statusStyle.Bold(true).Render(title), width, "…")}
	if len(entries) == 0 {
		empty := theme.muted.Italic(true).Render("(no steps provided)")
		return strings.Join(append(lines, ansi.Truncate("  "+empty, width, "…")), "\n")
	}
	for _, entry := range entries {
		lines = append(lines, renderPlanEntry(entry, width, theme)...)
	}
	return strings.Join(lines, "\n")
}

func renderPlanEntry(entry model.PlanEntry, width int, theme tuiTheme) []string {
	symbol := "□"
	symbolStyle := theme.muted
	textStyle := theme.muted
	switch entry.Status {
	case model.PlanStatusCompleted:
		symbol = "✔"
		symbolStyle = theme.success
		textStyle = theme.muted.Strikethrough(true)
	case model.PlanStatusInProgress:
		symbolStyle = theme.selected
		textStyle = theme.selected.Bold(true)
	}

	step := strings.TrimSpace(ansi.Strip(entry.Step))
	const prefixWidth = 4
	if width <= prefixWidth {
		line := "  " + symbolStyle.Render(symbol) + " " + textStyle.Render(step)
		return []string{ansi.Truncate(line, width, "…")}
	}
	wrapped := strings.Split(ansi.Wrap(step, width-prefixWidth, ""), "\n")
	for i, line := range wrapped {
		prefix := "    "
		if i == 0 {
			prefix = "  " + symbolStyle.Render(symbol) + " "
		}
		wrapped[i] = prefix + textStyle.Render(line)
	}
	return wrapped
}

// renderToolCall renders a compact status summary and an optional failure detail.
func renderToolCall(tc toolCallView, width int, theme tuiTheme) string {
	if width <= 0 {
		return ""
	}
	statusStyle := theme.working
	if tc.err {
		statusStyle = theme.error
	} else if tc.done {
		statusStyle = theme.success
	}

	line := renderToolSummary(tc, width, statusStyle, theme.text)
	if !tc.err {
		return line
	}
	errorLine := "  Failed: " + toolCallErrorDetail(tc)
	return line + "\n" + theme.error.Render(ansi.Truncate(errorLine, width, "…"))
}

func renderToolSummary(tc toolCallView, width int, statusStyle, textStyle lipgloss.Style) string {
	name, detail := toolCallPresentation(tc)
	prefix := "• " + name
	if detail == "" {
		return statusStyle.Render(ansi.Truncate(prefix, width, "…"))
	}
	prefix += ":"
	prefixWidth := ansi.StringWidth(prefix)
	if prefixWidth >= width {
		return statusStyle.Render(ansi.Truncate(prefix, width, "…"))
	}
	remaining := width - prefixWidth
	if remaining == 1 {
		return statusStyle.Render(prefix) + textStyle.Render("…")
	}
	return statusStyle.Render(prefix) + textStyle.Render(" "+ansi.Truncate(detail, remaining-1, "…"))
}

// toolCallSummary returns the stable name and primary user-facing argument.
func toolCallSummary(tc toolCallView) string {
	name, detail := toolCallPresentation(tc)
	if detail == "" {
		return name
	}
	return name + ": " + detail
}

func toolCallPresentation(tc toolCallView) (string, string) {
	name := toolDisplayName(tc.call.Name)
	detail := tool.DisplayDetail(tc.call)
	if tc.call.Name == "run_shell" {
		purpose := runShellPurpose(tc.call)
		if purpose != "" && purpose != "Run shell command" {
			detail = purpose
		}
	}
	detail = singleLine(detail)
	return name, detail
}

func toolDisplayName(name string) string {
	switch name {
	case "run_shell":
		return "RunShell"
	case "read":
		return "Read"
	case "edit":
		return "Edit"
	case "write":
		return "Write"
	case "web_search":
		return "WebSearch"
	case "web_fetch":
		return "WebFetch"
	case "load_skill":
		return "LoadSkill"
	case "update_plan":
		return "UpdatePlan"
	}
	if name == "" {
		return "Tool"
	}
	if name = singleLine(name); name != "" {
		return name
	}
	return "Tool"
}

func runShellPurpose(call model.ToolCall) string {
	var args struct {
		Purpose string `json:"purpose"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(ansi.Strip(args.Purpose))
}

func toolCallErrorDetail(tc toolCallView) string {
	if detail := singleLine(tc.errorText); detail != "" {
		return detail
	}
	if tc.metadata.DirectShell {
		return "Tool failed"
	}
	lines := strings.Split(strings.ReplaceAll(ansi.Strip(tc.result), "\r\n", "\n"), "\n")
	for _, line := range slices.Backward(lines) {
		if detail := singleLine(line); detail != "" {
			return detail
		}
	}
	return "Tool failed"
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(ansi.Strip(value)), " ")
}

func renderWrapped(content string, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	return style.Render(ansi.Hardwrap(ansi.Strip(content), width, true))
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
