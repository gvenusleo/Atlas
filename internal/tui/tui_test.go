package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/skill"
	"github.com/liuyuxin/atlas/internal/tool"
)

func TestInitRequestsTerminalBackgroundColor(t *testing.T) {
	m := New(Options{})
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) < 1 {
		t.Fatalf("Init() message = %T, want non-empty tea.BatchMsg", m.Init()())
	}
	if got := reflect.TypeOf(batch[0]()).Name(); got != "backgroundColorMsg" {
		t.Fatalf("first Init command returns %q, want backgroundColorMsg", got)
	}
}

func TestBackgroundColorMessageSelectsDarkTheme(t *testing.T) {
	m := New(Options{})
	m.messages = append(m.messages, newAssistantMessage())
	m.messages[0].content.WriteString("## Heading")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = updated.(Model)
	light := m.viewport.View()

	updated, _ = m.Update(tea.BackgroundColorMsg{Color: color.Black})
	m = updated.(Model)
	if !m.hasDarkBackground {
		t.Fatal("dark terminal background did not select the dark theme")
	}
	if dark := m.viewport.View(); dark == light {
		t.Fatal("dark terminal background did not rerender themed Markdown")
	}
}

func TestEmptyConversationUsesFullTerminalHeight(t *testing.T) {
	m := New(Options{CWD: "/work/atlas"})
	m.modelName = "gpt-5.6-sol"
	m.reasoningEffort = "high"
	m.contextTokens = 790
	m.contextWindow = 1000
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	if got := lipgloss.Height(m.View().Content); got != 24 {
		t.Fatalf("View height = %d, want 24", got)
	}
	if got := m.viewport.Height(); got != 19 {
		t.Fatalf("viewport height = %d, want 19", got)
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if got := lines[len(lines)-1]; got != "  gpt-5.6-sol high · Context 79% used" {
		t.Fatalf("footer = %q", got)
	}
	if content := ansi.Strip(m.viewport.View()); !strings.Contains(content, "Atlas  v") ||
		!strings.Contains(content, "cwd    /work/atlas") ||
		!strings.Contains(content, "model  gpt-5.6-sol high") {
		t.Fatalf("welcome content = %q", content)
	}
}

func TestWelcomeAdaptsToNarrowTerminal(t *testing.T) {
	m := New(Options{CWD: "/a/very/long/path/to/atlas"})
	m.modelName = "gpt-5.6-sol"
	m.reasoningEffort = "high"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: 14})
	m = updated.(Model)

	rendered := ansi.Strip(m.viewport.View())
	if strings.Contains(rendered, "▟█████▙") {
		t.Fatalf("narrow welcome still contains the large mark: %q", rendered)
	}
	if !strings.Contains(rendered, "Atlas  v") || !strings.Contains(rendered, "gpt-5.6-sol high") {
		t.Fatalf("narrow welcome content = %q", rendered)
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if width := lipgloss.Width(line); width > 24 {
			t.Fatalf("narrow welcome line width = %d, line = %q", width, line)
		}
	}
}

func TestWelcomeUpdatesWhenModelStatusLoads(t *testing.T) {
	m := New(Options{CWD: "/work"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m = updated.(Model)
	if rendered := ansi.Strip(m.viewport.View()); !strings.Contains(rendered, "model  loading...") {
		t.Fatalf("loading welcome = %q", rendered)
	}

	updated, _ = m.Update(modelStatusLoadedMsg{modelName: "Model A", reasoningEffort: "high"})
	m = updated.(Model)
	if rendered := ansi.Strip(m.viewport.View()); !strings.Contains(rendered, "model  Model A high") {
		t.Fatalf("loaded welcome = %q", rendered)
	}
}

func TestWelcomeUpdatesWhenSkillSummariesLoad(t *testing.T) {
	m := New(Options{CWD: "/work"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m = updated.(Model)
	if rendered := ansi.Strip(m.viewport.View()); !strings.Contains(rendered, "skills loading...") {
		t.Fatalf("loading welcome = %q", rendered)
	}

	updated, _ = m.Update(skillSummariesLoadedMsg{
		cwd: "/work",
		summaries: []runtime.SkillSummary{
			{Name: "check"},
			{Name: "think"},
		},
	})
	m = updated.(Model)
	if rendered := ansi.Strip(m.viewport.View()); !strings.Contains(rendered, "skills 2 enabled") {
		t.Fatalf("loaded welcome = %q", rendered)
	}
}

func TestWelcomeLabelsUseThemeMutedColor(t *testing.T) {
	m := New(Options{CWD: "/work"})
	m.modelName = "Model A"
	light := m.welcomeMetadata(80, true)
	if !strings.Contains(light, lightTheme.muted.Render("cwd    ")) {
		t.Fatalf("light welcome labels do not use muted style: %q", light)
	}

	m.hasDarkBackground = true
	dark := m.welcomeMetadata(80, true)
	if !strings.Contains(dark, darkTheme.muted.Render("cwd    ")) {
		t.Fatalf("dark welcome labels do not use readable grey: %q", dark)
	}
}

func TestThemesUseCatppuccinSemanticColors(t *testing.T) {
	tests := []struct {
		name    string
		theme   tuiTheme
		palette colorPalette
	}{
		{name: "light", theme: lightTheme, palette: colorPalette{
			text: "#4c4f69", muted: "#6c6f85", surface: "#e6e9ef", selected: "#1e66f5", link: "#04a5e5", code: "#179299", brand: "#7287fd",
			reasoning: "#8839ef", context: "#179299", working: "#df8e1d", success: "#40a02b", error: "#d20f39",
		}},
		{name: "dark", theme: darkTheme, palette: colorPalette{
			text: "#cdd6f4", muted: "#a6adc8", surface: "#313244", selected: "#89b4fa", link: "#89dceb", code: "#94e2d5", brand: "#b4befe",
			reasoning: "#cba6f7", context: "#94e2d5", working: "#f9e2af", success: "#a6e3a1", error: "#f38ba8",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.theme.palette != test.palette {
				t.Fatalf("palette = %#v, want %#v", test.theme.palette, test.palette)
			}
			for role, style := range map[string]lipgloss.Style{
				"text": test.theme.text, "muted": test.theme.muted, "selected": test.theme.selected,
				"brand": test.theme.brand, "reasoning": test.theme.reasoning, "context": test.theme.context,
				"working": test.theme.working, "success": test.theme.success, "error": test.theme.error,
			} {
				if reflect.DeepEqual(style.GetForeground(), lipgloss.NoColor{}) {
					t.Fatalf("%s style has no foreground color", role)
				}
			}
			markdown := markdownStyle(test.name == "dark")
			for role, got := range map[string]*string{
				"text":            markdown.Document.Color,
				"heading":         markdown.Heading.Color,
				"h1 heading":      markdown.H1.Color,
				"h6 heading":      markdown.H6.Color,
				"horizontal rule": markdown.HorizontalRule.Color,
				"link":            markdown.Link.Color,
				"inline code":     markdown.Code.Color,
				"code block":      markdown.CodeBlock.Color,
			} {
				want := test.palette.text
				switch role {
				case "heading", "h1 heading", "h6 heading":
					want = test.palette.selected
				case "link":
					want = test.palette.link
				case "inline code":
					want = test.palette.code
				case "horizontal rule":
					want = test.palette.muted
				}
				if got == nil || *got != want {
					t.Fatalf("markdown %s color = %v, want %q", role, got, want)
				}
			}
			if markdown.H1.BackgroundColor != nil {
				t.Fatalf("markdown H1 background = %q, want nil", *markdown.H1.BackgroundColor)
			}
			if markdown.Code.BackgroundColor != nil {
				t.Fatalf("markdown inline code background = %q, want nil", *markdown.Code.BackgroundColor)
			}
			wantSyntaxTheme := "catppuccin-latte"
			if test.name == "dark" {
				wantSyntaxTheme = "catppuccin-mocha"
			}
			if markdown.CodeBlock.Theme != wantSyntaxTheme || markdown.CodeBlock.Chroma != nil {
				t.Fatalf("markdown syntax highlighting = theme %q, chroma %v; want theme %q", markdown.CodeBlock.Theme, markdown.CodeBlock.Chroma, wantSyntaxTheme)
			}
		})
	}
}

func TestTurnStatusViewUsesPhaseAndWallClockElapsed(t *testing.T) {
	status := newTurnStatus()
	startedAt := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	status.start(startedAt)
	status.setPhase(turnPhaseThinking)

	raw := status.viewAt(80, startedAt.Add(64*time.Second), false)
	rendered := ansi.Strip(raw)
	if !strings.Contains(rendered, "Thinking (1m 04s • esc to interrupt)") {
		t.Fatalf("turn status = %q", rendered)
	}
	meta := lightTheme.muted.Render("(1m 04s • esc to interrupt)")
	if !strings.Contains(raw, meta) {
		t.Fatal("turn status metadata does not use the light gray style")
	}
	if !strings.Contains(raw, lightTheme.reasoning.Bold(true).Render("Thinking")) {
		t.Fatal("thinking status does not use the reasoning style")
	}
	status.setPhase(turnPhaseWorking)
	if working := status.viewAt(80, startedAt.Add(65*time.Second), false); !strings.Contains(working, lightTheme.working.Bold(true).Render("Working")) {
		t.Fatal("working status does not use the working style")
	}
	if narrow := status.viewAt(20, startedAt.Add(64*time.Second), false); ansi.StringWidth(narrow) > 20 {
		t.Fatalf("narrow turn status width = %d, want at most 20", ansi.StringWidth(narrow))
	}
	status.stop()
	if rendered := status.viewAt(80, startedAt.Add(65*time.Second), false); rendered != "" {
		t.Fatalf("stopped turn status = %q", rendered)
	}
}

func TestFormatTurnElapsed(t *testing.T) {
	tests := []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 0, want: "0s"},
		{elapsed: 59 * time.Second, want: "59s"},
		{elapsed: 64 * time.Second, want: "1m 04s"},
		{elapsed: 3661 * time.Second, want: "1h 01m 01s"},
	}
	for _, test := range tests {
		if got := formatTurnElapsed(test.elapsed); got != test.want {
			t.Fatalf("formatTurnElapsed(%s) = %q, want %q", test.elapsed, got, test.want)
		}
	}
}

func TestTurnStatusTracksAgentPhase(t *testing.T) {
	m := New(Options{})
	m.current = newAssistantMessage()
	m.turnStatus.start(time.Now())

	m.handleAgentEvent(agent.Event{Type: agent.EventModelReasoningDelta, Content: "reasoning"})
	if m.turnStatus.phase != turnPhaseThinking {
		t.Fatalf("reasoning phase = %d, want thinking", m.turnStatus.phase)
	}
	m.handleAgentEvent(agent.Event{Type: agent.EventModelDelta, Content: "answer"})
	if m.turnStatus.phase != turnPhaseWorking {
		t.Fatalf("model output phase = %d, want working", m.turnStatus.phase)
	}
	m.handleAgentEvent(agent.Event{Type: agent.EventModelReasoningDelta, Content: "reasoning"})
	m.handleAgentEvent(agent.Event{Type: agent.EventModelResponse})
	if m.turnStatus.phase != turnPhaseWorking {
		t.Fatalf("model response phase = %d, want working", m.turnStatus.phase)
	}
	m.handleAgentEvent(agent.Event{Type: agent.EventModelReasoningDelta, Content: "reasoning"})
	m.handleAgentEvent(agent.Event{Type: agent.EventToolStarted, ToolCall: model.ToolCall{ID: "tool-1", Name: "run_shell"}})
	if m.turnStatus.phase != turnPhaseWorking {
		t.Fatalf("tool phase = %d, want working", m.turnStatus.phase)
	}
}

func TestTurnStatusStopsWithTurn(t *testing.T) {
	m := New(Options{})
	m.turnActive = true
	m.turnStatus.start(time.Now())
	tick := m.turnStatus.spinner.Tick()

	updated, cmd := m.Update(tick)
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("active spinner tick did not schedule another frame")
	}
	m.handleTurnDone(turnDoneMsg{})
	if m.turnStatus.active() {
		t.Fatal("turn status remained active after completion")
	}
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("inactive spinner tick scheduled another frame")
	}
}

