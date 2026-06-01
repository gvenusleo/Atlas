package tui

import (
	"context"
	"fmt"
	"strings"

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
	lines     []string
	events    <-chan agent.Event
	errs      <-chan error
	streaming bool
	width     int
	height    int
	err       error
}

type sessionCreatedMsg storage.Session
type agentEventMsg agent.Event
type errMsg error
type turnDoneMsg struct{}

// newModel initializes the visible transcript and process dependencies.
func newModel(ctx context.Context, atlas *agent.Agent) model {
	return model{
		ctx:   ctx,
		agent: atlas,
		lines: []string{"Atlas coding agent", "Type a request and press Enter. Ctrl+C quits."},
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
			m.lines = append(m.lines, "", userStyle.Render("> "+text), "")
			m.streaming = true
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
		m.lines = append(m.lines, mutedStyle.Render("session "+m.session.ID))
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
	case errMsg:
		m.err = error(msg)
		m.lines = append(m.lines, errorStyle.Render(m.err.Error()))
		m.streaming = false
	}
	return m, nil
}

// View renders a compact Codex-style transcript with a bottom prompt.
func (m model) View() string {
	bodyHeight := m.height - 4
	if bodyHeight < 5 {
		bodyHeight = 5
	}
	start := 0
	if len(m.lines) > bodyHeight {
		start = len(m.lines) - bodyHeight
	}
	body := strings.Join(m.lines[start:], "\n")
	prompt := "atlas> " + m.input.String()
	if m.streaming {
		prompt = mutedStyle.Render("running...")
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Atlas"),
		body,
		statusStyle.Render(prompt),
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
		if len(m.lines) == 0 || strings.HasPrefix(m.lines[len(m.lines)-1], ">") {
			m.lines = append(m.lines, "")
		}
		m.lines[len(m.lines)-1] += event.Text
	case agent.EventToolStarted:
		m.lines = append(m.lines, toolStyle.Render(fmt.Sprintf("tool %s started", event.ToolName)))
	case agent.EventToolFinished:
		status := "finished"
		if event.Error {
			status = "failed"
		}
		m.lines = append(m.lines, toolStyle.Render(fmt.Sprintf("tool %s %s", event.ToolName, status)))
	case agent.EventTurnFinished:
		m.lines = append(m.lines, mutedStyle.Render("done"))
	case agent.EventError:
		m.lines = append(m.lines, errorStyle.Render(event.Text))
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

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).MarginBottom(1)
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).PaddingTop(1)
)
