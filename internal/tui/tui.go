// Package tui implements the Atlas interactive terminal interface.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
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
	width             int
	height            int
	ready             bool
	hasDarkBackground bool

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

	// Footer status reflects the runtime's active default model.
	modelName       string
	reasoningEffort string
	contextTokens   int
	contextWindow   int
	modelStatusErr  error

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

// Init starts the cursor blink cycle and loads footer metadata.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, loadModelStatus(m.ctx, m.rt)}
	if m.loading {
		cmds = append(cmds, loadSession(m.ctx, m.rt, m.sessionID))
	}
	return tea.Batch(cmds...)
}

// Update handles terminal events, user input, and agent events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.input.MaxHeight = max(min(maxComposerHeight, msg.Height-5), 1)
		m.input.SetWidth(max(msg.Width-2, 1))
		m.viewport.SetWidth(msg.Width)
		m.rebuild()
		return m, nil

	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
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
			m.contextTokens = msg.contextTokens
		}
		m.input.Focus()
		m.rebuild()
		return m, nil

	case modelStatusLoadedMsg:
		m.modelName = msg.modelName
		m.reasoningEffort = msg.reasoningEffort
		m.contextWindow = msg.contextWindow
		m.modelStatusErr = msg.err
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
	if msg.err == nil && msg.result.ContextWindow > 0 {
		m.contextTokens = usageTokens(msg.result.Usage)
		m.contextWindow = msg.result.ContextWindow
	}

	m.input.Focus()
}

// View renders the terminal screen.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}

	vpView := m.viewport.View()
	composerState := m.renderComposer()
	layout := calculateLayout(m.height, composerState.height)
	parts := make([]string, 0, 5)
	cursorY := 0
	if vpView != "" {
		parts = append(parts, vpView)
		cursorY += lipgloss.Height(vpView)
	}
	if layout.showInputGap {
		parts = append(parts, "")
		cursorY++
	}
	composer := userMessageStyle(m.hasDarkBackground).
		Width(m.width).
		Render(composerState.content)
	parts = append(parts, composer)
	if layout.showStatus {
		parts = append(parts, m.statusView())
	}

	v := tea.NewView(strings.Join(parts, "\n"))

	c := m.input.Cursor()
	if c != nil {
		c.X = 3 + composerState.cursorColumn
		c.Y = cursorY + 1 + composerState.cursorRow
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
		rendered := msg.render(m.width, m.hasDarkBackground)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	if len(parts) == 0 {
		m.viewport.SetContent("")
	} else {
		m.viewport.SetContent(strings.Join(parts, "\n\n"))
	}

	layout := calculateLayout(m.height, m.renderComposer().height)
	m.viewport.SetHeight(layout.viewportHeight)
	if followBottom {
		m.viewport.GotoBottom()
	}
}

type screenLayout struct {
	viewportHeight int
	showInputGap   bool
	showStatus     bool
}

// calculateLayout keeps the composer and footer stable while preserving history space.
func calculateLayout(totalHeight, composerHeight int) screenLayout {
	const composerVerticalPadding = 2
	remaining := max(totalHeight-composerHeight-composerVerticalPadding, 0)
	layout := screenLayout{showStatus: remaining > 0}
	if layout.showStatus {
		remaining--
	}
	layout.showInputGap = remaining > 1
	if layout.showInputGap {
		remaining--
	}
	layout.viewportHeight = remaining
	return layout
}

type composerRender struct {
	content      string
	height       int
	cursorRow    int
	cursorColumn int
}

func (m Model) renderComposer() composerRender {
	contentWidth := max(m.width-4, 1) // outer padding and the two-cell prompt
	maxRows := max(m.input.MaxHeight, 1)
	cursorInfo := m.input.LineInfo()
	cursorColumn := cursorInfo.StartColumn + cursorInfo.ColumnOffset
	return renderComposerValue(m.input.Value(), contentWidth, maxRows, m.input.Line(), cursorColumn)
}