func TestTurnStatusRendersAboveComposer(t *testing.T) {
	m := New(Options{})
	m.modelName = "gpt-5.6-sol"
	m.turnActive = true
	m.turnStatus.start(time.Now())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m = updated.(Model)

	rendered := ansi.Strip(m.View().Content)
	statusIndex := strings.Index(rendered, "Working (")
	composerIndex := strings.Index(rendered, "›")
	if statusIndex < 0 || composerIndex < 0 || statusIndex >= composerIndex {
		t.Fatalf("status/composer order = status:%d composer:%d content:%q", statusIndex, composerIndex, rendered)
	}
	rawLines := strings.Split(m.View().Content, "\n")
	for index, line := range rawLines {
		if !strings.Contains(ansi.Strip(line), "Working (") {
			continue
		}
		if index == 0 || ansi.Strip(rawLines[index-1]) != "" {
			t.Fatal("blank line is missing between conversation and turn status")
		}
		if index+1 >= len(rawLines) || rawLines[index+1] == "" {
			t.Fatal("blank line remains between turn status and composer")
		}
		break
	}
	if got := lipgloss.Height(m.View().Content); got != 12 {
		t.Fatalf("View height = %d, want 12", got)
	}
}

func TestSmallTerminalPrioritizesTurnStatus(t *testing.T) {
	m := New(Options{})
	m.modelName = "gpt-5.6-sol"
	m.turnActive = true
	m.turnStatus.start(time.Now())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 4})
	m = updated.(Model)

	rendered := ansi.Strip(m.View().Content)
	if !strings.Contains(rendered, "Working (") || strings.Contains(rendered, "gpt-5.6-sol") {
		t.Fatalf("small terminal content = %q", rendered)
	}
	if got := lipgloss.Height(m.View().Content); got != 4 {
		t.Fatalf("View height = %d, want 4", got)
	}
}

func TestSmallTerminalCapsComposerHeight(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: strings.Repeat("line\n", 12)})
	m = updated.(Model)

	if got := lipgloss.Height(m.View().Content); got != 5 {
		t.Fatalf("View height = %d, want 5", got)
	}
}

