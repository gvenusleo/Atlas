package tui

import (
	"context"
	"errors"
	"path/filepath"
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
	composer := composerStyle(m.hasDarkBackground).Width(m.width).Render(composerState.content)
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
	rendered := newUserMessage("hello").render(20, false)

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

	assertWordsShareVisualLine(t, ansi.Strip(message.render(60, false)), "我是", "Atlas")
}

func TestAssistantAndToolBlocksUseBulletsAndIndentedContent(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("first line\nsecond line")
	message.toolCalls = append(message.toolCalls, toolCallView{
		title:  "Explored",
		result: "Search Atlas",
		done:   true,
	})

	rendered := ansi.Strip(message.render(40, false))
	want := "• first line\n  second line\n\n• Explored\n  Search Atlas"
	if rendered != want {
		t.Fatalf("rendered message = %q, want %q", rendered, want)
	}
}

func TestAssistantContentUsesDefaultTerminalColor(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("plain response")

	rendered := message.render(40, false)
	if rendered != ansi.Strip(rendered) {
		t.Fatalf("assistant response contains foreground styling: %q", rendered)
	}
}

func TestAssistantDoesNotInsertLineBreakBeforeASCIIWord(t *testing.T) {
	message := newAssistantMessage()
	message.content.WriteString("你好！我是 Atlas，可以帮你处理本地文件、运行命令、写代码、查资料等。有什么需要帮忙的吗？")

	rendered := ansi.Strip(message.render(60, false))
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

	rendered := message.render(80, false)
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
		blocks = append(blocks, message.render(80, false))
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
		blocks = append(blocks, message.render(80, false))
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
