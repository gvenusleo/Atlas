package tui

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liuyuxin/atlas/internal/agent"
)

func TestSummarizeToolOutputLimitsPreview(t *testing.T) {
	got := summarizeToolOutput("a\nb\nc\nd\ne\nf")
	if !strings.Contains(got, "… 2 more line(s)") {
		t.Fatalf("missing truncation marker: %q", got)
	}
	if strings.Contains(got, "\nf") {
		t.Fatalf("preview should not include all lines: %q", got)
	}
}

func TestDeleteLastInputRuneHandlesChinese(t *testing.T) {
	m := model{}
	m.input.WriteString("测试a")
	m.deleteLastInputRune()
	if got := m.input.String(); got != "测试" {
		t.Fatalf("unexpected input after delete: %q", got)
	}
	m.deleteLastInputRune()
	if got := m.input.String(); got != "测" {
		t.Fatalf("unexpected input after delete: %q", got)
	}
}

func TestRenderEntryIncludesToolPreview(t *testing.T) {
	lines := renderEntry(entry{
		kind:  entryTool,
		title: "read_file",
		body:  "README.md\nok",
	}, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read_file") || !strings.Contains(joined, "README.md") {
		t.Fatalf("unexpected render: %q", joined)
	}
}

func TestAppendEventKeepsPostToolTextInOrder(t *testing.T) {
	m := model{}
	m.appendEvent(agent.Event{Type: agent.EventTextDelta, Text: "before"})
	m.appendEvent(agent.Event{Type: agent.EventToolStarted, ToolName: "read_file"})
	m.appendEvent(agent.Event{Type: agent.EventToolFinished, ToolName: "read_file", Text: "ok"})
	m.appendEvent(agent.Event{Type: agent.EventTextDelta, Text: "after"})

	if len(m.entries) != 3 {
		t.Fatalf("unexpected entry count: %d", len(m.entries))
	}
	if m.entries[0].kind != entryAssistant || m.entries[0].body != "before" {
		t.Fatalf("unexpected first entry: %#v", m.entries[0])
	}
	if m.entries[1].kind != entryTool || m.entries[1].body != "ok" {
		t.Fatalf("unexpected tool entry: %#v", m.entries[1])
	}
	if m.entries[2].kind != entryAssistant || m.entries[2].body != "after" {
		t.Fatalf("unexpected final entry: %#v", m.entries[2])
	}
}

func TestRenderTranscriptPadsToFixedHeight(t *testing.T) {
	m := model{entries: []entry{{kind: entryMeta, title: "Atlas", body: "ready"}}}
	got := m.renderTranscript(80, 5)
	if lines := strings.Count(got, "\n") + 1; lines != 5 {
		t.Fatalf("unexpected transcript height: %d in %q", lines, got)
	}
}

func TestPromptKeepsTailOfLongInput(t *testing.T) {
	m := model{width: 16}
	m.input.WriteString("abcdefghijklmnopqrstuvwxyz")
	got, _, _ := m.renderPromptLine(16)
	if !strings.Contains(got, "…tuvwxyz") {
		t.Fatalf("prompt should keep input tail: %q", got)
	}
}

func TestPromptReturnsCursorAfterCommittedInput(t *testing.T) {
	m := model{width: 24}
	m.input.WriteString("yi xia 与 rust比较")
	got, col, visible := m.renderPromptLine(24)
	if strings.Count(got, "\n") != 0 {
		t.Fatalf("prompt should stay on one line: %q", got)
	}
	if !strings.Contains(got, "atlas> …xia 与 rust比较") {
		t.Fatalf("prompt should keep committed input on prompt line: %q", got)
	}
	if !visible || col <= utf8.RuneCountInString("atlas> ") {
		t.Fatalf("cursor should be after prompt prefix: visible=%v col=%d", visible, col)
	}
}

func TestTailFitUsesDisplayWidth(t *testing.T) {
	got := tailFit("yi xia 与 rust比较", 16)
	if lipgloss.Width(got) > 16 {
		t.Fatalf("tail should fit display width: width=%d text=%q", lipgloss.Width(got), got)
	}
}

func TestUpdateAcceptsSpaceKey(t *testing.T) {
	m := model{}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next := updated.(model)
	got := next.input.String()
	if got != " " {
		t.Fatalf("space key was not appended: %q", got)
	}
}

func TestRenderTranscriptUsesScrollOffset(t *testing.T) {
	m := model{width: 80, height: 10}
	for i := 0; i < 8; i++ {
		m.entries = append(m.entries, entry{kind: entryMeta, title: "line " + strconv.Itoa(i)})
	}
	bottom := m.renderTranscript(80, 3)
	m.scrollTranscript(2)
	scrolled := m.renderTranscript(80, 3)

	if bottom == scrolled {
		t.Fatalf("scroll should change transcript window: %q", bottom)
	}
	if !strings.Contains(scrolled, "line 3") || strings.Contains(scrolled, "line 7") {
		t.Fatalf("unexpected scrolled transcript: %q", scrolled)
	}
}

func TestScrolledTranscriptDoesNotJumpOnNewOutput(t *testing.T) {
	m := model{width: 80, height: 10}
	for i := 0; i < 8; i++ {
		m.entries = append(m.entries, entry{kind: entryMeta, title: "line " + strconv.Itoa(i)})
	}
	m.scrollTranscript(2)
	m.appendEntry(entry{kind: entryMeta, title: "new"})

	got := m.renderTranscript(80, 3)
	if !strings.Contains(got, "line 3") || strings.Contains(got, "new") {
		t.Fatalf("new output should not move manual scroll: %q", got)
	}
}