func TestPageUpScrollsConversationHistory(t *testing.T) {
	m := New(Options{})
	for i := range 30 {
		m.messages = append(m.messages, newUserMessage(strings.Repeat("x", i+1)))
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = updated.(Model)
	start := m.viewport.YOffset()
	if start == 0 {
		t.Fatal("viewport did not start at the bottom")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = updated.(Model)
	if got := m.viewport.YOffset(); got >= start {
		t.Fatalf("viewport offset after PageUp = %d, want less than %d", got, start)
	}
}

func TestMouseWheelScrollsConversationHistory(t *testing.T) {
	m := New(Options{})
	for i := range 30 {
		m.messages = append(m.messages, newUserMessage(strings.Repeat("x", i+1)))
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = updated.(Model)
	start := m.viewport.YOffset()

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(Model)
	if got := m.viewport.YOffset(); got >= start {
		t.Fatalf("viewport offset after wheel up = %d, want less than %d", got, start)
	}
}

func TestPasteUpdatesPrompt(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.PasteMsg{Content: "first line\nsecond line"})
	m = updated.(Model)

	if got := m.input.Value(); got != "first line\nsecond line" {
		t.Fatalf("input value = %q", got)
	}
}

func TestComposerUsesMessageBackgroundWithoutDividers(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updated.(Model)

	composerState := m.renderComposer()
	composer := composerStyle(m.hasDarkBackground, nil).Width(m.width).Render(composerState.content)
	if got := lipgloss.Width(composer); got != 20 {
		t.Fatalf("composer width = %d, want 20", got)
	}
	if got := lipgloss.Height(composer); got != 3 {
		t.Fatalf("composer height = %d, want 3", got)
	}
	lines := strings.Split(ansi.Strip(composer), "\n")
	if !strings.HasPrefix(lines[1], "›") {
		t.Fatalf("composer input line has left padding: %q", lines[1])
	}
	view := m.View()
	if strings.Contains(ansi.Strip(view.Content), "─") {
		t.Fatal("composer view still contains horizontal dividers")
	}
	if view.Cursor == nil {
		t.Fatal("composer cursor is missing")
	}
	wantY := m.viewport.Height() + 2 // one gap row and one background padding row
	if view.Cursor.Y != wantY {
		t.Fatalf("cursor Y = %d, want %d", view.Cursor.Y, wantY)
	}
	lines = strings.Split(ansi.Strip(view.Content), "\n")
	statusLine := lines[len(lines)-1]
	if lines[len(lines)-2] == "" {
		t.Fatalf("blank line remains before status line %q", statusLine)
	}
}

func TestComposerBackgroundUsesCatppuccinSurface(t *testing.T) {
	tests := []struct {
		name       string
		background color.Color
		dark       bool
		want       color.RGBA
	}{
		{name: "latte without terminal color", want: color.RGBA{R: 230, G: 233, B: 239, A: 255}},
		{name: "latte ignores terminal color", background: color.White, want: color.RGBA{R: 230, G: 233, B: 239, A: 255}},
		{name: "mocha without terminal color", dark: true, want: color.RGBA{R: 49, G: 50, B: 68, A: 255}},
		{name: "mocha ignores terminal color", background: color.Black, dark: true, want: color.RGBA{R: 49, G: 50, B: 68, A: 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := color.RGBAModel.Convert(composerStyle(tt.dark, tt.background).GetBackground()).(color.RGBA)
			if got != tt.want {
				t.Fatalf("composer background = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComposerDoesNotInsertLineBreakBeforeASCIIWord(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: "你好！我是 Atlas，可以帮你处理本地文件、运行命令、写代码、查资料等。有什么需要帮忙的吗？"})
	m = updated.(Model)

	assertWordsShareVisualLine(t, ansi.Strip(m.renderComposer().content), "我是", "Atlas")
}

func TestComposerNewlineKeysInsertLineBreakWithoutSubmitting(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "shift enter", key: tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}},
		{name: "ctrl j", key: tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := New(Options{})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
			m = updated.(Model)
			m.input.SetValue("first")
			m.input.MoveToEnd()

			updated, _ = m.Update(test.key)
			m = updated.(Model)
			if got := m.input.Value(); got != "first\n" {
				t.Fatalf("input value = %q, want %q", got, "first\n")
			}
			if len(m.messages) != 0 || m.turnActive {
				t.Fatalf("newline submitted input: messages=%d active=%t", len(m.messages), m.turnActive)
			}
			if got := m.renderComposer().height; got != 2 {
				t.Fatalf("composer height = %d, want 2", got)
			}
		})
	}
}

func TestEnterSubmitsMultilineInput(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("first\nsecond")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || len(m.messages) != 2 || !m.turnActive {
		t.Fatalf("submit state: cmd=%v messages=%d active=%t", cmd, len(m.messages), m.turnActive)
	}
	if got := m.messages[0].content.String(); got != "first\nsecond" {
		t.Fatalf("submitted message = %q", got)
	}
}

func TestSlashPopupCompletesSelectedSkill(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(skillSummariesLoadedMsg{summaries: []runtime.SkillSummary{
		{Name: "hunt", Description: "Find root causes"},
		{Name: "think", Description: strings.Repeat("Plan work ", 10)},
	}})
	m = updated.(Model)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: "/th"})
	m = updated.(Model)

	if !m.slashPopup.active() {
		t.Fatal("slash popup did not open")
	}
	inputArea := m.renderInputArea()
	rendered := ansi.Strip(inputArea.content)
	if !strings.Contains(rendered, "/think") || strings.Contains(rendered, "/hunt") {
		t.Fatalf("input area = %q", rendered)
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 || lines[1] != "" || !strings.HasPrefix(lines[2], "› /th") {
		t.Fatalf("popup spacing = %q", rendered)
	}
	if inputArea.height != 3 || inputArea.cursorRow != 2 {
		t.Fatalf("input area layout = height:%d cursorRow:%d", inputArea.height, inputArea.cursorRow)
	}
	expected := composerStyle(m.hasDarkBackground, m.terminalBackground).
		Width(m.width).
		Render(inputArea.content)
	if !strings.Contains(m.View().Content, expected) {
		t.Fatal("slash popup is rendered outside the composer background")
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if width := ansi.StringWidth(line); width > m.width-1 {
			t.Fatalf("input area line width = %d, want at most %d: %q", width, m.width-1, line)
		}
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if cmd != nil || m.input.Value() != "/think " || m.slashPopup.active() {
		t.Fatalf("completion state: value=%q active=%t cmd=%v", m.input.Value(), m.slashPopup.active(), cmd)
	}
}

func TestInlineSlashPopupCompletesSkillWithoutSubmitting(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(skillSummariesLoadedMsg{summaries: []runtime.SkillSummary{
		{Name: "think", Description: "Plan work"},
	}})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: "review /th"})
	m = updated.(Model)
	if !m.slashPopup.active() {
		t.Fatal("inline slash popup did not open")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.input.Value() != "review /think " || m.slashPopup.active() || len(m.messages) != 0 {
		t.Fatalf("completion state: value=%q active=%t messages=%d cmd=%v", m.input.Value(), m.slashPopup.active(), len(m.messages), cmd)
	}
}

func TestEscapeDismissesInlineSlashPopupUntilDraftChanges(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(skillSummariesLoadedMsg{summaries: []runtime.SkillSummary{
		{Name: "think", Description: "Plan work"},
	}})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: "review /"})
	m = updated.(Model)
	if !m.slashPopup.active() {
		t.Fatal("inline slash popup did not open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.slashPopup.active() || m.input.Value() != "review /" {
		t.Fatalf("dismissed state: value=%q active=%t", m.input.Value(), m.slashPopup.active())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if !m.slashPopup.active() || m.input.Value() != "review /t" {
		t.Fatalf("reopened state: value=%q active=%t", m.input.Value(), m.slashPopup.active())
	}
}

func TestEnterCompletesSelectedSlashCommandWithoutSubmitting(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if !m.slashPopup.active() {
		t.Fatal("slash popup did not open")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.input.Value() != "/model " || m.slashPopup.active() || len(m.messages) != 0 {
		t.Fatalf("completion state: value=%q active=%t messages=%d cmd=%v", m.input.Value(), m.slashPopup.active(), len(m.messages), cmd)
	}
}

func TestEscapeDismissesSlashPopup(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if !m.slashPopup.active() {
		t.Fatal("slash popup did not open")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if cmd != nil || m.slashPopup.active() || m.input.Value() != "/" {
		t.Fatalf("dismiss state: active=%t value=%q cmd=%v", m.slashPopup.active(), m.input.Value(), cmd)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	if !m.slashPopup.active() || m.input.Value() != "/m" {
		t.Fatalf("reopen state: active=%t value=%q", m.slashPopup.active(), m.input.Value())
	}
}

func TestQuitCommandExits(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("/quit")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("quit command did not return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit command returned %T, want tea.QuitMsg", cmd())
	}
}

func TestCompactCommandInstruction(t *testing.T) {
	tests := []struct {
		input           string
		wantInstruction string
		wantCompact     bool
	}{
		{input: "/compact", wantCompact: true},
		{input: "/compact keep decisions", wantInstruction: "keep decisions", wantCompact: true},
		{input: "/compact\nkeep files", wantInstruction: "keep files", wantCompact: true},
		{input: "/compactness matters"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			instruction, ok := compactCommandInstruction(tt.input)
			if ok != tt.wantCompact || instruction != tt.wantInstruction {
				t.Fatalf("compactCommandInstruction(%q) = %q, %t", tt.input, instruction, ok)
			}
		})
	}
}

func TestCompactCommandWithoutSessionShowsNotice(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("/compact keep decisions")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.compactActive || m.input.Value() != "" || !m.input.Focused() {
		t.Fatalf("compact state: cmd=%v active=%t value=%q focused=%t", cmd, m.compactActive, m.input.Value(), m.input.Focused())
	}
	if len(m.messages) != 1 || !strings.Contains(ansi.Strip(m.messages[0].render(80, false, nil)), "No session to compact") {
		t.Fatalf("compact notice = %#v", m.messages)
	}
}

func TestCompactCompletionShowsNoSafeBoundaryAndErrors(t *testing.T) {
	tests := []struct {
		name       string
		msg        compactDoneMsg
		want       string
		wantFailed bool
	}{
		{name: "no safe boundary", msg: compactDoneMsg{result: runtime.CompactResult{Reason: "no safe compaction boundary"}}, want: "No safe context to compact."},
		{name: "provider error", msg: compactDoneMsg{err: errors.New("provider failed")}, want: "Compaction failed: provider failed", wantFailed: true},
		{name: "cancelled", msg: compactDoneMsg{err: context.Canceled}, want: "Compaction cancelled."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Options{})
			m.compactActive = true
			m.input.Blur()
			m.turnStatus.start(time.Now())

			updated, cmd := m.Update(tt.msg)
			m = updated.(Model)
			if cmd != nil || m.compactActive || m.turnStatus.active() || !m.input.Focused() {
				t.Fatalf("completion state: cmd=%v active=%t status=%t focused=%t", cmd, m.compactActive, m.turnStatus.active(), m.input.Focused())
			}
			if len(m.messages) != 1 || m.messages[0].noticeError != tt.wantFailed {
				t.Fatalf("notice state = %#v", m.messages)
			}
			if rendered := ansi.Strip(m.messages[0].render(80, false, nil)); !strings.Contains(rendered, tt.want) {
				t.Fatalf("notice = %q, want %q", rendered, tt.want)
			}
		})
	}
}

func TestEscapeInterruptsTurnAndCtrlCDoesNothing(t *testing.T) {
	m := New(Options{})
	m.turnActive = true
	cancelled := false
	abandoned := false
	m.turnCancel = func() { cancelled = true }
	m.turnAbandon = func() { abandoned = true }

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)
	if cmd != nil || cancelled || abandoned {
		t.Fatalf("ctrl+c state: cmd=%v cancelled=%t abandoned=%t", cmd, cancelled, abandoned)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !cancelled || abandoned {
		t.Fatalf("escape state: cmd=%v cancelled=%t abandoned=%t", cmd, cancelled, abandoned)
	}

	m.turnActive = false
	m.turnCancel = nil
	m.compactActive = true
	compactCancelled := false
	m.compactCancel = func() { compactCancelled = true }
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)
	if cmd != nil || compactCancelled {
		t.Fatalf("compact ctrl+c state: cmd=%v cancelled=%t", cmd, compactCancelled)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || !compactCancelled {
		t.Fatalf("compact escape state: cmd=%v cancelled=%t", cmd, compactCancelled)
	}
}

func TestModelCommandOpensPickerWithoutEnteringHistory(t *testing.T) {
	m := New(Options{})
	m.models = pickerTestModels()
	m.modelValue = m.models[0].Value
	m.modelName = m.models[0].Name
	m.reasoningEffort = "high"
	m.input.SetValue("/model")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if cmd != nil || !m.modelPicker.active() || len(m.messages) != 0 {
		t.Fatalf("model command state: active=%t messages=%d cmd=%v", m.modelPicker.active(), len(m.messages), cmd)
	}
	if m.input.Focused() || m.input.Value() != "" {
		t.Fatalf("input remained active after opening picker: focused=%t value=%q", m.input.Focused(), m.input.Value())
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = updated.(Model)
	view := m.View()
	if view.Cursor != nil || !strings.Contains(ansi.Strip(view.Content), "Select model") {
		t.Fatalf("picker view cursor=%v content=%q", view.Cursor, ansi.Strip(view.Content))
	}
}

func TestModelCommandWaitsForModelOptions(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("/model")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if cmd != nil || m.modelPicker.active() || m.input.Value() != "/model" || !m.input.Focused() {
		t.Fatalf("unavailable model command state: cmd=%v active=%t value=%q focused=%t", cmd, m.modelPicker.active(), m.input.Value(), m.input.Focused())
	}
}

func TestModelPickerUpdatesFooterState(t *testing.T) {
	m := New(Options{})
	m.models = pickerTestModels()
	m.modelValue = m.models[0].Value
	m.modelName = m.models[0].Name
	m.reasoningEffort = "high"
	m.contextTokens = 200
	m.contextWindow = 1000
	m.openModelPicker()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.modelValue != "provider/model-c" || m.modelName != "Model C" || m.reasoningEffort != "medium" {
		t.Fatalf("selected model state = value:%q name:%q effort:%q", m.modelValue, m.modelName, m.reasoningEffort)
	}
	if m.contextWindow != 2000 || !m.input.Focused() || m.modelPicker.active() {
		t.Fatalf("selection completion state: context=%d focused=%t active=%t", m.contextWindow, m.input.Focused(), m.modelPicker.active())
	}
}

func TestEscapeClosesModelPickerAndCtrlCDoesNothing(t *testing.T) {
	m := New(Options{})
	m.models = pickerTestModels()
	m.modelValue = m.models[0].Value
	m.openModelPicker()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)
	if cmd != nil || !m.modelPicker.active() || m.input.Focused() {
		t.Fatalf("ctrl+c picker state: cmd=%v active=%t focused=%t", cmd, m.modelPicker.active(), m.input.Focused())
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if cmd != nil || m.modelPicker.active() || !m.input.Focused() {
		t.Fatalf("escape picker state: cmd=%v active=%t focused=%t", cmd, m.modelPicker.active(), m.input.Focused())
	}
}

func TestSubmitTurnUsesSelectedModelAndReasoningEffort(t *testing.T) {
	provider := &tuiRecordingProvider{response: model.ChatResponse{Content: "ok"}}
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "provider/model-b",
				Providers: []config.ProviderConfig{{
					Name: "provider",
					Models: []config.ProviderModel{
						{Value: "model-a", Name: "Model A", ContextWindow: 1000, MaxTokens: 100, InputFormats: []string{config.ModelInputFormatText}, ReasoningEfforts: []config.ProviderReasoningEffort{{Value: "high", Name: "High"}, {Value: "xhigh", Name: "XHigh"}}},
						{Value: "model-b", Name: "Model B", ContextWindow: 1000, MaxTokens: 100, InputFormats: []string{config.ModelInputFormatText}},
					},
				}},
				Agent:   config.AgentConfig{MaxSteps: 2},
				Session: config.SessionConfig{DBPath: dbPath},
			}, nil
		},
		NewProvider: func(_ config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			provider.selectedModel = selected.Value
			return provider, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	m := New(Options{Runtime: rt, CWD: t.TempDir()})
	m.modelValue = "provider/model-a"
	m.reasoningEffort = "xhigh"
	_, cmd := m.submitTurn("hello")
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("submit command = %T with %d entries, want tea.BatchMsg with 3", batch, len(batch))
	}
	batch[1]()

	if provider.selectedModel != "model-a" || provider.request.ReasoningEffort != "xhigh" {
		t.Fatalf("provider selection = model:%q effort:%q", provider.selectedModel, provider.request.ReasoningEffort)
	}
}

