package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/storage"
)

// Run starts the Atlas terminal UI.
func Run(ctx context.Context, atlas *agent.Agent) error {
	cursor := &cursorState{}
	model := newModel(ctx, atlas, cursor)
	output := &cursorWriter{File: os.Stdout, cursor: cursor}
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(output)).Run()
	return err
}

type model struct {
	ctx       context.Context
	agent     *agent.Agent
	session   storage.Session
	composer  composerState
	entries   []entry
	events    <-chan agent.Event
	errs      <-chan error
	streaming bool
	status    string
	scroll    int
	width     int
	height    int
	err       error
	cursor    *cursorState
}

type entryKind int

const (
	entryMeta entryKind = iota
	entryUser
	entryAssistant
	entryTool
	entryError
)

type entry struct {
	kind    entryKind
	title   string
	body    string
	running bool
	failed  bool
}

type composerState struct {
	text   string
	scroll int
}

const (
	promptPrefix       = "> "
	promptContinuation = "  "
	maxComposerRows    = 10
)

type sessionCreatedMsg storage.Session
type agentEventMsg agent.Event
type errMsg error
type turnDoneMsg struct{}

// newModel initializes the visible transcript and process dependencies.
func newModel(ctx context.Context, atlas *agent.Agent, cursor *cursorState) model {
	return model{
		ctx:    ctx,
		agent:  atlas,
		cursor: cursor,
		status: "starting",
	}
}

// Init creates the first durable session for this TUI process.
func (m model) Init() tea.Cmd {
	createSession := func() tea.Msg {
		session, err := m.agent.CreateSession(m.ctx, "TUI session")
		if err != nil {
			return errMsg(err)
		}
		return sessionCreatedMsg(session)
	}
	return tea.Batch(tea.ShowCursor, createSession)
}

// Update handles keyboard input, session creation, and streamed agent events.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.composer.Clamp(promptContentWidth(m.viewWidth()))
		m.clampScroll()
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.syncCursor(0, 0, false)
			return m, tea.Quit
		case tea.KeyCtrlJ:
			if !m.streaming {
				m.composer.WriteRune('\n')
				m.composer.ScrollToBottom()
				m.clampScroll()
			}
		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			text := strings.TrimSpace(m.composer.String())
			if text == "" || m.session.ID == "" {
				return m, nil
			}
			m.composer.Reset()
			m.scroll = 0
			m.appendEntry(entry{kind: entryUser, body: text})
			m.appendEntry(entry{kind: entryAssistant, running: true})
			m.streaming = true
			m.status = "running"
			m.events, m.errs = m.agent.RunTurn(m.ctx, m.session.ID, text)
			return m, m.nextAgentMessage()
		case tea.KeyUp:
			if !m.composer.Scroll(1, promptContentWidth(m.viewWidth())) {
				m.scrollTranscript(1)
			}
		case tea.KeyDown:
			if !m.composer.Scroll(-1, promptContentWidth(m.viewWidth())) {
				m.scrollTranscript(-1)
			}
		case tea.KeyPgUp:
			m.scrollTranscript(m.pageScrollSize())
		case tea.KeyPgDown:
			m.scrollTranscript(-m.pageScrollSize())
		case tea.KeyBackspace:
			m.deleteLastInputRune()
		case tea.KeySpace:
			m.composer.WriteRune(' ')
			m.composer.ScrollToBottom()
		default:
			if msg.Type == tea.KeyRunes {
				m.composer.WriteString(msg.String())
				m.composer.ScrollToBottom()
			}
		}
	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scrollTranscript(3)
		case tea.MouseWheelDown:
			m.scrollTranscript(-3)
		}
	case sessionCreatedMsg:
		m.session = storage.Session(msg)
		m.status = "ready"
	case agentEventMsg:
		event := agent.Event(msg)
		m.appendEvent(event)
		if event.Type == agent.EventTurnFinished || event.Type == agent.EventError {
			m.streaming = false
			m.events = nil
			m.errs = nil
			return m, nil
		}
		return m, m.nextAgentMessage()
	case turnDoneMsg:
		m.streaming = false
		m.events = nil
		m.errs = nil
		m.status = "ready"
	case errMsg:
		m.err = error(msg)
		m.appendEntry(entry{kind: entryError, title: "Error", body: m.err.Error(), failed: true})
		m.streaming = false
		m.status = "error"
	}
	return m, nil
}

