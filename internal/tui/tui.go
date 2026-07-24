// Package tui implements the Atlas interactive terminal interface.
package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
)

const (
	maxComposerHeight = 10
	statusIndent      = "  "
)

// Options configures the TUI at startup.
type Options struct {
	Runtime   *runtime.Runtime
	SessionID string
	CWD       string
	context   context.Context
}

// Model is the top-level TUI model managing layout, input, and content display.
type Model struct {
	width              int
	height             int
	ready              bool
	hasDarkBackground  bool
	terminalBackground color.Color

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

	// Footer status reflects the model selection used for subsequent turns.
	models          []runtime.ModelOption
	modelValue      string
	modelName       string
	reasoningEffort string
	contextTokens   int
	contextWindow   int
	modelStatusErr  error
	modelPicker     modelPicker
	resumePicker    sessionPicker
	slashPopup      slashPopup
	filePicker      fileMentionPicker

	// Turn state.
	turnActive  bool
	turnCancel  context.CancelFunc
	turnAbandon context.CancelFunc
	eventCh     chan turnUpdateMsg
	selection   textSelection
	turnStatus  turnStatus

	// Manual compaction runs outside the agent turn loop.
	compactActive bool
	compactCancel context.CancelFunc

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
	ta.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")

	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Cursor.Shape = tea.CursorBar
	ta.SetStyles(s)

	ctx := opts.context
	if ctx == nil {
		ctx = context.Background()
	}

	model := Model{
		viewport:   vp,
		input:      ta,
		rt:         opts.Runtime,
		cwd:        opts.CWD,
		sessionID:  opts.SessionID,
		ctx:        ctx,
		loading:    opts.SessionID != "",
		slashPopup: newSlashPopup(),
		turnStatus: newTurnStatus(),
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

// Init starts the cursor blink cycle and loads TUI metadata.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.RequestBackgroundColor,
		textarea.Blink,
		loadModelStatus(m.ctx, m.rt),
		loadSkillSummaries(m.ctx, m.rt, m.cwd),
	}
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
		m.terminalBackground = msg.Color
		m.rebuild()
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()
		if m.resumePicker.active() {
			return m.handleResumePickerKey(msg)
		}
		if m.filePicker.browsing() {
			return m.handleFileBrowserKey(msg)
		}
		if handled, cmd := m.handleFileMentionKey(msg); handled {
			return m, cmd
		}
		switch key {
		case "ctrl+c":
			return m, nil
		case "esc":
			if m.modelPicker.active() {
				m.modelPicker.close()
				m.input.Focus()
				m.rebuild()
			} else if m.slashPopup.active() {
				m.slashPopup.dismiss(m.input.Value())
				m.rebuild()
			} else if m.turnActive && m.turnCancel != nil {
				m.turnCancel()
			} else if m.compactActive && m.compactCancel != nil {
				m.compactCancel()
			}
			return m, nil
		}
		if m.modelPicker.active() {
			selection := m.modelPicker.update(key)
			if selection != nil {
				m.applyModelSelection(*selection)
			} else if !m.modelPicker.active() {
				m.input.Focus()
			}
			m.rebuild()
			return m, nil
		}
		if m.slashPopup.active() {
			switch key {
			case "up":
				m.slashPopup.move(-1)
				m.rebuild()
				return m, nil
			case "down":
				m.slashPopup.move(1)
				m.rebuild()
				return m, nil
			case "tab", "enter":
				if command, ok := m.slashPopup.selectedCommand(); ok {
					if replaceSlashCompletion(&m.input, m.slashPopup.target, command.name) {
						m.slashPopup.dismiss(m.input.Value())
						m.rebuild()
					}
				}
				return m, nil
			}
		}
		switch key {
		case "pgup", "pgdown":
			m.viewport, _ = m.viewport.Update(msg)
			return m, nil
		case "enter":
			if m.turnActive || m.compactActive || m.loading {
				return m, nil // ignore input while running
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			if text == "/quit" {
				return m, tea.Quit
			}
			if sessionID, ok := resumeCommandSessionID(text); ok {
				m.input.Reset()
				m.slashPopup.sync(m.input)
				m.filePicker.reset()
				if sessionID != "" {
					if err := session.ValidateID(sessionID); err != nil {
						m.messages = append(m.messages, newNoticeMessage("Resume failed: "+err.Error(), true))
						m.rebuild()
						return m, nil
					}
					if sessionID == m.sessionID {
						m.messages = append(m.messages, newNoticeMessage("Already in this session.", false))
						m.rebuild()
						return m, nil
					}
					generation := m.resumePicker.openDirect(m.cwd, m.sessionID, time.Now())
					m.selection = textSelection{}
					m.input.Blur()
					m.rebuild()
					return m, loadResumedSession(m.ctx, m.rt, sessionID, generation)
				}
				generation := m.resumePicker.open(m.cwd, m.sessionID, time.Now())
				m.selection = textSelection{}
				m.input.Blur()
				m.rebuild()
				return m, loadSessionPage(m.ctx, m.rt, m.cwd, m.resumePicker.scope, "", generation)
			}
			if instruction, ok := compactCommandInstruction(text); ok {
				m.input.Reset()
				m.slashPopup.sync(m.input)
				m.filePicker.reset()
				return m.submitCompaction(instruction)
			}
			if text == "/model" {
				if len(m.models) == 0 {
					return m, nil
				}
				m.input.Reset()
				m.slashPopup.sync(m.input)
				m.filePicker.reset()
				m.openModelPicker()
				return m, nil
			}
			m.input.Reset()
			m.slashPopup.sync(m.input)
			m.filePicker.reset()
			return m.submitTurn(text)
		}
		// Forward all other keypresses to the textarea.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.slashPopup.sync(m.input)
		fileCmd := m.syncFileMention()
		m.rebuild()
		return m, tea.Batch(cmd, fileCmd)

	case tea.MouseClickMsg:
		if m.resumePicker.active() || m.filePicker.browsing() {
			return m, nil
		}
		if msg.Button == tea.MouseLeft && msg.Y >= 0 && msg.Y < m.viewport.Height() {
			m.selection.begin(selectionPoint{x: msg.X, y: msg.Y}, m.viewport.AtBottom())
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.resumePicker.active() || m.filePicker.browsing() {
			return m, nil
		}
		if m.selection.active {
			m.selection.move(selectionPoint{x: msg.X, y: msg.Y})
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.resumePicker.active() || m.filePicker.browsing() {
			return m, nil
		}
		if !m.selection.active {
			return m, nil
		}
		m.selection.move(selectionPoint{x: msg.X, y: msg.Y})
		text := m.selection.content(m.viewport.View())
		if text == "" {
			m.clearSelection(true)
			return m, nil
		}
		return m, func() tea.Msg { return copySelectionMsg{text: text} }

	case tea.MouseWheelMsg:
		if m.resumePicker.active() {
			delta := 0
			switch msg.Button {
			case tea.MouseWheelUp:
				delta = -3
			case tea.MouseWheelDown:
				delta = 3
			}
			m.resumePicker.move(delta)
			m.rebuild()
			if delta > 0 {
				return m, m.loadMoreResumeSessionsIfNeeded()
			}
			return m, nil
		}
		if m.filePicker.browsing() {
			return m, nil
		}
		m.selection = textSelection{}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.PasteMsg:
		if m.resumePicker.active() {
			if m.resumePicker.stage == sessionPickerList {
				m.resumePicker.appendQuery(msg.Content)
				m.rebuild()
			}
			return m, m.continueResumeSearchIfNeeded()
		}
		if m.filePicker.browsing() {
			return m, nil
		}
		if m.turnActive || m.compactActive || m.loading || m.modelPicker.active() {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.slashPopup.sync(m.input)
		fileCmd := m.syncFileMention()
		m.rebuild()
		return m, tea.Batch(cmd, fileCmd)

	case copySelectionMsg:
		m.clearSelection(true)
		return m, tea.SetClipboard(msg.text)

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

	case compactDoneMsg:
		m.handleCompactDone(msg)
		m.rebuild()
		return m, nil

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

	case sessionPageLoadedMsg:
		if !m.resumePicker.active() || msg.generation != m.resumePicker.generation {
			return m, nil
		}
		if msg.err != nil {
			m.resumePicker.pageLoading = false
			m.resumePicker.err = "Failed to load sessions: " + msg.err.Error()
			m.rebuild()
			return m, nil
		} else {
			m.resumePicker.appendPage(msg.page)
		}
		m.rebuild()
		return m, m.continueResumeSearchIfNeeded()

	case resumedSessionLoadedMsg:
		if !m.resumePicker.active() || msg.generation != m.resumePicker.generation {
			return m, nil
		}
		if msg.err != nil {
			direct := m.resumePicker.direct
			m.resumePicker.failLoad(msg.err)
			if direct {
				m.messages = append(m.messages, newNoticeMessage("Resume failed: "+msg.err.Error(), true))
				m.input.Focus()
			}
			m.rebuild()
			return m, nil
		}
		if !m.resumePicker.sameCWD(msg.session.info.CWD) {
			m.resumePicker.confirm(msg.session)
			m.rebuild()
			return m, nil
		}
		cmd := m.applyResumedSession(msg.session)
		m.rebuild()
		return m, cmd

	case fileCatalogLoadedMsg:
		if m.filePicker.acceptCatalog(msg) {
			m.rebuild()
		}
		return m, nil

	case modelStatusLoadedMsg:
		m.models = msg.models
		m.modelValue = msg.modelValue
		m.modelName = msg.modelName
		m.reasoningEffort = msg.reasoningEffort
		m.contextWindow = msg.contextWindow
		m.modelStatusErr = msg.err
		m.rebuild()
		return m, nil

	case skillSummariesLoadedMsg:
		if msg.cwd != m.cwd {
			return m, nil
		}
		if msg.err == nil {
			m.slashPopup.setSkills(msg.summaries)
			m.slashPopup.sync(m.input)
		}
		m.rebuild()
		return m, nil

	case spinner.TickMsg:
		return m, m.turnStatus.update(msg)

	case cursor.BlinkMsg:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if m.filePicker.browsing() {
		cmd, path, selected := m.filePicker.updateBrowser(msg)
		if selected {
			m.input.Focus()
			m.insertSelectedFile(path)
		}
		m.rebuild()
		return m, cmd
	}

	return m, nil
}

// handleResumePickerKey routes keys while the session picker owns the screen.
func (m Model) handleResumePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	loadAfterNavigation := false
	continueSearch := false
	switch key {
	case "ctrl+c":
		return m, nil
	case "esc":
		m.resumePicker.close()
		m.input.Focus()
		m.rebuild()
		return m, nil
	case "enter":
		switch m.resumePicker.stage {
		case sessionPickerList:
			candidate, ok := m.resumePicker.selectedSession()
			if !ok {
				return m, nil
			}
			m.resumePicker.startSessionLoad()
			m.rebuild()
			return m, loadResumedSession(m.ctx, m.rt, candidate.ID, m.resumePicker.generation)
		case sessionPickerConfirm:
			if m.resumePicker.pending == nil {
				return m, nil
			}
			cmd := m.applyResumedSession(*m.resumePicker.pending)
			m.rebuild()
			return m, cmd
		}
		return m, nil
	case "left", "right":
		if m.resumePicker.stage != sessionPickerList {
			return m, nil
		}
		scope := sessionPickerCWD
		if key == "right" {
			scope = sessionPickerAll
		}
		generation, changed := m.resumePicker.setScope(scope)
		if !changed {
			return m, nil
		}
		m.rebuild()
		return m, loadSessionPage(m.ctx, m.rt, m.cwd, m.resumePicker.scope, "", generation)
	case "up":
		m.resumePicker.move(-1)
	case "down":
		m.resumePicker.move(1)
		loadAfterNavigation = true
	case "pgup":
		m.resumePicker.move(-max((m.height-4)/3, 1))
	case "pgdown":
		m.resumePicker.move(max((m.height-4)/3, 1))
		loadAfterNavigation = true
	case "home":
		m.resumePicker.selectFirst()
	case "end":
		m.resumePicker.selectLast()
		loadAfterNavigation = true
	case "backspace":
		if m.resumePicker.stage == sessionPickerList {
			m.resumePicker.deleteQueryRune()
			continueSearch = true
		}
	default:
		if m.resumePicker.stage == sessionPickerList && msg.Text != "" {
			m.resumePicker.appendQuery(msg.Text)
			continueSearch = true
		}
	}
	m.rebuild()
	if loadAfterNavigation {
		return m, m.loadMoreResumeSessionsIfNeeded()
	}
	if continueSearch {
		return m, m.continueResumeSearchIfNeeded()
	}
	return m, nil
}

func (m *Model) loadMoreResumeSessionsIfNeeded() tea.Cmd {
	if !m.resumePicker.shouldLoadMore() {
		return nil
	}
	return m.loadNextResumePage()
}

func (m *Model) continueResumeSearchIfNeeded() tea.Cmd {
	picker := &m.resumePicker
	if picker.stage != sessionPickerList || picker.query == "" || len(picker.matches) > 0 || picker.pageLoading || picker.nextCursor == "" {
		return nil
	}
	return m.loadNextResumePage()
}

func (m *Model) loadNextResumePage() tea.Cmd {
	m.resumePicker.markPageLoading()
	return loadSessionPage(
		m.ctx,
		m.rt,
		m.cwd,
		m.resumePicker.scope,
		m.resumePicker.nextCursor,
		m.resumePicker.generation,
	)
}

// applyResumedSession replaces conversation state only after loading and directory validation.
func (m *Model) applyResumedSession(resumed resumedSession) tea.Cmd {
	if !m.resumePicker.sameCWD(resumed.info.CWD) {
		if err := validateResumeCWD(resumed.info.CWD); err != nil {
			m.resumePicker.err = err.Error()
			return nil
		}
	}

	m.sessionID = resumed.info.ID
	m.cwd = filepath.Clean(resumed.info.CWD)
	m.messages = messagesFromTranscript(resumed.messages)
	m.contextTokens = resumed.contextTokens
	m.current = nil
	m.selection = textSelection{}
	m.input.Reset()
	m.input.Focus()
	m.resumePicker.close()
	m.slashPopup.setSkills(nil)
	m.filePicker.reset()
	m.rebuild()
	m.viewport.GotoBottom()
	return loadSkillSummaries(m.ctx, m.rt, m.cwd)
}

func validateResumeCWD(cwd string) error {
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("saved working directory is empty")
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return fmt.Errorf("saved working directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("saved working directory is not a directory: %s", cwd)
	}
	return nil
}

// submitTurn starts a RunTurn goroutine with an Observer that writes to a channel.
func (m Model) submitTurn(text string) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, newUserMessage(text))
	m.appendAssistantMessage()

	m.turnActive = true
	statusCmd := m.turnStatus.start(time.Now())
	m.input.Blur()

	ch := make(chan turnUpdateMsg, 64)
	m.eventCh = ch

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	deliveryCtx, abandon := context.WithCancel(m.ctx)
	m.turnAbandon = abandon

	opts := runtime.TurnOptions{
		SessionID:          m.sessionID,
		Prompt:             text,
		Skills:             selectedSkillNames(text),
		Model:              m.modelValue,
		ReasoningEffort:    m.reasoningEffort,
		ReasoningEffortSet: m.reasoningEffort != "",
		CWD:                m.cwd,
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
		statusCmd,
	)
	return m, cmd
}

// submitCompaction starts manual compaction without adding a transcript turn.
func (m Model) submitCompaction(instruction string) (tea.Model, tea.Cmd) {
	if m.sessionID == "" {
		m.messages = append(m.messages, newNoticeMessage("No session to compact.", false))
		m.rebuild()
		return m, nil
	}

	m.compactActive = true
	statusCmd := m.turnStatus.start(time.Now())
	m.input.Blur()
	ctx, cancel := context.WithCancel(m.ctx)
	m.compactCancel = cancel
	m.viewport.GotoBottom()
	m.rebuild()

	compactCmd := func() tea.Msg {
		result, err := m.rt.CompactSession(ctx, runtime.CompactOptions{
			SessionID:          m.sessionID,
			Model:              m.modelValue,
			ReasoningEffort:    m.reasoningEffort,
			ReasoningEffortSet: m.reasoningEffort != "",
			CWD:                m.cwd,
			Instruction:        instruction,
		})
		return compactDoneMsg{result: result, err: err}
	}
	return m, tea.Batch(compactCmd, statusCmd)
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
		m.turnStatus.setPhase(turnPhaseWorking)
	case agent.EventModelDelta:
		m.turnStatus.setPhase(turnPhaseWorking)
		if m.current == nil || len(m.current.toolCalls) > 0 {
			m.appendAssistantMessage()
		}
		m.current.handleEvent(e)
	case agent.EventModelReasoningDelta:
		m.turnStatus.setPhase(turnPhaseThinking)
		// Reasoning deltas are intentionally not shown in the conversation.
	case agent.EventModelResponse:
		m.turnStatus.setPhase(turnPhaseWorking)
		// If tools follow, the next model delta starts a new message block.
	case agent.EventToolStarted, agent.EventToolFinished:
		m.turnStatus.setPhase(turnPhaseWorking)
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

// openModelPicker replaces the composer with the configured model selector.
func (m *Model) openModelPicker() {
	m.modelPicker.open(m.models, m.modelValue, m.reasoningEffort)
	if m.modelPicker.active() {
		m.input.Blur()
		m.rebuild()
	}
}

// applyModelSelection updates the footer and subsequent turn overrides together.
func (m *Model) applyModelSelection(selection modelSelection) {
	m.modelValue = selection.model.Value
	m.modelName = modelOptionName(selection.model)
	m.reasoningEffort = selection.effort
	m.contextWindow = selection.model.ContextWindow
	m.modelStatusErr = nil
	m.input.Focus()
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
	m.turnStatus.stop()
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

// handleCompactDone restores input state and reports the non-transcript result.
func (m *Model) handleCompactDone(msg compactDoneMsg) {
	m.compactActive = false
	m.turnStatus.stop()
	if m.compactCancel != nil {
		m.compactCancel()
	}
	m.compactCancel = nil

	switch {
	case errors.Is(msg.err, context.Canceled):
		m.messages = append(m.messages, newNoticeMessage("Compaction cancelled.", false))
	case msg.err != nil:
		m.messages = append(m.messages, newNoticeMessage("Compaction failed: "+msg.err.Error(), true))
	case !msg.result.Compacted:
		m.messages = append(m.messages, newNoticeMessage("No safe context to compact.", false))
	default:
		m.contextTokens = msg.result.TokensAfter
		if msg.result.ContextWindow > 0 {
			m.contextWindow = msg.result.ContextWindow
		}
		m.messages = append(m.messages, newNoticeMessage(fmt.Sprintf("Context compacted. Kept %d recent messages.", msg.result.KeepCount), false))
	}
	m.input.Focus()
}

// View renders the terminal screen.
func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("")
	}
	if m.resumePicker.active() {
		rendered := m.resumePicker.render(m.width, m.height)
		v := tea.NewView(rendered.content)
		if rendered.showCursor {
			v.Cursor = tea.NewCursor(rendered.cursorX, rendered.cursorY)
			v.Cursor.Shape = tea.CursorBar
		}
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	vpView := m.selection.render(m.viewport.View(), selectionStyle)
	composerState := m.renderInputArea()
	layout := calculateLayout(m.height, composerState.height, m.turnStatus.active())
	parts := make([]string, 0, 6)
	cursorY := 0
	if vpView != "" {
		parts = append(parts, vpView)
		cursorY += lipgloss.Height(vpView)
	}
	if layout.showInputGap {
		parts = append(parts, "")
		cursorY++
	}
	if layout.showTurnStatus {
		parts = append(parts, m.turnStatus.viewAt(m.width, time.Now()))
		cursorY++
	}
	composer := composerStyle(m.hasDarkBackground, m.terminalBackground).
		Width(m.width).
		Render(composerState.content)
	parts = append(parts, composer)
	if layout.showStatus {
		parts = append(parts, m.statusView())
	}

	v := tea.NewView(strings.Join(parts, "\n"))

	c := m.input.Cursor()
	if m.modelPicker.active() || m.filePicker.browsing() {
		c = nil
	}
	if c != nil {
		c.X = 2 + composerState.cursorColumn
		c.Y = cursorY + 1 + composerState.cursorRow
	}
	v.Cursor = c
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}

// clearSelection resets the drag and optionally restores automatic following.
func (m *Model) clearSelection(resumeFollow bool) {
	shouldFollow := resumeFollow && m.selection.resumeFollow
	m.selection = textSelection{}
	if shouldFollow {
		m.viewport.GotoBottom()
	}
}

// rebuild refreshes viewport content and recalculates the layout split.
func (m *Model) rebuild() {
	if !m.ready || m.height == 0 {
		return
	}

	followBottom := m.viewport.AtBottom() && !m.selection.active
	var parts []string
	for _, msg := range m.messages {
		rendered := msg.render(m.width, m.hasDarkBackground, m.terminalBackground)
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	if len(parts) == 0 {
		m.viewport.SetContent("")
	} else {
		m.viewport.SetContent(strings.Join(parts, "\n\n"))
	}

	layout := calculateLayout(m.height, m.renderInputArea().height, m.turnStatus.active())
	m.viewport.SetHeight(layout.viewportHeight)
	if followBottom {
		m.viewport.GotoBottom()
	}
}

type screenLayout struct {
	viewportHeight int
	showInputGap   bool
	showStatus     bool
	showTurnStatus bool
}

// calculateLayout keeps the composer and footer stable while preserving history space.
func calculateLayout(totalHeight, composerHeight int, turnActive bool) screenLayout {
	const composerVerticalPadding = 2
	remaining := max(totalHeight-composerHeight-composerVerticalPadding, 0)
	layout := screenLayout{showTurnStatus: turnActive && remaining > 0}
	if layout.showTurnStatus {
		remaining--
	}
	layout.showStatus = remaining > 0
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

func (m Model) renderInputArea() composerRender {
	if m.modelPicker.active() {
		return m.modelPicker.render(m.width, m.input.MaxHeight)
	}
	composer := m.renderComposer()
	popupRows := min(maxSlashPopupRows, max(m.height-composer.height-3, 0))
	popup := m.filePicker.render(
		max(m.width-1, 1),
		popupRows,
		userMessageBackground(m.hasDarkBackground, m.terminalBackground),
	)
	if popup == "" {
		popup = m.slashPopup.render(max(m.width-1, 1), popupRows)
	}
	if popup == "" {
		return composer
	}
	popupHeight := lipgloss.Height(popup)
	composer.content = popup + "\n\n" + composer.content
	composer.height += popupHeight + 1
	composer.cursorRow += popupHeight + 1
	return composer
}

func (m Model) renderComposer() composerRender {
	contentWidth := max(m.width-3, 1) // right padding and the two-cell prompt
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
		return ansi.Truncate(statusIndent+errorStyle.Render("Model unavailable"), max(m.width, 0), "…")
	}
	if m.modelName == "" {
		return ansi.Truncate(statusIndent+mutedStyle.Render("Loading model…"), max(m.width, 0), "…")
	}

	modelStatus := m.modelName
	if m.reasoningEffort != "" {
		modelStatus += " " + m.reasoningEffort
	}
	line := statusIndent + userStyle.Render(modelStatus)
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
			status := modelStatusLoadedMsg{
				models:        options.Models,
				modelValue:    option.Value,
				modelName:     modelOptionName(option),
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

func loadSkillSummaries(ctx context.Context, rt *runtime.Runtime, cwd string) tea.Cmd {
	return func() tea.Msg {
		summaries, err := rt.SkillSummaries(ctx, cwd)
		return skillSummariesLoadedMsg{cwd: cwd, summaries: summaries, err: err}
	}
}

// loadResumedSession retrieves metadata and transcript as one atomic picker result.
func loadResumedSession(ctx context.Context, rt *runtime.Runtime, sessionID string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		info, transcript, err := rt.ShowSession(ctx, sessionID)
		if err != nil {
			return resumedSessionLoadedMsg{generation: generation, err: err}
		}
		return resumedSessionLoadedMsg{
			generation: generation,
			session: resumedSession{
				info:     info,
				messages: transcript.Messages(),
				contextTokens: usageTokens(model.Usage{
					InputTokens:  info.LastInputTokens,
					OutputTokens: info.LastOutputTokens,
					TotalTokens:  info.LastTotalTokens,
				}),
			},
		}
	}
}