func TestCompactCommandUsesRuntimeWithoutPersistingCommand(t *testing.T) {
	provider := &tuiRecordingProvider{response: model.ChatResponse{Content: "reply"}}
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	cwd := t.TempDir()
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "provider/model-a",
				Providers: []config.ProviderConfig{{
					Name: "provider",
					Models: []config.ProviderModel{{
						Value: "model-a", ContextWindow: 1000, MaxTokens: 100,
						InputFormats:     []string{config.ModelInputFormatText},
						ReasoningEfforts: []config.ProviderReasoningEffort{{Value: "high", Name: "High"}},
					}},
				}},
				Agent:   config.AgentConfig{MaxSteps: 2},
				Session: config.SessionConfig{DBPath: dbPath},
			}, nil
		},
		NewProvider: func(config.ProviderConfig, config.ProviderModel) (model.Provider, error) {
			return provider, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	for _, prompt := range []string{"first", "second"} {
		if _, err := rt.RunTurn(t.Context(), runtime.TurnOptions{SessionID: "work", Prompt: prompt, CWD: cwd}); err != nil {
			t.Fatalf("RunTurn(%q) error = %v", prompt, err)
		}
	}
	provider.response = model.ChatResponse{Content: "summary"}

	m := New(Options{Runtime: rt, SessionID: "work", CWD: cwd})
	m.loading = false
	m.input.Focus()
	m.width = 80
	m.modelValue = "provider/model-a"
	m.modelName = "Model A"
	m.reasoningEffort = "high"
	m.contextTokens = 900
	m.contextWindow = 1000
	m.input.SetValue("/compact keep decisions")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || !m.compactActive || !m.turnStatus.active() || m.input.Focused() || len(m.messages) != 0 {
		t.Fatalf("running state: cmd=%v active=%t status=%t focused=%t messages=%d", cmd, m.compactActive, m.turnStatus.active(), m.input.Focused(), len(m.messages))
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("compact command = %T with %d entries, want tea.BatchMsg with 2", batch, len(batch))
	}
	done, ok := batch[0]().(compactDoneMsg)
	if !ok || done.err != nil || !done.result.Compacted {
		t.Fatalf("compact result = %#v, %t", done, ok)
	}
	if provider.request.ReasoningEffort != "high" || len(provider.request.Messages) != 1 || !strings.Contains(provider.request.Messages[0].Content, "Additional user instruction:\nkeep decisions") {
		t.Fatalf("compact request = %#v", provider.request)
	}

	updated, _ = m.Update(done)
	m = updated.(Model)
	if m.compactActive || !m.input.Focused() || m.contextTokens != done.result.TokensAfter || m.contextWindow != done.result.ContextWindow {
		t.Fatalf("completed state: active=%t focused=%t tokens=%d window=%d", m.compactActive, m.input.Focused(), m.contextTokens, m.contextWindow)
	}
	wantContext := fmt.Sprintf("Context %d%% used", contextUsagePercent(done.result.TokensAfter, done.result.ContextWindow))
	if status := ansi.Strip(m.statusView()); !strings.Contains(status, wantContext) {
		t.Fatalf("status = %q, want %q", status, wantContext)
	}
	if len(m.messages) != 1 || !strings.Contains(ansi.Strip(m.messages[0].render(80, false, nil)), "Context compacted. Kept 2 recent messages.") {
		t.Fatalf("compact notice = %#v", m.messages)
	}
	info, transcript, err := rt.ShowSession(t.Context(), "work")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if info.ContextSummary != "summary" || len(transcript.Messages()) != 4 {
		t.Fatalf("persisted session = info:%#v messages:%d", info, len(transcript.Messages()))
	}
}

