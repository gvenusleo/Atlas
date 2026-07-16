package tui

import (
	"context"
	"errors"
	"image/color"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
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
	m := New(Options{})
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
	if strings.Contains(lines[0], "Atlas") {
		t.Fatalf("top line still contains a header: %q", lines[0])
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

func TestComposerBackgroundMatchesCodexBlend(t *testing.T) {
	tests := []struct {
		name       string
		background color.Color
		dark       bool
		want       color.RGBA
	}{
		{name: "black", background: color.Black, dark: true, want: color.RGBA{R: 30, G: 30, B: 30, A: 255}},
		{name: "dark gray", background: color.RGBA{R: 40, G: 44, B: 52, A: 255}, dark: true, want: color.RGBA{R: 65, G: 69, B: 76, A: 255}},
		{name: "white", background: color.White, want: color.RGBA{R: 244, G: 244, B: 244, A: 255}},
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
		title:  "Explored",
		result: "Search Atlas",
		done:   true,
	})

	rendered := ansi.Strip(message.render(40, false, nil))
	want := "• first line second line\n\n• Explored\n  Search Atlas"
	if rendered != want {
		t.Fatalf("rendered message = %q, want %q", rendered, want)
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
	if !strings.HasPrefix(lines[0], "• ") {
		t.Fatalf("first markdown line = %q, want assistant marker", lines[0])
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("continuation markdown line = %q, want two-space indent", line)
		}
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

func TestAssistantMarkdownDoesNotInsertSpacesIntoCJK(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("## 核心特点\n\nGo 是一门静态类型、编译型编程语言。\n\n- **简洁易学**：语法精简，关键字少\n- **编译速度快**：大型项目也能快速编译")

	rendered := ansi.Strip(message.render(40, false, nil))
	want := "• 核心特点\n  \n  Go 是一门静态类型、编译型编程语言。\n  \n  • 简洁易学：语法精简，关键字少\n  • 编译速度快：大型项目也能快速编译"
	if rendered != want {
		t.Fatalf("rendered CJK markdown = %q, want %q", rendered, want)
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 40 {
			t.Fatalf("rendered line width = %d, want at most 40: %q", got, line)
		}
	}
}

func TestAssistantMarkdownPreservesBlankLineBeforeHeading(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("Go 适合构建后端服务。\n\n## 核心特点")

	rendered := ansi.Strip(message.render(40, false, nil))
	if !strings.Contains(rendered, "Go 适合构建后端服务。\n  \n  核心特点") {
		t.Fatalf("rendered markdown lost the blank line before a heading: %q", rendered)
	}
}

func TestMarkdownInlineCodeHasNoBackground(t *testing.T) {
	for _, dark := range []bool{false, true} {
		if background := markdownStyle(dark).Code.BackgroundColor; background != nil {
			t.Fatalf("markdownStyle(%t) inline code background = %q, want nil", dark, *background)
		}
	}
}

func TestAssistantMarkdownReflowsBlockquoteWithoutInlineGutters(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("> **Go 是工程效率优先的语言**,牺牲一些性能换取简单、可读、易上手、快速编译,适合构建大规模后端服务。\n> **Rust 是性能与安全优先的语言**,牺牲一些开发效率换取零成本抽象和编译期保证,适合对性能要求苛刻的场景。")

	rendered := ansi.Strip(message.render(44, true, nil))
	var content strings.Builder
	for i, line := range strings.Split(rendered, "\n") {
		prefix := "  │ "
		if i == 0 {
			prefix = "• │ "
		}
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("blockquote line %d = %q, want prefix %q", i, line, prefix)
		}
		if strings.Contains(strings.TrimPrefix(line, prefix), "│") {
			t.Fatalf("blockquote gutter leaked into line %d content: %q", i, line)
		}
		content.WriteString(strings.TrimPrefix(line, prefix))
		if got := ansi.StringWidth(line); got > 44 {
			t.Fatalf("blockquote line width = %d, want at most 44: %q", got, line)
		}
	}
	if joined := content.String(); !strings.Contains(joined, "Go 是工程效率优先的语言") || !strings.Contains(joined, "Rust 是性能与安全优先的语言") {
		t.Fatalf("blockquote content was split unexpectedly: %q", rendered)
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

func TestAssistantContentUsesDefaultTerminalColor(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("plain response")

	rendered := message.render(40, false, nil)
	if rendered != ansi.Strip(rendered) {
		t.Fatalf("assistant response contains foreground styling: %q", rendered)
	}
}

func TestAssistantDoesNotInsertLineBreakBeforeASCIIWord(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("你好！我是 Atlas，可以帮你处理本地文件、运行命令、写代码、查资料等。有什么需要帮忙的吗？")

	rendered := ansi.Strip(message.render(60, false, nil))
	if strings.Contains(rendered, "我是\n  Atlas") {
		t.Fatalf("assistant rendering inserted an unexpected line break: %q", rendered)
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
	if !strings.Contains(m.statusView(), userStyle.Render("gpt-5.6-sol high")) {
		t.Fatal("model name and reasoning effort do not share the model status style")
	}
	if !strings.Contains(m.statusView(), assistantStyle.Render("Context 79% used")) {
		t.Fatal("context status does not use one shared style")
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
	if loaded.modelName != "gpt-5.6-sol" || loaded.reasoningEffort != "high" || loaded.contextWindow != 1000 {
		t.Fatalf("loaded model status = %+v", loaded)
	}
}

func TestToolResultTruncationPreservesUTF8(t *testing.T) {
	result := strings.Repeat("中", 27)
	rendered := renderToolCall(toolCallView{
		title:  "Read output",
		result: result,
		done:   true,
	}, 80)

	if !utf8.ValidString(rendered) {
		t.Fatal("rendered tool result contains invalid UTF-8")
	}
	for line := range strings.SplitSeq(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 80 {
			t.Fatalf("rendered line width = %d, want at most 80", got)
		}
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
	rendered := strings.Join(blocks, "\n")
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
		{Role: model.RoleTool, ToolCallID: call.ID, Content: "/tmp/work"},
		model.TextMessage(model.RoleAssistant, "You are in /tmp/work."),
	})

	if len(messages) != 3 {
		t.Fatalf("rendered message count = %d, want 3", len(messages))
	}
	if len(messages[1].toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(messages[1].toolCalls))
	}
	toolCall := messages[1].toolCalls[0]
	if !toolCall.done || toolCall.result != "/tmp/work" {
		t.Fatalf("restored tool call = %+v", toolCall)
	}
	var blocks []string
	for _, message := range messages {
		blocks = append(blocks, message.render(80, false, nil))
	}
	rendered := strings.Join(blocks, "\n")
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