// View renders a compact Codex-style transcript with a bottom prompt.
func (m model) View() string {
	width := m.viewWidth()
	status := m.renderStatus(width)
	prompt, promptCursorRow, cursorCol, cursorVisible := m.renderPromptLine(width)
	bodyHeight := m.bodyHeight()
	body := m.renderTranscript(width, bodyHeight)
	view := lipgloss.JoinVertical(
		lipgloss.Left,
		body,
		status,
		prompt,
	)
	cursorRow := renderedLineCount(body) + renderedLineCount(status) + promptCursorRow
	m.syncCursor(cursorRow, cursorCol, cursorVisible)
	return view
}

// nextAgentMessage waits for the next event from the active turn.
func (m model) nextAgentMessage() tea.Cmd {
	return func() tea.Msg {
		events := m.events
		errs := m.errs
		for events != nil || errs != nil {
			select {
			case event, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				return agentEventMsg(event)
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err != nil {
					return errMsg(err)
				}
			}
		}
		return turnDoneMsg{}
	}
}

// appendEvent folds one agent event into the visible transcript.
func (m *model) appendEvent(event agent.Event) {
	switch event.Type {
	case agent.EventTextDelta:
		m.appendAssistantDelta(event.Text)
	case agent.EventToolStarted:
		m.finishAssistant()
		m.appendEntry(entry{kind: entryTool, title: event.ToolName, body: "running", running: true})
	case agent.EventToolFinished:
		m.finishTool(event)
	case agent.EventTurnFinished:
		m.finishAssistant()
		m.status = "ready"
	case agent.EventError:
		m.finishAssistant()
		m.appendEntry(entry{kind: entryError, title: "Error", body: event.Text, failed: true})
		m.status = "error"
	}
}

// deleteLastInputRune removes one user-visible rune from the input buffer.
func (m *model) deleteLastInputRune() {
	m.composer.DeleteLastRune()
	m.composer.ScrollToBottom()
	m.composer.Clamp(promptContentWidth(m.viewWidth()))
}

// appendAssistantDelta appends streamed assistant text to the active response.
func (m *model) appendAssistantDelta(delta string) {
	before := m.transcriptLineCount()
	idx := m.activeAssistantIndex()
	if idx < 0 {
		m.entries = append(m.entries, entry{kind: entryAssistant})
		idx = len(m.entries) - 1
	}
	m.entries[idx].body += delta
	m.entries[idx].running = true
	m.keepScrollPosition(before)
}

// finishAssistant marks the active assistant message as complete.
func (m *model) finishAssistant() {
	before := m.transcriptLineCount()
	idx := m.activeAssistantIndex()
	if idx >= 0 {
		m.entries[idx].running = false
		if strings.TrimSpace(m.entries[idx].body) == "" {
			m.entries = append(m.entries[:idx], m.entries[idx+1:]...)
		}
		m.keepScrollPosition(before)
	}
}

// finishTool replaces the latest running tool entry with a compact result.
func (m *model) finishTool(event agent.Event) {
	before := m.transcriptLineCount()
	idx := -1
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].kind == entryTool && m.entries[i].title == event.ToolName && m.entries[i].running {
			idx = i
			break
		}
	}
	body := summarizeToolOutput(event.Text)
	next := entry{kind: entryTool, title: event.ToolName, body: body, failed: event.Error}
	if idx >= 0 {
		m.entries[idx] = next
		m.keepScrollPosition(before)
		return
	}
	m.appendEntry(next)
}

// activeAssistantIndex returns the trailing assistant entry, if one is active.
func (m model) activeAssistantIndex() int {
	idx := len(m.entries) - 1
	if idx >= 0 && m.entries[idx].kind == entryAssistant {
		return idx
	}
	return -1
}

// appendEntry adds transcript content while preserving the user's scroll view.
func (m *model) appendEntry(item entry) {
	before := m.transcriptLineCount()
	m.entries = append(m.entries, item)
	m.keepScrollPosition(before)
}

// scrollTranscript moves the conversation viewport without changing input.
func (m *model) scrollTranscript(delta int) {
	m.scroll += delta
	m.clampScroll()
}