func TestSubmitTurnInjectsSelectedSkillAndPreservesPrompt(t *testing.T) {
	provider := &tuiRecordingProvider{response: model.ChatResponse{Content: "ok"}}
	catalog, err := skill.NewCatalog([]skill.Skill{{
		Name:        "think",
		Description: "Plan work",
		Content:     "# Think\nPlan first.",
	}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "provider/model-a",
				Providers: []config.ProviderConfig{{
					Name: "provider",
					Models: []config.ProviderModel{{
						Value: "model-a", ContextWindow: 1000, MaxTokens: 100,
						InputFormats: []string{config.ModelInputFormatText},
					}},
				}},
				Agent:   config.AgentConfig{MaxSteps: 2},
				Session: config.SessionConfig{DBPath: dbPath},
			}, nil
		},
		NewProvider: func(config.ProviderConfig, config.ProviderModel) (model.Provider, error) {
			return provider, nil
		},
		LoadSkills: func(string) (*skill.Catalog, error) {
			return catalog, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	m := New(Options{Runtime: rt, CWD: t.TempDir()})
	_, cmd := m.submitTurn("/think design this")
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("submit command = %T with %d entries, want tea.BatchMsg with 3", batch, len(batch))
	}
	batch[1]()

	if len(provider.request.Messages) != 2 {
		t.Fatalf("provider messages = %#v", provider.request.Messages)
	}
	if !strings.Contains(provider.request.Messages[0].Content, "<name>think</name>") || !strings.Contains(provider.request.Messages[0].Content, "# Think") {
		t.Fatalf("skill message = %q", provider.request.Messages[0].Content)
	}
	if provider.request.Messages[1].Content != "/think design this" {
		t.Fatalf("user prompt = %q", provider.request.Messages[1].Content)
	}
}

func TestComposerHardWrapMapsCursorPosition(t *testing.T) {
	rendered := renderComposerValue("abcdef", 4, 10, 0, 4)

	if rendered.content != "› abcd\n  ef" {
		t.Fatalf("composer content = %q", rendered.content)
	}
	if rendered.cursorRow != 1 || rendered.cursorColumn != 0 {
		t.Fatalf("cursor = (%d, %d), want (1, 0)", rendered.cursorRow, rendered.cursorColumn)
	}
}

func TestComposerPreservesExplicitNewlineAndCursor(t *testing.T) {
	rendered := renderComposerValue("one\ntwo", 20, 10, 1, 3)

	if rendered.content != "› one\n  two" {
		t.Fatalf("composer content = %q", rendered.content)
	}
	if rendered.cursorRow != 1 || rendered.cursorColumn != 3 {
		t.Fatalf("cursor = (%d, %d), want (1, 3)", rendered.cursorRow, rendered.cursorColumn)
	}
}

func TestComposerUpMovesToPreviousVisualLine(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	m = updated.(Model)
	updated, _ = m.Update(tea.PasteMsg{Content: "你好！我是 Atlas，可以帮你处理本地文件、运行命令、写代码、查资料等。有什么需要帮忙的吗？"})
	m = updated.(Model)
	before := m.renderComposer()
	if before.cursorRow < 1 {
		t.Fatalf("test input did not wrap: %+v", before)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)
	after := m.renderComposer()
	if after.cursorRow != before.cursorRow-1 {
		t.Fatalf("cursor row after Up = %d, want %d", after.cursorRow, before.cursorRow-1)
	}
}

func TestUserMessageUsesFullWidthBackgroundWithVerticalPadding(t *testing.T) {
	rendered := newUserMessage("hello").render(20, false, nil)

	if got := lipgloss.Width(rendered); got != 20 {
		t.Fatalf("rendered width = %d, want 20", got)
	}
	if got := lipgloss.Height(rendered); got != 3 {
		t.Fatalf("rendered height = %d, want 3", got)
	}
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if strings.TrimSpace(lines[0]) != "" || strings.TrimSpace(lines[2]) != "" {
		t.Fatalf("vertical padding lines contain text: %q", lines)
	}
	if !strings.Contains(lines[1], "› hello") {
		t.Fatalf("user content line = %q", lines[1])
	}
}

func TestUserMessageDoesNotInsertLineBreakBeforeASCIIWord(t *testing.T) {
	message := newUserMessage("你好！我是 Atlas，可以帮你处理本地文件、运行命令、写代码、查资料等。有什么需要帮忙的吗？")

	assertWordsShareVisualLine(t, ansi.Strip(message.render(60, false, nil)), "我是", "Atlas")
}

func TestAssistantAndToolBlocksUseBulletsAndIndentedContent(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("first line\nsecond line")
	message.toolCalls = append(message.toolCalls, toolCallView{
		call:   model.ToolCall{Name: "run_shell", Arguments: `{"command":"Search Atlas"}`},
		result: "Search Atlas",
		done:   true,
	})

	rendered := ansi.Strip(message.render(40, false, nil))
	if !strings.Contains(rendered, "• first line second line") ||
		!strings.Contains(rendered, "• Ran Search Atlas\n  └ Search Atlas") {
		t.Fatalf("rendered message omitted assistant or tool content: %q", rendered)
	}
}

func TestAssistantRendersMarkdown(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("# Heading\n\n**bold** and `code`\n\n- one\n- two")

	rendered := message.render(40, true, nil)
	plain := ansi.Strip(rendered)
	for _, rawMarker := range []string{"# Heading", "**bold**", "`code`"} {
		if strings.Contains(plain, rawMarker) {
			t.Fatalf("rendered markdown retained %q: %q", rawMarker, plain)
		}
	}
	for _, content := range []string{"Heading", "bold", "code", "one", "two"} {
		if !strings.Contains(plain, content) {
			t.Fatalf("rendered markdown omitted %q: %q", content, plain)
		}
	}
	if rendered == plain {
		t.Fatalf("rendered markdown contains no styling: %q", rendered)
	}
	lines := strings.Split(plain, "\n")
	firstContentLine := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "• ") {
			firstContentLine = i
			break
		}
	}
	if firstContentLine < 0 {
		t.Fatalf("rendered markdown has no assistant marker: %q", plain)
	}
	for _, line := range lines[firstContentLine+1:] {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("continuation markdown line = %q, want two-space indent", line)
		}
	}
}

func TestAssistantMarkdownStartsWithoutDocumentGapOrH1Padding(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("# Markdown 示例")

	rendered := ansi.Strip(message.render(40, false, nil))
	firstLine, _, _ := strings.Cut(rendered, "\n")
	if firstLine != "• Markdown 示例" {
		t.Fatalf("first markdown line = %q, want %q", firstLine, "• Markdown 示例")
	}
}

