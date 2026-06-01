package tui

import (
	"strings"
	"testing"

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
	got := m.renderPrompt(16)
	if !strings.Contains(got, "…stuvwxyz") {
		t.Fatalf("prompt should keep input tail: %q", got)
	}
}