// clampScroll keeps the scroll offset inside the rendered transcript bounds.
func (m *model) clampScroll() {
	max := m.maxScroll()
	if m.scroll > max {
		m.scroll = max
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// keepScrollPosition prevents background output from moving a manual scroll.
func (m *model) keepScrollPosition(beforeLineCount int) {
	if m.scroll > 0 {
		m.scroll += m.transcriptLineCount() - beforeLineCount
	}
	m.clampScroll()
}

// transcriptLineCount returns the number of rendered transcript rows.
func (m model) transcriptLineCount() int {
	return len(m.transcriptLines(m.viewWidth()))
}

// maxScroll returns how many lines can be hidden below the transcript viewport.
func (m model) maxScroll() int {
	lines := m.transcriptLineCount()
	overflow := lines - m.bodyHeight()
	if overflow < 0 {
		return 0
	}
	return overflow
}

// pageScrollSize returns a conservative page step for keyboard scrolling.
func (m model) pageScrollSize() int {
	size := m.bodyHeight() - 1
	if size < 1 {
		return 1
	}
	return size
}

// bodyHeight returns the transcript height left after status and composer rows.
func (m model) bodyHeight() int {
	width := m.viewWidth()
	bodyHeight := m.height - m.statusHeight(width) - m.composer.Height(promptContentWidth(width), m.streaming)
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	return bodyHeight
}

// viewWidth returns a stable render width before the terminal reports size.
func (m model) viewWidth() int {
	if m.width <= 0 {
		return 96
	}
	return m.width
}

// transcriptLines renders every transcript line before viewport clipping.
func (m model) transcriptLines(width int) []string {
	var lines []string
	for _, item := range m.entries {
		lines = append(lines, renderEntry(item, width)...)
	}
	return lines
}

// renderTranscript renders the scrollback window at a fixed width.
func (m model) renderTranscript(width int, maxLines int) string {
	lines := m.transcriptLines(width)
	if len(lines) > maxLines {
		end := len(lines) - m.scroll
		if end > len(lines) {
			end = len(lines)
		}
		if end < maxLines {
			end = maxLines
		}
		lines = lines[end-maxLines : end]
	}
	for len(lines) < maxLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderStatus renders the fixed status strip above the input.
func (m model) renderStatus(width int) string {
	sessionID := "new"
	modelName := "deepseek"
	if m.session.ID != "" {
		sessionID = m.session.ID
		modelName = m.session.Model
	}
	text := fmt.Sprintf("%s  session %s  model %s", m.status, shortID(sessionID), modelName)
	if m.streaming {
		text = "running  " + spinnerFrame() + "  session " + shortID(sessionID) + "  model " + modelName
	}
	return statusStyle.Width(width).Render(fitLine(text, width))
}

// statusHeight returns the visible height of the status strip.
func (m model) statusHeight(width int) int {
	return renderedLineCount(m.renderStatus(width))
}

// renderPromptLine renders the composer and returns the cursor row and cell.
func (m model) renderPromptLine(width int) (string, int, int, bool) {
	if m.streaming {
		prefixWidth := lipgloss.Width(promptPrefix)
		line := promptPrefix + fitLine("waiting for Atlas...", width-prefixWidth)
		return promptStyle.Width(width).Render(line), 1, 0, false
	}
	return m.composer.Render(width)
}

// renderEntry converts one transcript entry into styled terminal lines.
func renderEntry(item entry, width int) []string {
	label := item.title
	if item.running {
		label += " " + spinnerFrame()
	}
	var head string
	switch item.kind {
	case entryMeta:
		head = mutedStyle.Render("• " + label)
	case entryUser:
		lines := wrapAndPrefix(item.body, width, "› ", "  ")
		for i := range lines {
			lines[i] = userStyle.Render(lines[i])
		}
		return lines
	case entryAssistant:
		if strings.TrimSpace(item.body) == "" {
			if item.running {
				return []string{mutedStyle.Render(spinnerFrame())}
			}
			return nil
		}
		return wrapAndIndent(item.body, width, "")
	case entryTool:
		marker := "✓ "
		style := toolStyle
		if item.failed {
			marker = "× "
			style = errorStyle
		} else if item.running {
			marker = "… "
		}
		head = style.Render(marker + label)
	case entryError:
		head = errorStyle.Render("× " + label)
	}
	body := wrapAndIndent(item.body, width, "  ")
	if strings.TrimSpace(item.body) == "" {
		return []string{head}
	}
	return append([]string{head}, body...)
}

// String returns the raw composer draft.
func (c *composerState) String() string {
	return c.text
}

// WriteRune appends one rune to the composer draft.
func (c *composerState) WriteRune(r rune) {
	c.text += string(r)
}

// WriteString appends text to the composer draft.
func (c *composerState) WriteString(text string) {
	c.text += text
}

// Reset clears the composer draft and viewport.
func (c *composerState) Reset() {
	c.text = ""
	c.scroll = 0
}

// DeleteLastRune removes one user-visible rune from the draft.
func (c *composerState) DeleteLastRune() {
	runes := []rune(c.text)
	if len(runes) == 0 {
		return
	}
	c.text = string(runes[:len(runes)-1])
}

// Height returns the visible composer row count, capped at maxComposerRows.
func (c composerState) Height(width int, streaming bool) int {
	if streaming {
		return 1
	}
	lines := c.visualLines(width)
	if len(lines) < 1 {
		return 1
	}
	if len(lines) > maxComposerRows {
		return maxComposerRows
	}
	return len(lines)
}

// Render draws the composer and returns the cursor row and column.
func (c composerState) Render(width int) (string, int, int, bool) {
	lines := c.visualLines(promptContentWidth(width))
	visible := c.visibleLines(lines, promptContentWidth(width))
	rendered := make([]string, 0, len(visible))
	for i, line := range visible {
		prefix := promptContinuation
		if i == 0 {
			prefix = promptPrefix
		}
		rendered = append(rendered, promptStyle.Width(width).Render(prefix+line))
	}
	if len(rendered) == 0 {
		rendered = append(rendered, promptStyle.Width(width).Render(promptPrefix))
	}

	cursorRow := len(visible)
	lastLine := ""
	if len(visible) > 0 {
		lastLine = visible[len(visible)-1]
	}
	cursorPrefix := promptContinuation
	if len(visible) == 1 {
		cursorPrefix = promptPrefix
	}
	cursorCol := lipgloss.Width(cursorPrefix) + lipgloss.Width(lastLine) + 1
	if cursorCol > width {
		cursorCol = width
	}
	return strings.Join(rendered, "\n"), cursorRow, cursorCol, c.scroll == 0
}

// Scroll moves the composer viewport when the draft exceeds maxComposerRows.
func (c *composerState) Scroll(delta int, width int) bool {
	lines := c.visualLines(width)
	if len(lines) <= maxComposerRows {
		c.scroll = 0
		return false
	}
	before := c.scroll
	c.scroll += delta
	c.Clamp(width)
	return c.scroll != before
}

// ScrollToBottom makes newly typed content visible.
func (c *composerState) ScrollToBottom() {
	c.scroll = 0
}

// Clamp keeps the composer viewport inside the wrapped draft.
func (c *composerState) Clamp(width int) {
	max := c.maxScroll(width)
	if c.scroll > max {
		c.scroll = max
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
}

func (c composerState) maxScroll(width int) int {
	lines := c.visualLines(width)
	if len(lines) <= maxComposerRows {
		return 0
	}
	return len(lines) - maxComposerRows
}

func (c composerState) visibleLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return []string{""}
	}
	maxScroll := 0
	if len(lines) > maxComposerRows {
		maxScroll = len(lines) - maxComposerRows
	}
	scroll := c.scroll
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := len(lines) - scroll
	start := end - maxComposerRows
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

func (c composerState) visualLines(width int) []string {
	return visualLines(c.text, width)
}

// wrapAndIndent wraps text to the available terminal width.
func wrapAndIndent(text string, width int, indent string) []string {
	return wrapAndPrefix(text, width, indent, indent)
}

// wrapAndPrefix wraps text with separate first-line and continuation prefixes.
func wrapAndPrefix(text string, width int, firstIndent string, nextIndent string) []string {
	var out []string
	first := true
	for _, paragraph := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		indent := nextIndent
		if first {
			indent = firstIndent
		}
		if paragraph == "" {
			out = append(out, indent)
			first = false
			continue
		}
		out = append(out, wrapParagraph(paragraph, width, firstIndent, nextIndent, first)...)
		first = false
	}
	return out
}

// wrapParagraph wraps one logical line using display width.
func wrapParagraph(line string, width int, firstIndent string, nextIndent string, useFirst bool) []string {
	var out []string
	indent := nextIndent
	if useFirst {
		indent = firstIndent
	}
	for _, chunk := range wrapDisplayLine(line, contentWidth(width, indent)) {
		out = append(out, indent+chunk)
		indent = nextIndent
	}
	return out
}

// wrapDisplayLine wraps a single line by terminal cell width.
func wrapDisplayLine(line string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	var chunk strings.Builder
	used := 0
	for _, r := range line {
		cellWidth := lipgloss.Width(string(r))
		if used > 0 && used+cellWidth > width {
			out = append(out, chunk.String())
			chunk.Reset()
			used = 0
		}
		chunk.WriteRune(r)
		used += cellWidth
	}
	if chunk.Len() > 0 || len(out) == 0 {
		out = append(out, chunk.String())
	}
	return out
}

// visualLines converts explicit newlines and wrapped rows into composer rows.
func visualLines(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapDisplayLine(part, width)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// summarizeToolOutput returns a compact, readable preview for tool output.
func summarizeToolOutput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "completed"
	}
	lines := strings.Split(text, "\n")
	limit := 4
	if len(lines) < limit {
		limit = len(lines)
	}
	preview := strings.Join(lines[:limit], "\n")
	if len(lines) > limit {
		preview += "\n… " + strconv.Itoa(len(lines)-limit) + " more line(s)"
	}
	return preview
}

// shortID keeps session identifiers visible without taking over the status bar.
func shortID(id string) string {
	if len(id) <= 10 {
		return id
	}
	return id[:10]
}

// fitLine keeps fixed chrome from wrapping in narrow terminals.
func fitLine(text string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// contentWidth returns the available terminal cells after a prefix is applied.
func contentWidth(width int, prefix string) int {
	available := width - lipgloss.Width(prefix)
	if available < 1 {
		return 1
	}
	return available
}

// promptContentWidth returns the editable width inside the composer prompt.
func promptContentWidth(width int) int {
	return contentWidth(width, promptPrefix)
}

// renderedLineCount returns the number of terminal rows in a rendered block.
func renderedLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// syncCursor records the terminal cursor target after a render.
func (m model) syncCursor(row int, col int, visible bool) {
	if m.cursor == nil {
		return
	}
	m.cursor.Set(row, col, visible)
}

// cursorState stores the real terminal cursor target for IME composition.
type cursorState struct {
	mu      sync.Mutex
	row     int
	col     int
	visible bool
}

// Set updates the desired cursor location for the next terminal write.
func (s *cursorState) Set(row int, col int, visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.row = row
	s.col = col
	s.visible = visible
}

// Sequence returns the ANSI sequence needed after Bubble Tea resets the cursor.
func (s *cursorState) Sequence() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.visible || s.row <= 0 || s.col <= 0 {
		return ansi.HideCursor
	}
	return ansi.ShowCursor + ansi.CursorPosition(s.col, s.row)
}

// cursorWriter restores the real terminal cursor after Bubble Tea renders.
type cursorWriter struct {
	*os.File
	cursor *cursorState
}

// Write forwards Bubble Tea output and then positions the real cursor for IME.
func (w *cursorWriter) Write(p []byte) (int, error) {
	n, err := w.File.Write(p)
	if err != nil || w.cursor == nil || !bytes.Contains(p, []byte(promptPrefix)) {
		return n, err
	}
	_, cursorErr := w.File.WriteString(w.cursor.Sequence())
	if err == nil {
		err = cursorErr
	}
	return n, err
}

// spinnerFrame returns a stable minimal activity glyph.
func spinnerFrame() string {
	return "·"
}

var (
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	mutedStyle  = lipgloss.NewStyle().Faint(true)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle = lipgloss.NewStyle().Faint(true).Border(lipgloss.NormalBorder(), true, false, false, false).PaddingTop(1)
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)