func TestAssistantRendersGFMTable(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("| Name | Status |\n| --- | --- |\n| Atlas | Ready for long responses |")

	rendered := ansi.Strip(message.render(24, true, nil))
	for _, content := range []string{"Name", "Status", "Atlas", "Ready"} {
		if !strings.Contains(rendered, content) {
			t.Fatalf("rendered table omitted %q: %q", content, rendered)
		}
	}
	if strings.Contains(rendered, "| --- |") {
		t.Fatalf("rendered table retained markdown delimiter: %q", rendered)
	}
	if firstLine, _, _ := strings.Cut(rendered, "\n"); strings.TrimSpace(firstLine) == "•" {
		t.Fatalf("rendered table starts with an empty assistant marker: %q", rendered)
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 24 {
			t.Fatalf("rendered table line width = %d, want at most 24: %q", got, line)
		}
	}
}

func TestAssistantMarkdownWrapsWithinMessageWidth(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("Markdown wrapping keeps 中文、English words, and emoji ✅ inside the viewport.")

	rendered := message.render(24, false, nil)
	for line := range strings.SplitSeq(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 24 {
			t.Fatalf("rendered line width = %d, want at most 24: %q", got, line)
		}
	}
}

func TestAssistantMarkdownPreservesCJKEmphasis(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("这是 **中文强调** 文本")

	rendered := message.render(40, false, nil)
	plain := ansi.Strip(rendered)
	if strings.Contains(plain, "**") || !strings.Contains(plain, "中文强调") {
		t.Fatalf("rendered CJK emphasis = %q", plain)
	}
	if rendered == plain {
		t.Fatalf("CJK emphasis contains no styling: %q", rendered)
	}
	if strings.ContainsRune(rendered, '\u2060') {
		t.Fatalf("rendered CJK emphasis retained wrap hint: %q", rendered)
	}
}

func TestMarkdownStyleKeepsOnlyRequiredLayoutOverrides(t *testing.T) {
	for _, dark := range []bool{false, true} {
		style := markdownStyle(dark)
		native := glamourstyles.LightStyleConfig
		if dark {
			native = glamourstyles.DarkStyleConfig
		}
		if style.Document.BlockPrefix != "" {
			t.Fatalf("markdownStyle(%t) document prefix = %q, want empty", dark, style.Document.BlockPrefix)
		}
		if style.Document.BlockSuffix != native.Document.BlockSuffix {
			t.Fatalf("markdownStyle(%t) changes native document suffix", dark)
		}
		if style.Document.Margin != nil {
			t.Fatalf("markdownStyle(%t) document margin = %v, want nil", dark, style.Document.Margin)
		}
		if !reflect.DeepEqual(style.BlockQuote.Indent, native.BlockQuote.Indent) ||
			!reflect.DeepEqual(style.BlockQuote.IndentToken, native.BlockQuote.IndentToken) {
			t.Fatalf("markdownStyle(%t) changes native blockquote layout", dark)
		}
		if style.Heading.BlockSuffix != native.Heading.BlockSuffix ||
			!reflect.DeepEqual(style.Heading.Bold, native.Heading.Bold) {
			t.Fatalf("markdownStyle(%t) changes native heading layout", dark)
		}
		if style.H1.Prefix != "" {
			t.Fatalf("markdownStyle(%t) H1 prefix = %q, want empty", dark, style.H1.Prefix)
		}
		if style.H1.Suffix != native.H1.Suffix ||
			!reflect.DeepEqual(style.H1.Bold, native.H1.Bold) ||
			!reflect.DeepEqual(style.H1.Underline, native.H1.Underline) {
			t.Fatalf("markdownStyle(%t) changes native H1 typography", dark)
		}
		if style.H2.Prefix != native.H2.Prefix || style.H3.Prefix != native.H3.Prefix ||
			style.H4.Prefix != native.H4.Prefix || style.H5.Prefix != native.H5.Prefix ||
			style.H6.Prefix != native.H6.Prefix {
			t.Fatalf("markdownStyle(%t) changes native heading prefixes", dark)
		}
		if style.Code.Prefix != native.Code.Prefix || style.Code.Suffix != native.Code.Suffix ||
			!reflect.DeepEqual(style.CodeBlock.Margin, native.CodeBlock.Margin) {
			t.Fatalf("markdownStyle(%t) changes native code layout", dark)
		}
	}
}

func TestAssistantMarkdownHeadingsRenderInBlue(t *testing.T) {
	tests := []struct {
		name string
		dark bool
		rgb  string
	}{
		{name: "light", rgb: "30;102;245"},
		{name: "dark", dark: true, rgb: "137;180;250"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := newAssistantMessage()
			message.content.WriteString("# First\n\n## Second\n\n### Third")

			rendered := message.render(40, test.dark, nil)
			blue := regexp.MustCompile(`\x1b\[[0-9;]*38;2;` + test.rgb + `[0-9;]*m`)
			if headings := blue.FindAllString(rendered, -1); len(headings) < 3 {
				t.Fatalf("rendered headings contain %d Blue sequences, want at least 3: %q", len(headings), rendered)
			}
		})
	}
}

func TestAssistantMarkdownHandlesIncompleteFenceWhileStreaming(t *testing.T) {
	message := newAssistantMessage()
	message.handleEvent(agent.Event{Type: agent.EventModelDelta, Content: "```go\nfmt.Println(\"hi\")"})

	partial := ansi.Strip(message.render(40, true, nil))
	if strings.Contains(partial, "```") || !strings.Contains(partial, "fmt.Println") {
		t.Fatalf("partial fenced code render = %q", partial)
	}

	message.handleEvent(agent.Event{Type: agent.EventModelDelta, Content: "\n```\n\nDone."})
	complete := ansi.Strip(message.render(40, true, nil))
	if strings.Contains(complete, "```") || !strings.Contains(complete, "Done.") {
		t.Fatalf("completed fenced code render = %q", complete)
	}
}

func TestAssistantMarkdownCodeBlockUsesTrueColorSyntaxHighlighting(t *testing.T) {
	colorPattern := regexp.MustCompile(`\x1b\[38;2;[0-9;]+m`)
	for _, dark := range []bool{false, true} {
		message := newAssistantMessage()
		message.content.WriteString("```go\npackage main\n\nfunc main() { println(\"hello\") }\n```")

		colors := make(map[string]bool)
		for _, sequence := range colorPattern.FindAllString(message.render(60, dark, nil), -1) {
			colors[sequence] = true
		}
		if len(colors) < 2 {
			t.Fatalf("dark=%t syntax colors = %v, want at least two true-color sequences", dark, colors)
		}
	}
}

func TestAssistantMarkdownCacheInvalidatesForThemeAndContent(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("## Heading")

	dark := message.render(40, true, nil)
	light := message.render(40, false, nil)
	if dark == light {
		t.Fatal("light and dark markdown code styles are identical")
	}

	message.content.WriteString("\n\nand more")
	updated := ansi.Strip(message.render(40, false, nil))
	if !strings.Contains(updated, "and more") {
		t.Fatalf("cached markdown omitted appended content: %q", updated)
	}
}

func TestAssistantContentUsesPrimaryThemeColor(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("plain response")

	rendered := message.render(40, false, nil)
	if !strings.Contains(rendered, lightTheme.text.Render("plain")) {
		t.Fatalf("assistant response does not use the light primary color: %q", rendered)
	}
}

func assertWordsShareVisualLine(t *testing.T, rendered, first, second string) {
	t.Helper()
	for line := range strings.SplitSeq(rendered, "\n") {
		if strings.Contains(line, first) && strings.Contains(line, second) {
			return
		}
	}
	t.Fatalf("%q and %q are split across visual lines: %q", first, second, rendered)
}

func TestTurnErrorRestoresInputFocus(t *testing.T) {
	m := New(Options{})
	m.turnActive = true
	m.current = newAssistantMessage()
	m.input.Blur()

	m.handleTurnDone(turnDoneMsg{err: errors.New("provider failed")})

	if !m.input.Focused() {
		t.Fatal("input remained blurred after turn error")
	}
}

