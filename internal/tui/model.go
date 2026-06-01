package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/storage"
)

// Run starts the Atlas terminal UI.
func Run(ctx context.Context, atlas *agent.Agent) error {
	model := newModel(ctx, atlas)
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

type model struct {
	ctx       context.Context
	agent     *agent.Agent
	session   storage.Session
	input     strings.Builder
	entries   []entry
	events    <-chan agent.Event
	errs      <-chan error
	streaming bool
	status    string
	width     int
	height    int
	err       error
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

type sessionCreatedMsg storage.Session
type agentEventMsg agent.Event
type errMsg error
type turnDoneMsg struct{}

// newModel initializes the visible transcript and process dependencies.
func newModel(ctx context.Context, atlas *agent.Agent) model {
	return model{
		ctx:    ctx,
		agent:  atlas,
		status: "starting",
		entries: []entry{
			{kind: entryMeta, title: "Atlas", body: "Coding agent, full local access, DeepSeek"},
			{kind: entryMeta, title: "Help", body: "Enter sends, Esc/Ctrl+C quits"},
		},
	}
}

// Init creates the first durable session for this TUI process.
func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		session, err := m.agent.CreateSession(m.ctx, "TUI session")
		if err != nil {
			return errMsg(err)
		}
		return sessionCreatedMsg(session)
	}
}

// Update handles keyboard input, session creation, and streamed agent events.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			text := strings.TrimSpace(m.input.String())
			if text == "" || m.session.ID == "" {
				return m, nil
			}
			m.input.Reset()
			m.entries = append(m.entries, entry{kind: entryUser, title: "You", body: text})
			m.entries = append(m.entries, entry{kind: entryAssistant, title: "Atlas", running: true})
			m.streaming = true
			m.status = "running"
			m.events, m.errs = m.agent.RunTurn(m.ctx, m.session.ID, text)
			return m, m.nextAgentMessage()
		case tea.KeyBackspace:
			m.deleteLastInputRune()
		default:
			if msg.Type == tea.KeyRunes {
				m.input.WriteString(msg.String())
			}
		}
	case sessionCreatedMsg:
		m.session = storage.Session(msg)
		m.status = "ready"
		m.entries = append(m.entries, entry{kind: entryMeta, title: "Session", body: m.session.ID})
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
		m.entries = append(m.entries, entry{kind: entryError, title: "Error", body: m.err.Error(), failed: true})
		m.streaming = false
		m.status = "error"
	}
	return m, nil
}

// View renders a compact Codex-style transcript with a bottom prompt.
func (m model) View() string {
	bodyHeight := m.height - 5
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	width := m.width
	if width <= 0 {
		width = 96
	}
	body := m.renderTranscript(width, bodyHeight)
	status := m.renderStatus(width)
	prompt := m.renderPrompt(width)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		body,
		status,
		prompt,
	)
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
		m.entries = append(m.entries, entry{kind: entryTool, title: event.ToolName, body: "running", running: true})
	case agent.EventToolFinished:
		m.finishTool(event)
	case agent.EventTurnFinished:
		m.finishAssistant()
		m.status = "ready"
	case agent.EventError:
		m.finishAssistant()
		m.entries = append(m.entries, entry{kind: entryError, title: "Error", body: event.Text, failed: true})
		m.status = "error"
	}
}

// deleteLastInputRune removes one user-visible rune from the input buffer.
func (m *model) deleteLastInputRune() {
	runes := []rune(m.input.String())
	if len(runes) == 0 {
		return
	}
	m.input.Reset()
	m.input.WriteString(string(runes[:len(runes)-1]))
}

// appendAssistantDelta appends streamed assistant text to the active response.
func (m *model) appendAssistantDelta(delta string) {
	idx := m.activeAssistantIndex()
	if idx < 0 {
		m.entries = append(m.entries, entry{kind: entryAssistant, title: "Atlas"})
		idx = len(m.entries) - 1
	}
	m.entries[idx].body += delta
	m.entries[idx].running = true
}

// finishAssistant marks the active assistant message as complete.
func (m *model) finishAssistant() {
	idx := m.activeAssistantIndex()
	if idx >= 0 {
		m.entries[idx].running = false
		if strings.TrimSpace(m.entries[idx].body) == "" {
			m.entries = append(m.entries[:idx], m.entries[idx+1:]...)
		}
	}
}

// finishTool replaces the latest running tool entry with a compact result.
func (m *model) finishTool(event agent.Event) {
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
		return
	}
	m.entries = append(m.entries, next)
}

// activeAssistantIndex returns the trailing assistant entry, if one is active.
func (m model) activeAssistantIndex() int {
	idx := len(m.entries) - 1
	if idx >= 0 && m.entries[idx].kind == entryAssistant {
		return idx
	}
	return -1
}

// renderTranscript renders the scrollback window at a fixed width.
func (m model) renderTranscript(width int, maxLines int) string {
	var lines []string
	for _, item := range m.entries {
		lines = append(lines, renderEntry(item, width)...)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
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

// renderPrompt renders the composer row.
func (m model) renderPrompt(width int) string {
	prefix := "atlas> "
	value := m.input.String()
	if m.streaming {
		value = "waiting for Atlas..."
	}
	available := width - utf8.RuneCountInString(prefix)
	if available < 1 {
		available = 1
	}
	return promptStyle.Width(width).Render(prefix + tailFit(value, available))
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
		head = userStyle.Render("› " + label)
	case entryAssistant:
		head = titleStyle.Render("● " + label)
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

// wrapAndIndent wraps text to the available terminal width.
func wrapAndIndent(text string, width int, indent string) []string {
	available := width - utf8.RuneCountInString(indent)
	if available < 1 {
		available = 1
	}
	var out []string
	for _, paragraph := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapLine(paragraph, available, indent)...)
	}
	return out
}

// wrapLine wraps one line on rune count boundaries.
func wrapLine(line string, width int, indent string) []string {
	runes := []rune(line)
	if len(runes) <= width {
		return []string{indent + line}
	}
	var out []string
	for len(runes) > 0 {
		n := width
		if len(runes) < n {
			n = len(runes)
		}
		out = append(out, indent+string(runes[:n]))
		runes = runes[n:]
	}
	return out
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

// tailFit keeps the visible end of user input inside the composer row.
func tailFit(text string, width int) string {
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
	return "…" + string(runes[len(runes)-width+1:])
}

// spinnerFrame returns a stable minimal activity glyph.
func spinnerFrame() string {
	return "·"
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	mutedStyle  = lipgloss.NewStyle().Faint(true)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle = lipgloss.NewStyle().Faint(true).Border(lipgloss.NormalBorder(), true, false, false, false).PaddingTop(1)
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)
