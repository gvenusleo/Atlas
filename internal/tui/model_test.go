package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/storage"
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
	m.composer.WriteString("测试a")
	m.deleteLastInputRune()
	if got := m.composer.String(); got != "测试" {
		t.Fatalf("unexpected input after delete: %q", got)
	}
	m.deleteLastInputRune()
	if got := m.composer.String(); got != "测" {
		t.Fatalf("unexpected input after delete: %q", got)
	}
}

func TestNewModelStartsWithEmptyTranscript(t *testing.T) {
	m := newModel(nil, nil)
	if len(m.entries) != 0 {
		t.Fatalf("startup transcript should be empty: %#v", m.entries)
	}
	updated, _ := m.Update(sessionCreatedMsg(storage.Session{ID: "session-123456", Model: "deepseek"}))
	next := updated.(model)
	if len(next.entries) != 0 {
		t.Fatalf("session creation should not append transcript entries: %#v", next.entries)
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

func TestPromptWrapsLongInput(t *testing.T) {
	m := model{width: 16}
	m.composer.WriteString("abcdefghijklmnopqrstuvwxyz")
	got, _, _, _ := m.renderPromptLine(16)
	if strings.Count(got, "\n") < 1 {
		t.Fatalf("prompt should wrap long input: %q", got)
	}
	if !strings.Contains(got, "> abcdefghijklmn") || !strings.Contains(got, "  opqrstuvwxyz") {
		t.Fatalf("prompt should render wrapped prompt lines: %q", got)
	}
}

func TestPromptReturnsCursorAfterCommittedInput(t *testing.T) {
	m := model{width: 32}
	m.composer.WriteString("yi xia 与 rust比较")
	got, row, col, visible := m.renderPromptLine(32)
	if strings.Count(got, "\n") != 0 {
		t.Fatalf("prompt should stay on one line: %q", got)
	}
	if !strings.Contains(got, "> yi xia 与 rust比较") {
		t.Fatalf("prompt should keep committed input on prompt line: %q", got)
	}
	if row != 1 || !visible || col <= lipgloss.Width(promptPrefix) {
		t.Fatalf("cursor should be after prompt prefix: row=%d visible=%v col=%d", row, visible, col)
	}
}

func TestWrapDisplayLineUsesDisplayWidth(t *testing.T) {
	got := wrapDisplayLine("yi xia 与 rust比较", 16)
	for _, line := range got {
		if lipgloss.Width(line) > 16 {
			t.Fatalf("wrapped line should fit display width: width=%d text=%q", lipgloss.Width(line), line)
		}
	}
}

func TestUpdateAcceptsSpaceKey(t *testing.T) {
	m := model{}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: " ", Code: tea.KeySpace}))
	next := updated.(model)
	got := next.composer.String()
	if got != " " {
		t.Fatalf("space key was not appended: %q", got)
	}
}

func TestCtrlJInsertsComposerNewline(t *testing.T) {
	m := model{}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "one"}))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: 'j'}))
	m = updated.(model)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "two"}))
	m = updated.(model)

	if got := m.composer.String(); got != "one\ntwo" {
		t.Fatalf("unexpected composer text: %q", got)
	}
}

func TestMultilineComposerCapsAtTenRows(t *testing.T) {
	var c composerState
	for i := 0; i < 12; i++ {
		if i > 0 {
			c.WriteRune('\n')
		}
		c.WriteString("line " + strconv.Itoa(i))
	}
	if got := c.Height(80, false); got != maxComposerRows {
		t.Fatalf("composer height should cap at %d: %d", maxComposerRows, got)
	}
	rendered, row, _, visible := c.Render(80)
	if lines := strings.Count(rendered, "\n") + 1; lines != maxComposerRows {
		t.Fatalf("rendered composer should have %d rows, got %d in %q", maxComposerRows, lines, rendered)
	}
	if !strings.Contains(rendered, "line 11") || strings.Contains(rendered, "line 0") {
		t.Fatalf("composer should show the bottom window by default: %q", rendered)
	}
	if row != maxComposerRows || !visible {
		t.Fatalf("cursor should be on final visible row at bottom: row=%d visible=%v", row, visible)
	}
}

func TestComposerScrollsInsideOverflow(t *testing.T) {
	var c composerState
	for i := 0; i < 12; i++ {
		if i > 0 {
			c.WriteRune('\n')
		}
		c.WriteString("line " + strconv.Itoa(i))
	}
	if !c.Scroll(1, 80) {
		t.Fatal("expected composer scroll to consume the key")
	}
	rendered, _, _, visible := c.Render(80)
	if !strings.Contains(rendered, "line 1") || strings.Contains(rendered, "line 11") {
		t.Fatalf("unexpected scrolled composer window: %q", rendered)
	}
	if visible {
		t.Fatal("cursor should hide while composer is scrolled away from bottom")
	}
}

func TestUpdateScrollsComposerWhenOverflowed(t *testing.T) {
	m := model{width: 80}
	for i := 0; i < 12; i++ {
		if i > 0 {
			m.composer.WriteRune('\n')
		}
		m.composer.WriteString("line " + strconv.Itoa(i))
	}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	next := updated.(model)
	if next.composer.scroll != 1 {
		t.Fatalf("up should scroll overflowing composer, got %d", next.composer.scroll)
	}
	if next.scroll != 0 {
		t.Fatalf("transcript scroll should not move while composer consumes up: %d", next.scroll)
	}

	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	next = updated.(model)
	if next.composer.scroll != 0 {
		t.Fatalf("down should scroll composer back to bottom, got %d", next.composer.scroll)
	}
}

func TestBodyHeightShrinksForMultilineComposer(t *testing.T) {
	short := model{width: 80, height: 20}
	tall := model{width: 80, height: 20}
	for i := 0; i < 5; i++ {
		if i > 0 {
			tall.composer.WriteRune('\n')
		}
		tall.composer.WriteString("line")
	}
	if short.bodyHeight() <= tall.bodyHeight() {
		t.Fatalf("body should shrink when composer grows: short=%d tall=%d", short.bodyHeight(), tall.bodyHeight())
	}
}

func TestRenderEntryOmitsRoleLabels(t *testing.T) {
	user := strings.Join(renderEntry(entry{kind: entryUser, body: "hello"}, 80), "\n")
	assistant := strings.Join(renderEntry(entry{kind: entryAssistant, body: "hi"}, 80), "\n")
	if strings.Contains(user, "You") || strings.Contains(assistant, "Atlas") {
		t.Fatalf("role labels should be omitted: user=%q assistant=%q", user, assistant)
	}
	if !strings.Contains(user, "› hello") || !strings.Contains(assistant, "hi") {
		t.Fatalf("unexpected rendered entries: user=%q assistant=%q", user, assistant)
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