func TestTurnResultUpdatesContextUsage(t *testing.T) {
	m := New(Options{})
	m.width = 80
	m.modelName = "gpt-5.6-sol"
	m.reasoningEffort = "high"
	m.turnActive = true
	m.current = newAssistantMessage()
	m.input.Blur()

	m.handleTurnDone(turnDoneMsg{result: runtime.TurnResult{
		SessionID:     "session-1",
		Usage:         model.Usage{InputTokens: 700, OutputTokens: 90, TotalTokens: 790},
		ContextWindow: 1000,
	}})

	if got := ansi.Strip(m.statusView()); got != "  gpt-5.6-sol high · Context 79% used" {
		t.Fatalf("statusView() = %q", got)
	}
	if !strings.Contains(m.statusView(), lightTheme.brand.Render("gpt-5.6-sol")) {
		t.Fatal("model name does not use the model status style")
	}
	if !strings.Contains(m.statusView(), lightTheme.reasoning.Render("high")) {
		t.Fatal("reasoning effort does not use the reasoning status style")
	}
	if !strings.Contains(m.statusView(), lightTheme.context.Render("Context 79% used")) {
		t.Fatal("context status does not use the context style")
	}
}

func TestToolCallUsesLifecycleStatusColors(t *testing.T) {
	tests := []struct {
		name  string
		call  toolCallView
		style lipgloss.Style
	}{
		{name: "running", call: toolCallView{call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`}}, style: lightTheme.working},
		{name: "completed", call: toolCallView{call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`}, done: true}, style: lightTheme.success},
		{name: "failed", call: toolCallView{call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`}, done: true, err: true}, style: lightTheme.error},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, _ := toolCallSummary(test.call)
			rendered := renderToolCall(test.call, 80, false)
			if !strings.Contains(rendered, test.style.Bold(true).Render(action)) {
				t.Fatalf("tool status does not use %s style: %q", test.name, rendered)
			}
		})
	}
}

func TestLoadModelStatusUsesRuntimeDefault(t *testing.T) {
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "gpt-5.6-sol",
				Providers: []config.ProviderConfig{{
					Name:   "openai",
					Format: config.ProviderFormatResponses,
					Models: []config.ProviderModel{{
						Value:         "gpt-5.6-sol",
						Name:          "gpt-5.6-sol",
						ContextWindow: 1000,
						ReasoningEfforts: []config.ProviderReasoningEffort{{
							Value: "high",
							Name:  "High",
						}},
					}},
				}},
			}, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	loaded, ok := loadModelStatus(t.Context(), rt)().(modelStatusLoadedMsg)
	if !ok {
		t.Fatal("loadModelStatus() returned an unexpected message type")
	}
	if loaded.err != nil {
		t.Fatalf("loadModelStatus() error = %v", loaded.err)
	}
	if len(loaded.models) != 1 || loaded.modelValue != "openai/gpt-5.6-sol" || loaded.modelName != "gpt-5.6-sol" || loaded.reasoningEffort != "high" || loaded.contextWindow != 1000 {
		t.Fatalf("loaded model status = %+v", loaded)
	}
}

func TestLoadSkillSummariesUsesRuntimeCWD(t *testing.T) {
	catalog, err := skill.NewCatalog([]skill.Skill{{Name: "think", Description: "Plan work"}})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	var loadedCWD string
	rt := runtime.New(runtime.Dependencies{
		LoadSkills: func(cwd string) (*skill.Catalog, error) {
			loadedCWD = cwd
			return catalog, nil
		},
	})

	loaded, ok := loadSkillSummaries(t.Context(), rt, "/tmp/atlas-work")().(skillSummariesLoadedMsg)
	if !ok {
		t.Fatal("loadSkillSummaries() returned an unexpected message type")
	}
	if loaded.err != nil {
		t.Fatalf("loadSkillSummaries() error = %v", loaded.err)
	}
	if loadedCWD != "/tmp/atlas-work" || len(loaded.summaries) != 1 || loaded.summaries[0].Name != "think" {
		t.Fatalf("loaded skill summaries: cwd=%q summaries=%+v", loadedCWD, loaded.summaries)
	}
}

type tuiRecordingProvider struct {
	selectedModel string
	request       model.ChatRequest
	response      model.ChatResponse
}

func (p *tuiRecordingProvider) Stream(_ context.Context, request model.ChatRequest, _ func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.request = request
	return p.response, nil
}

func TestToolResultTruncationPreservesUTF8(t *testing.T) {
	result := strings.Repeat("中", 27)
	rendered := renderToolCall(toolCallView{
		call:   model.ToolCall{Name: "run_shell", Arguments: `{"command":"read output"}`},
		result: result,
		done:   true,
	}, 80, false)

	if !utf8.ValidString(rendered) {
		t.Fatal("rendered tool result contains invalid UTF-8")
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 80 {
			t.Fatalf("rendered line width = %d, want at most 80", got)
		}
	}
}

func TestToolInputLimitsWrappedContinuationRows(t *testing.T) {
	rendered := ansi.Strip(renderToolCall(toolCallView{
		call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"` + strings.Repeat("abcdefghij", 20) + `"}`},
	}, 20, false))
	lines := strings.Split(rendered, "\n")

	if len(lines) != 4 {
		t.Fatalf("rendered input rows = %d, want header, two continuations, and ellipsis: %q", len(lines), rendered)
	}
	if !strings.Contains(lines[3], "… +") {
		t.Fatalf("last input row = %q, want omission marker", lines[3])
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 20 {
			t.Fatalf("rendered line width = %d, want at most 20: %q", got, line)
		}
	}
}

func TestToolInputOmissionMarkerFitsNarrowWidths(t *testing.T) {
	tc := toolCallView{
		call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"` + strings.Repeat("abcdefghij", 20) + `"}`},
	}
	for width := 5; width <= 10; width++ {
		rendered := renderToolCall(tc, width, false)
		for line := range strings.SplitSeq(rendered, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Fatalf("width %d rendered line width = %d: %q", width, got, ansi.Strip(line))
			}
		}
	}
}

func TestToolOutputKeepsHeadAndTailWithinScreenRowLimit(t *testing.T) {
	resultLines := make([]string, 12)
	for i := range resultLines {
		resultLines[i] = fmt.Sprintf("line %02d", i+1)
	}
	tc := toolCallView{
		call:   model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`},
		result: strings.Join(resultLines, "\n"),
		done:   true,
	}
	rendered := ansi.Strip(renderToolCall(tc, 40, false))
	lines := strings.Split(rendered, "\n")

	if got := len(lines) - 1; got != toolOutputRows {
		t.Fatalf("rendered output rows = %d, want %d: %q", got, toolOutputRows, rendered)
	}
	for _, want := range []string{"line 01", "line 12", "… +8 lines"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered output missing %q: %q", want, rendered)
		}
	}
	if tc.result != strings.Join(resultLines, "\n") {
		t.Fatal("rendering mutated the stored tool result")
	}
}

