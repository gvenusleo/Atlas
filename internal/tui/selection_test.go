package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTextSelectionExtractsStyledAndWideText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		anchor  selectionPoint
		cursor  selectionPoint
		want    string
	}{
		{
			name:    "styled text",
			content: "\x1b[34mhello world\x1b[0m",
			anchor:  selectionPoint{x: 1},
			cursor:  selectionPoint{x: 4},
			want:    "ello",
		},
		{
			name:    "reverse CJK drag",
			content: "A 你好 B",
			anchor:  selectionPoint{x: 5},
			cursor:  selectionPoint{x: 2},
			want:    "你好",
		},
		{
			name:    "emoji grapheme",
			content: "A 👩‍💻 B",
			anchor:  selectionPoint{x: 2},
			cursor:  selectionPoint{x: 3},
			want:    "👩‍💻",
		},
		{
			name:    "multiple lines",
			content: "first\nsecond",
			anchor:  selectionPoint{x: 2},
			cursor:  selectionPoint{x: 2, y: 1},
			want:    "rst\nsec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection := textSelection{active: true, dragged: true, anchor: tt.anchor, cursor: tt.cursor}
			if got := selection.content(tt.content); got != tt.want {
				t.Fatalf("selected content = %q, want %q", got, tt.want)
			}
			rendered := selection.render(tt.content, lightTheme.selection)
			if ansi.Strip(rendered) != ansi.Strip(tt.content) {
				t.Fatalf("highlight changed visible content: %q", rendered)
			}
			if rendered == tt.content {
				t.Fatal("selection did not add highlight styling")
			}
		})
	}
}

func TestMouseDragCopiesSelectionWithoutDisablingWheel(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updated.(Model)
	m.viewport.SetContent("hello world")

	updated, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: 1, Y: 0, Button: tea.MouseLeft}))
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMotionMsg(tea.Mouse{X: 4, Y: 0, Button: tea.MouseLeft}))
	m = updated.(Model)
	if !m.selection.active || !m.selection.dragged {
		t.Fatalf("selection state = %+v, want active drag", m.selection)
	}
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want MouseModeCellMotion", got)
	}
	if view := m.View().Content; !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("dragged selection is not highlighted: %q", view)
	}

	updated, cmd := m.Update(tea.MouseReleaseMsg(tea.Mouse{X: 4, Y: 0}))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("mouse release did not schedule clipboard copy")
	}
	msg := cmd()
	copyMsg, ok := msg.(copySelectionMsg)
	if !ok {
		t.Fatalf("clipboard command returned %T, want copySelectionMsg", msg)
	}
	if copyMsg.text != "ello" {
		t.Fatalf("copied text = %q, want %q", copyMsg.text, "ello")
	}

	updated, clipboardCmd := m.Update(copyMsg)
	m = updated.(Model)
	if m.selection.active {
		t.Fatal("selection remained active after clipboard copy")
	}
	if clipboardCmd == nil {
		t.Fatal("copySelectionMsg did not produce a clipboard command")
	}
}

func TestMouseClickWithoutDragDoesNotCopy(t *testing.T) {
	m := New(Options{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updated.(Model)
	m.viewport.SetContent("hello")

	updated, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: 1, Y: 0, Button: tea.MouseLeft}))
	m = updated.(Model)
	updated, cmd := m.Update(tea.MouseReleaseMsg(tea.Mouse{X: 1, Y: 0}))
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("plain click scheduled clipboard copy")
	}
	if m.selection.active {
		t.Fatal("plain click left an active selection")
	}
}

func TestClearSelectionRestoresFollowMode(t *testing.T) {
	m := New(Options{})
	m.viewport.SetWidth(20)
	m.viewport.SetHeight(1)
	m.viewport.SetContent("one\ntwo")
	m.viewport.GotoBottom()
	m.selection.begin(selectionPoint{}, true)
	m.viewport.SetContent("one\ntwo\nthree")

	m.clearSelection(true)

	if !m.viewport.AtBottom() {
		t.Fatalf("viewport offset = %d, want bottom after selection", m.viewport.YOffset())
	}
}

func TestSelectionRenderOnlyChangesSelectedCells(t *testing.T) {
	content := "alpha\nbeta"
	selection := textSelection{
		active:  true,
		dragged: true,
		anchor:  selectionPoint{x: 2},
		cursor:  selectionPoint{x: 1, y: 1},
	}

	rendered := selection.render(content, lightTheme.selection)
	if ansi.Strip(rendered) != content {
		t.Fatalf("rendered content = %q, want %q", ansi.Strip(rendered), content)
	}
	if strings.Count(rendered, "\x1b[") == 0 {
		t.Fatalf("rendered selection contains no ANSI styling: %q", rendered)
	}
}
