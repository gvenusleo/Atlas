// Package tui implements the Atlas interactive terminal interface.
package tui

import (
	"context"
	"errors"
	"strings"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/runtime"
)

const maxComposerHeight = 10

// Options configures the TUI at startup.
type Options struct {
	Runtime   *runtime.Runtime
	SessionID string
	CWD       string
	context   context.Context
}

// Model is the top-level TUI model managing layout, input, and content display.
type Model struct {
	width  int
	height int
	ready  bool

	// viewport renders the accumulated message log.
	viewport viewport.Model
	// input is the multi-line text editor for user prompts.
	input textarea.Model

	// Agent integration.
	rt        *runtime.Runtime
	cwd       string
	sessionID string
	ctx       context.Context
	loading   bool

	// Turn state.
	turnActive  bool
	turnCancel  context.CancelFunc
	turnAbandon context.CancelFunc
	eventCh     chan turnUpdateMsg

	// Conversation messages.
	messages []*chatMessage
	// current points to the assistant message being streamed (or nil).
	current *chatMessage
}

// New creates the initial TUI model.
func New(opts Options) Model {
	vp := viewport.New()
	vp.SoftWrap = true

	ta := textarea.New()
	ta.SetVirtualCursor(false)
	ta.Focus()
	ta.Prompt = "› "
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = maxComposerHeight
	ta.KeyMap.InsertNewline.SetEnabled(false)

	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Cursor.Shape = tea.CursorBar
	ta.SetStyles(s)

	ctx := opts.context
	if ctx == nil {
		ctx = context.Background()
	}

	model := Model{
		viewport:  vp,
		input:     ta,
		rt:        opts.Runtime,
		cwd:       opts.CWD,
		sessionID: opts.SessionID,
		ctx:       ctx,
		loading:   opts.SessionID != "",
	}
	if model.loading {
		model.input.Blur()
	}
	return model
}

// Run starts the TUI program and blocks until the user quits.
func Run(ctx context.Context, opts Options) error {
	opts.context = ctx
	p := tea.NewProgram(New(opts), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// Init starts the cursor blink cycle.
func (m Model) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(textarea.Blink, loadSession(m.ctx, m.rt, m.sessionID))
	}
	return textarea.Blink
}

// Update handles terminal events, user input, and agent events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.input.MaxHeight = max(min(maxComposerHeight, msg.Height-2), 1)
		m.input.SetWidth(msg.Width)
		m.viewport.SetWidth(msg.Width)
		m.rebuild()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.turnActive && m.turnCancel != nil {
				m.turnCancel()
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			if m.turnCancel != nil {
				m.turnCancel()
			}
			if m.turnAbandon != nil {
				m.turnAbandon()
			}
			return m, tea.Quit
		case "pgup", "pgdown":
			m.viewport, _ = m.viewport.Update(msg)
			return m, nil
		case "enter":
			if m.turnActive || m.loading {
				return m, nil // ignore input while running
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			return m.submitTurn(text)
		}
		// Forward all other keypresses to the textarea.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.rebuild()
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.PasteMsg:
		if m.turnActive || m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.rebuild()
		return m, cmd

	case turnUpdateMsg:
		if msg.event != nil {
			m.handleAgentEvent(*msg.event)
		}
		if msg.done != nil {
			m.handleTurnDone(*msg.done)
		}
		m.rebuild()
		if msg.done != nil {
			return m, nil
		}
		return m, pollTurnUpdates(m.eventCh)

	case sessionLoadedMsg:
		m.loading = false
		if msg.err != nil {
			failed := newAssistantMessage()
			failed.err = msg.err
			m.messages = append(m.messages, failed)
		} else {
			m.messages = messagesFromTranscript(msg.messages)
		}
		m.input.Focus()
		m.rebuild()
		return m, nil

	case cursor.BlinkMsg:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// submitTurn starts a RunTurn goroutine with an Observer that writes to a channel.
func (m Model) submitTurn(text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, newUserMessage(text))
	m.appendAssistantMessage()

	m.turnActive = true
	m.input.Blur()

	ch := make(chan turnUpdateMsg, 64)
	m.eventCh = ch

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	deliveryCtx, abandon := context.WithCancel(m.ctx)
	m.turnAbandon = abandon

	opts := runtime.TurnOptions{
		SessionID: m.sessionID,
		Prompt:    text,
		CWD:       m.cwd,
		Observer: func(e agent.Event) {
			event := e
			select {
			case ch <- turnUpdateMsg{event: &event}:
			case <-deliveryCtx.Done():
			}
		},
	}

	m.viewport.GotoBottom()
	m.rebuild()
	cmd := tea.Batch(
		pollTurnUpdates(ch),
		func() tea.Msg {
			result, err := m.rt.RunTurn(ctx, opts)
			done := turnDoneMsg{result: result, err: err}
			select {
			case ch <- turnUpdateMsg{done: &done}:
			case <-deliveryCtx.Done():
			}
			close(ch)
			return nil
		},
	)
	return m, cmd
}

