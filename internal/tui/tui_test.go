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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	if got := lipgloss.Height(m.View().Content); got != 24 {
		t.Fatalf("View height = %d, want 24", got)
	}
	if got := m.viewport.Height(); got != 21 {
		t.Fatalf("viewport height = %d, want 21", got)
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

	rendered := message.render(80)
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
		blocks = append(blocks, message.render(80))
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
		blocks = append(blocks, message.render(80))
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