// renderComposerValue hard-wraps the draft and keeps the cursor inside the visible row window.
func renderComposerValue(value string, width, maxRows, cursorLine, cursorColumn int) composerRender {
	logicalLines := strings.Split(value, "\n")
	cursorLine = min(max(cursorLine, 0), len(logicalLines)-1)

	var lines []string
	cursorRow := 0
	cursorCell := 0
	for i, line := range logicalLines {
		trackedColumn := -1
		if i == cursorLine {
			trackedColumn = cursorColumn
		}
		wrapped, row, column := wrapComposerLine(line, width, trackedColumn)
		if i == cursorLine {
			cursorRow = len(lines) + row
			cursorCell = column
		}
		lines = append(lines, wrapped...)
	}

	maxRows = max(maxRows, 1)
	start := 0
	if len(lines) > maxRows {
		start = min(max(cursorRow-maxRows+1, 0), len(lines)-maxRows)
	}
	end := min(start+maxRows, len(lines))
	visible := lines[start:end]
	rendered := make([]string, 0, len(visible))
	for i, line := range visible {
		prefix := "  "
		if start+i == 0 {
			prefix = "› "
		}
		rendered = append(rendered, prefix+line)
	}

	return composerRender{
		content:      strings.Join(rendered, "\n"),
		height:       max(len(visible), 1),
		cursorRow:    cursorRow - start,
		cursorColumn: cursorCell,
	}
}

// wrapComposerLine wraps one logical line by terminal cell width and maps its cursor position.
func wrapComposerLine(line string, width, cursorColumn int) ([]string, int, int) {
	width = max(width, 1)
	runes := []rune(line)
	if cursorColumn >= 0 {
		cursorColumn = min(max(cursorColumn, 0), len(runes))
	}

	lines := make([]string, 0, 1)
	var chunk strings.Builder
	used := 0
	cursorRow := 0
	cursorCell := 0
	for i, r := range runes {
		cellWidth := lipgloss.Width(string(r))
		if used > 0 && used+cellWidth > width {
			lines = append(lines, chunk.String())
			chunk.Reset()
			used = 0
		}
		if i == cursorColumn {
			cursorRow = len(lines)
			cursorCell = used
		}
		chunk.WriteRune(r)
		used += cellWidth
	}
	if cursorColumn == len(runes) {
		cursorRow = len(lines)
		cursorCell = used
	}
	lines = append(lines, chunk.String())
	if cursorColumn == len(runes) && cursorCell == width {
		lines = append(lines, "")
		cursorRow++
		cursorCell = 0
	}
	return lines, cursorRow, cursorCell
}

func (m Model) statusView() string {
	if m.modelStatusErr != nil {
		return ansi.Truncate(errorStyle.Render("Model unavailable"), max(m.width, 0), "…")
	}
	if m.modelName == "" {
		return ansi.Truncate(mutedStyle.Render("Loading model…"), max(m.width, 0), "…")
	}

	modelStatus := m.modelName
	if m.reasoningEffort != "" {
		modelStatus += " " + m.reasoningEffort
	}
	line := userStyle.Render(modelStatus)
	line += mutedStyle.Render(" · ")
	line += assistantStyle.Render(fmt.Sprintf("Context %d%% used", contextUsagePercent(m.contextTokens, m.contextWindow)))
	return ansi.Truncate(line, max(m.width, 0), "…")
}

func contextUsagePercent(tokens, contextWindow int) int {
	if tokens <= 0 || contextWindow <= 0 {
		return 0
	}
	return min(tokens*100/contextWindow, 100)
}

func usageTokens(usage model.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens
}

func loadSession(ctx context.Context, rt *runtime.Runtime, sessionID string) tea.Cmd {
	return func() tea.Msg {
		info, transcript, err := rt.ShowSession(ctx, sessionID)
		if runtime.IsSessionNotFound(err) {
			return sessionLoadedMsg{}
		}
		if err != nil {
			return sessionLoadedMsg{err: err}
		}
		return sessionLoadedMsg{
			messages: transcript.Messages(),
			contextTokens: usageTokens(model.Usage{
				InputTokens:  info.LastInputTokens,
				OutputTokens: info.LastOutputTokens,
				TotalTokens:  info.LastTotalTokens,
			}),
		}
	}
}

func loadModelStatus(ctx context.Context, rt *runtime.Runtime) tea.Cmd {
	return func() tea.Msg {
		options, err := rt.ModelOptions(ctx)
		if err != nil {
			return modelStatusLoadedMsg{err: err}
		}
		for _, option := range options.Models {
			if option.Value != options.Default {
				continue
			}
			name := option.Name
			if name == "" {
				name = option.Value
			}
			status := modelStatusLoadedMsg{
				modelName:     name,
				contextWindow: option.ContextWindow,
			}
			if len(option.ReasoningEfforts) > 0 {
				status.reasoningEffort = option.ReasoningEfforts[0].Value
			}
			return status
		}
		return modelStatusLoadedMsg{err: fmt.Errorf("default model %q is unavailable", options.Default)}
	}
}