// pollTurnUpdates reads one ordered event or completion update from a turn.
func pollTurnUpdates(ch <-chan turnUpdateMsg) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return update
	}
}

// handleAgentEvent updates conversation state from an agent event.
func (m *Model) handleAgentEvent(e agent.Event) {
	switch e.Type {
	case agent.EventTurnStarted:
		// Nothing to do; assistant message already created.
	case agent.EventModelDelta:
		if m.current == nil || len(m.current.toolCalls) > 0 {
			m.appendAssistantMessage()
		}
		m.current.handleEvent(e)
	case agent.EventModelReasoningDelta:
		// Reasoning deltas are intentionally not shown in the conversation.
	case agent.EventModelResponse:
		// If tools follow, the next model delta starts a new message block.
	case agent.EventToolStarted, agent.EventToolFinished:
		if m.current == nil {
			m.appendAssistantMessage()
		}
		m.current.handleEvent(e)
	case agent.EventTurnFinished:
		if e.Err != nil && m.current != nil {
			m.current.err = e.Err
		}
	}
}

func (m *Model) appendAssistantMessage() {
	m.current = newAssistantMessage()
	m.messages = append(m.messages, m.current)
}

// handleTurnDone processes the RunTurn result.
func (m *Model) handleTurnDone(msg turnDoneMsg) {
	if msg.err != nil && m.current != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.current.cancelled = true
		} else if m.current.err == nil {
			m.current.err = msg.err
		}
	}

	m.turnActive = false
	m.turnCancel = nil
	if m.turnAbandon != nil {
		m.turnAbandon()
	}
	m.turnAbandon = nil
	m.eventCh = nil
	m.current = nil

	if msg.err == nil && msg.result.SessionID != "" {
		m.sessionID = msg.result.SessionID
	}

	m.input.Focus()
}

// View renders the terminal screen.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}

	vpView := m.viewport.View()
	layout := calculateLayout(m.height, m.input.Height())
	parts := make([]string, 0, 4)
	cursorY := 0
	if layout.showHeader {
		header := m.headerView()
		parts = append(parts, header)
		cursorY += lipgloss.Height(header)
	}
	if vpView != "" {
		parts = append(parts, vpView)
		cursorY += lipgloss.Height(vpView)
	}
	if layout.showDivider {
		divider := dividerStyle.Render(strings.Repeat("─", max(m.width, 0)))
		parts = append(parts, divider)
		cursorY += lipgloss.Height(divider)
	}
	parts = append(parts, m.input.View())

	v := tea.NewView(strings.Join(parts, "\n"))

	c := m.input.Cursor()
	if c != nil {
		c.Y += cursorY
	}
	v.Cursor = c
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}

// rebuild refreshes viewport content and recalculates the layout split.
func (m *Model) rebuild() {
	if !m.ready || m.height == 0 {
		return
	}

	followBottom := m.viewport.AtBottom()
	var parts []string
	for _, msg := range m.messages {
		rendered := msg.render(m.width)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	if len(parts) == 0 {
		m.viewport.SetContent("")
	} else {
		m.viewport.SetContent(strings.Join(parts, "\n\n"))
	}

	layout := calculateLayout(m.height, m.input.Height())
	m.viewport.SetHeight(layout.viewportHeight)
	if followBottom {
		m.viewport.GotoBottom()
	}
}

type screenLayout struct {
	viewportHeight int
	showHeader     bool
	showDivider    bool
}

// calculateLayout reserves stable header and composer rows before the viewport.
func calculateLayout(totalHeight, inputHeight int) screenLayout {
	remaining := max(totalHeight-inputHeight, 0)
	layout := screenLayout{showHeader: remaining > 0}
	if layout.showHeader {
		remaining--
	}
	layout.showDivider = remaining > 1
	if layout.showDivider {
		remaining--
	}
	layout.viewportHeight = remaining
	return layout
}

func (m Model) headerView() string {
	line := headerStyle.Render("Atlas")
	status := ""
	switch {
	case m.loading:
		status = "loading"
	case m.turnActive:
		status = "working"
	case m.sessionID != "":
		status = "session " + shortSessionID(m.sessionID)
	}
	if status != "" {
		line += mutedStyle.Render("  ·  " + status)
	}
	if m.cwd != "" {
		line += mutedStyle.Render("  ·  " + ansi.Strip(m.cwd))
	}
	return ansi.Truncate(line, max(m.width, 0), "…")
}

func shortSessionID(sessionID string) string {
	const maxLength = 12
	if len(sessionID) <= maxLength {
		return sessionID
	}
	return sessionID[:maxLength] + "…"
}

func loadSession(ctx context.Context, rt *runtime.Runtime, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_, transcript, err := rt.ShowSession(ctx, sessionID)
		if runtime.IsSessionNotFound(err) {
			return sessionLoadedMsg{}
		}
		if err != nil {
			return sessionLoadedMsg{err: err}
		}
		return sessionLoadedMsg{messages: transcript.Messages()}
	}
}