func TestToolOutputLimitCountsWrappedScreenRows(t *testing.T) {
	rendered := ansi.Strip(renderToolCall(toolCallView{
		call:   model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`},
		result: strings.Repeat("abcdefghij", 30),
		done:   true,
	}, 20, false))
	lines := strings.Split(rendered, "\n")

	if got := len(lines) - 1; got != toolOutputRows {
		t.Fatalf("wrapped output rows = %d, want %d: %q", got, toolOutputRows, rendered)
	}
	if !strings.Contains(rendered, "… +") {
		t.Fatalf("wrapped output missing omission marker: %q", rendered)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 20 {
			t.Fatalf("rendered line width = %d, want at most 20: %q", got, line)
		}
	}
}

func TestDirectShellOutputUsesExpandedRowLimit(t *testing.T) {
	result := strings.Repeat("0123456789\n", 12)
	rendered := ansi.Strip(renderToolCall(toolCallView{
		call:     model.ToolCall{Name: "run_shell", Arguments: `{"command":"test"}`},
		result:   result,
		metadata: model.ToolMetadata{DirectShell: true},
		done:     true,
	}, 40, false))

	if strings.Contains(rendered, "… +") {
		t.Fatalf("direct shell output was limited as a model tool call: %q", rendered)
	}
	if got := len(strings.Split(rendered, "\n")) - 1; got <= toolOutputRows || got > directShellOutputRows {
		t.Fatalf("direct shell output rows = %d, want > %d and <= %d", got, toolOutputRows, directShellOutputRows)
	}
}

func TestFailedToolAlwaysShowsResult(t *testing.T) {
	rendered := ansi.Strip(renderToolCall(toolCallView{
		call:   model.ToolCall{Name: "web_search", Arguments: `{"query":"atlas"}`},
		result: "network unavailable",
		err:    true,
		done:   true,
	}, 80, false))

	if !strings.Contains(rendered, "Failed to search the web for atlas") || !strings.Contains(rendered, "network unavailable") {
		t.Fatalf("failed tool did not show its action and result: %q", rendered)
	}
}

func TestSuccessfulSemanticToolHidesRawResult(t *testing.T) {
	rendered := ansi.Strip(renderToolCall(toolCallView{
		call:   model.ToolCall{Name: "web_search", Arguments: `{"query":"atlas"}`},
		result: "large raw result",
		done:   true,
	}, 80, false))

	if rendered != "• Searched the web for atlas" {
		t.Fatalf("rendered web search = %q", rendered)
	}
}

func TestToolCompletionsMatchCallsByID(t *testing.T) {
	message := newAssistantMessage()
	first := model.ToolCall{ID: "call-1", Name: "run_shell", Arguments: `{"command":"first"}`}
	second := model.ToolCall{ID: "call-2", Name: "run_shell", Arguments: `{"command":"second"}`}
	message.handleEvent(agent.Event{Type: agent.EventToolStarted, ToolCall: first})
	message.handleEvent(agent.Event{Type: agent.EventToolStarted, ToolCall: second})
	message.handleEvent(agent.Event{Type: agent.EventToolFinished, ToolCall: first, ToolResult: "one"})

	if !message.toolCalls[0].done || message.toolCalls[0].result != "one" || message.toolCalls[1].done {
		t.Fatalf("tool calls were not matched by ID: %#v", message.toolCalls)
	}
}

func TestMessageRenderingStripsTerminalControlSequences(t *testing.T) {
	const osc = "\x1b]52;c;dGVzdA==\x07"
	message := newAssistantMessage()
	message.handleEvent(agent.Event{Type: agent.EventModelDelta, Content: "answer " + osc})
	message.handleEvent(agent.Event{
		Type: agent.EventToolStarted,
		ToolCall: model.ToolCall{
			ID:        "call-1",
			Name:      "run_shell",
			Arguments: `{"command":"printf '\u001b]52;c;dGVzdA==\u0007'"}`,
		},
	})
	message.handleEvent(agent.Event{Type: agent.EventToolFinished, ToolResult: "output " + osc})
	message.err = errors.New("failed " + osc)

	rendered := message.render(80, false, nil)
	if strings.Contains(rendered, "]52;") {
		t.Fatalf("rendered output retained an OSC sequence: %q", rendered)
	}
}

func TestTurnUpdatesPreserveEventOrder(t *testing.T) {
	m := New(Options{})
	m.turnActive = true
	m.current = newAssistantMessage()
	m.messages = append(m.messages, m.current)
	m.input.Blur()

	event := agent.Event{Type: agent.EventModelDelta, Content: "final delta"}
	done := turnDoneMsg{result: runtime.TurnResult{SessionID: "session-1"}}
	updates := make(chan turnUpdateMsg, 2)
	updates <- turnUpdateMsg{event: &event}
	updates <- turnUpdateMsg{done: &done}
	close(updates)
	m.eventCh = updates

	first := pollTurnUpdates(updates)()
	updated, next := m.Update(first)
	m = updated.(Model)
	if next == nil {
		t.Fatal("event update did not schedule the next channel read")
	}

	second := next()
	updated, _ = m.Update(second)
	m = updated.(Model)
	if got := m.messages[0].content.String(); got != "final delta" {
		t.Fatalf("assistant content = %q, want final delta", got)
	}
	if m.turnActive {
		t.Fatal("turn remained active after ordered completion")
	}
	if !m.input.Focused() {
		t.Fatal("input remained blurred after ordered completion")
	}
}

func TestTurnRendersModelAndToolsChronologically(t *testing.T) {
	m := New(Options{})
	m.current = newAssistantMessage()
	m.messages = append(m.messages, m.current)
	call := model.ToolCall{ID: "call-1", Name: "run_shell", Arguments: `{"command":"pwd"}`}

	for _, event := range []agent.Event{
		{Type: agent.EventModelDelta, Content: "before tool"},
		{Type: agent.EventModelResponse},
		{Type: agent.EventToolStarted, ToolCall: call},
		{Type: agent.EventToolFinished, ToolCall: call, ToolResult: "/tmp/work"},
		{Type: agent.EventModelDelta, Content: "after tool"},
	} {
		m.handleAgentEvent(event)
	}

	var blocks []string
	for _, message := range m.messages {
		blocks = append(blocks, message.render(80, false, nil))
	}
	rendered := ansi.Strip(strings.Join(blocks, "\n"))
	before := strings.Index(rendered, "before tool")
	toolResult := strings.Index(rendered, "/tmp/work")
	after := strings.Index(rendered, "after tool")
	if before < 0 || toolResult < 0 || after < 0 {
		t.Fatalf("rendered output = %q", rendered)
	}
	if !(before < toolResult && toolResult < after) {
		t.Fatalf("render order = %q, want model text then tool result then later model text", rendered)
	}
}

func TestMessagesFromTranscriptRestoresToolResults(t *testing.T) {
	call := model.ToolCall{ID: "call-1", Name: "run_shell", Arguments: `{"command":"pwd"}`}
	messages := messagesFromTranscript([]model.Message{
		model.TextMessage(model.RoleUser, "Where am I?"),
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{call}},
		{Role: model.RoleTool, ToolCallID: call.ID, Content: "/tmp/work", ToolMetadata: model.ToolMetadata{Error: true, DirectShell: true}},
		model.TextMessage(model.RoleAssistant, "You are in /tmp/work."),
	})

	if len(messages) != 3 {
		t.Fatalf("rendered message count = %d, want 3", len(messages))
	}
	if len(messages[1].toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(messages[1].toolCalls))
	}
	toolCall := messages[1].toolCalls[0]
	if !toolCall.done || toolCall.result != "/tmp/work" || !toolCall.err || !toolCall.metadata.DirectShell {
		t.Fatalf("restored tool call = %+v", toolCall)
	}
	var blocks []string
	for _, message := range messages {
		blocks = append(blocks, message.render(80, false, nil))
	}
	rendered := ansi.Strip(strings.Join(blocks, "\n"))
	toolResult := strings.Index(rendered, "/tmp/work")
	finalAnswer := strings.Index(rendered, "You are in /tmp/work.")
	if toolResult < 0 || finalAnswer < 0 || toolResult >= finalAnswer {
		t.Fatalf("restored render order = %q, want tool result before final answer", rendered)
	}
}

func TestLoadSessionRestoresPersistedMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{Session: config.SessionConfig{DBPath: dbPath}}, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	_, err := rt.RunTurn(t.Context(), runtime.TurnOptions{
		SessionID: "work",
		Prompt:    "!pwd",
		CWD:       t.TempDir(),
		ToolRunner: func(context.Context, model.ToolCall, tool.RunFunc) (tool.RunResult, error) {
			return tool.RunResult{Content: "/tmp/work"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	loaded, ok := loadSession(t.Context(), rt, "work")().(sessionLoadedMsg)
	if !ok {
		t.Fatal("loadSession() returned an unexpected message type")
	}
	if loaded.err != nil {
		t.Fatalf("loadSession() error = %v", loaded.err)
	}
	rendered := messagesFromTranscript(loaded.messages)
	if len(rendered) != 2 {
		t.Fatalf("rendered message count = %d, want 2", len(rendered))
	}
	if got := rendered[1].toolCalls[0].result; got != "/tmp/work" {
		t.Fatalf("restored tool result = %q, want /tmp/work", got)
	}
}

func TestLoadSessionDoesNotHideUnrelatedNotFoundError(t *testing.T) {
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{}, errors.New("configuration not found")
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	loaded, ok := loadSession(t.Context(), rt, "work")().(sessionLoadedMsg)
	if !ok {
		t.Fatal("loadSession() returned an unexpected message type")
	}
	if loaded.err == nil {
		t.Fatal("loadSession() hid an unrelated not-found error")
	}
}
