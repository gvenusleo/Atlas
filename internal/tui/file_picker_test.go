package tui

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCurrentFileMentionTargetUsesTokenAtCursor(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		cursor      int
		wantToken   string
		wantQuery   string
		wantPresent bool
	}{
		{name: "start", value: "@src", cursor: 4, wantToken: "@src", wantQuery: "src", wantPresent: true},
		{name: "after whitespace", value: "read @源.go next", cursor: 10, wantToken: "@源.go", wantQuery: "源.go", wantPresent: true},
		{name: "email", value: "me@example.com", cursor: 14},
		{name: "cursor after whitespace", value: "@src next", cursor: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := New(Options{})
			m.input.SetValue(test.value)
			m.input.SetCursorColumn(test.cursor)
			target, ok := currentFileMentionTarget(m.input)
			if ok != test.wantPresent || target.token != test.wantToken || target.query != test.wantQuery {
				t.Fatalf("target = %#v, %t", target, ok)
			}
		})
	}
}

func TestCurrentFileMentionTargetSupportsMultilineDraft(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("first\nuse @internal/tui")
	m.input.MoveToEnd()

	target, ok := currentFileMentionTarget(m.input)
	if !ok || target.line != 1 || target.token != "@internal/tui" || target.query != "internal/tui" {
		t.Fatalf("target = %#v, %t", target, ok)
	}
}

func TestReplaceFileMentionPreservesDraftAndCursor(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("read @src next")
	m.input.SetCursorColumn(len("read @src"))
	target, ok := currentFileMentionTarget(m.input)
	if !ok {
		t.Fatal("file target is missing")
	}

	if !replaceFileMention(&m.input, target, "internal/tui/tui.go") {
		t.Fatal("replaceFileMention() = false")
	}
	if got := m.input.Value(); got != "read internal/tui/tui.go next" {
		t.Fatalf("input value = %q", got)
	}
	if got := m.input.Column(); got != len("read internal/tui/tui.go ") {
		t.Fatalf("cursor column = %d", got)
	}
}

func TestReplaceFileMentionQuotesWhitespaceAndAddsSeparator(t *testing.T) {
	m := New(Options{})
	m.input.SetValue("@my")
	m.input.MoveToEnd()
	target, _ := currentFileMentionTarget(m.input)

	if !replaceFileMention(&m.input, target, "docs/my file.md") {
		t.Fatal("replaceFileMention() = false")
	}
	if got := m.input.Value(); got != `"docs/my file.md" ` {
		t.Fatalf("input value = %q", got)
	}
}

func TestFileMentionPickerFiltersAndRendersForegroundSelection(t *testing.T) {
	picker := fileMentionPicker{
		mode:    filePickerSearch,
		target:  fileMentionTarget{query: "tm"},
		catalog: []string{"README.md", "internal/tui/messages.go", "internal/tui/tui.go"},
	}
	picker.filter()
	if len(picker.matches) != 1 || picker.matches[0] != "internal/tui/messages.go" {
		t.Fatalf("matches = %#v", picker.matches)
	}
	rendered := picker.render(60, maxFilePopupRows, nil)
	if !strings.Contains(rendered, userStyle.Render("› internal/tui/messages.go")) {
		t.Fatalf("selected row = %q", rendered)
	}
	if strings.Contains(ansi.Strip(rendered), "README.md") {
		t.Fatalf("unmatched path rendered: %q", rendered)
	}
}

func TestWalkWorkspaceFilesSkipsVCSDirectoriesAndCapsResults(t *testing.T) {
	root := t.TempDir()
	writePickerTestFile(t, filepath.Join(root, ".git", "config"))
	writePickerTestFile(t, filepath.Join(root, "a.txt"))
	writePickerTestFile(t, filepath.Join(root, "b.txt"))
	writePickerTestFile(t, filepath.Join(root, "c.txt"))

	paths, truncated, err := walkWorkspaceFiles(context.Background(), root, 2)
	if err != nil {
		t.Fatalf("walkWorkspaceFiles() error = %v", err)
	}
	if !truncated || len(paths) != 2 {
		t.Fatalf("walk result = %#v, truncated=%t", paths, truncated)
	}
	for _, path := range paths {
		if strings.Contains(path, ".git") {
			t.Fatalf("VCS path included: %q", path)
		}
	}
}

func TestFileMentionSelectionDoesNotSubmit(t *testing.T) {
	m := New(Options{CWD: "/work"})
	m.input.SetValue("check @mai")
	m.input.MoveToEnd()
	target, _ := currentFileMentionTarget(m.input)
	m.filePicker = fileMentionPicker{
		mode:    filePickerSearch,
		cwd:     "/work",
		target:  target,
		catalog: []string{"src/main.go"},
		matches: []string{"src/main.go"},
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || len(m.messages) != 0 || m.turnActive {
		t.Fatalf("selection submitted: cmd=%v messages=%d active=%t", cmd, len(m.messages), m.turnActive)
	}
	if got := m.input.Value(); got != "check src/main.go " {
		t.Fatalf("input value = %q", got)
	}
}

func TestBareAtOpensBrowserAndSelectsRelativePath(t *testing.T) {
	root := t.TempDir()
	writePickerTestFile(t, filepath.Join(root, "note.txt"))
	m := New(Options{CWD: root})
	m.input.SetValue("@")
	m.input.MoveToEnd()
	target, _ := currentFileMentionTarget(m.input)
	m.filePicker = fileMentionPicker{mode: filePickerSearch, cwd: root, target: target}

	updated, initCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if initCmd == nil || !m.filePicker.browsing() || m.input.Focused() {
		t.Fatalf("browser state: cmd=%v browsing=%t focused=%t", initCmd, m.filePicker.browsing(), m.input.Focused())
	}
	updated, _ = m.Update(initCmd())
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.filePicker.active() || !m.input.Focused() || m.input.Value() != "note.txt " {
		t.Fatalf("selection state: active=%t focused=%t value=%q", m.filePicker.active(), m.input.Focused(), m.input.Value())
	}
}

func TestFileBrowserRenderUsesPlainBackgroundAndSpacedTitle(t *testing.T) {
	root := t.TempDir()
	writePickerTestFile(t, filepath.Join(root, "cmd", "main.go"))
	writePickerTestFile(t, filepath.Join(root, "dist", "atlas"))
	picker := fileMentionPicker{mode: filePickerSearch, cwd: root}
	initCmd := picker.openBrowser()
	_, _, _ = picker.updateBrowser(initCmd())

	background := color.RGBA{R: 244, G: 244, B: 244, A: 255}
	rendered := picker.renderBrowser(40, maxFilePopupRows, background)
	lines := strings.Split(rendered, "\n")
	if ansi.Strip(lines[0]) != "  Browse · ." || lines[1] != "" {
		t.Fatalf("browser title = %q", ansi.Strip(rendered))
	}
	if lines[0] != "  "+fileBrowserStyle(messageStyle.Bold(true), background).Render("Browse · .") {
		t.Fatalf("browser title style = %q", lines[0])
	}
	if !strings.HasPrefix(lines[3], "  ") {
		t.Fatalf("unselected browser row resets the background before its leading spaces: %q", lines[3])
	}
	styles := []lipgloss.Style{
		fileBrowserStyle(picker.browser.Styles.Selected, background),
		fileBrowserStyle(picker.browser.Styles.Directory, background),
		fileBrowserStyle(picker.browser.Styles.File, background),
		fileBrowserStyle(picker.browser.Styles.Symlink, background),
	}
	for _, style := range styles {
		if !reflect.DeepEqual(style.GetBackground(), background) {
			t.Fatal("file browser item style does not match the composer background")
		}
	}
}

func TestFileBrowserPopupStaysInsideComposerBackground(t *testing.T) {
	root := t.TempDir()
	writePickerTestFile(t, filepath.Join(root, "cmd", "main.go"))
	m := New(Options{CWD: root})
	m.input.SetValue("@")
	m.input.MoveToEnd()
	target, _ := currentFileMentionTarget(m.input)
	m.filePicker = fileMentionPicker{mode: filePickerSearch, cwd: root, target: target}
	initCmd := m.filePicker.openBrowser()
	_, _, _ = m.filePicker.updateBrowser(initCmd())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 14})
	m = updated.(Model)

	inputArea := m.renderInputArea()
	expected := composerStyle(m.hasDarkBackground, m.terminalBackground).
		Width(m.width).
		Render(inputArea.content)
	if !strings.Contains(m.View().Content, expected) {
		t.Fatal("file browser popup is rendered outside the composer background")
	}
}

func TestFileBrowserKeepsThreeCandidatesVisibleWhileScrolling(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		writePickerTestFile(t, filepath.Join(root, directory, "file"))
	}
	picker := fileMentionPicker{mode: filePickerSearch, cwd: root}
	initCmd := picker.openBrowser()
	_, _, _ = picker.updateBrowser(initCmd())

	for move, wantRow := range []int{0, 1, 1, 1, 1, 1, 1, 2} {
		if move > 0 {
			_, _, _ = picker.updateBrowser(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		selected := filepath.Base(picker.browser.HighlightedPath())
		lines := strings.Split(ansi.Strip(picker.renderBrowser(40, maxFilePopupRows, color.White)), "\n")
		if len(lines) != maxFilePopupRows {
			t.Fatalf("move %d rendered rows = %d, want %d: %q", move, len(lines), maxFilePopupRows, lines)
		}
		for row, line := range lines[2:] {
			if strings.TrimSpace(line) == "" {
				t.Fatalf("move %d candidate row %d is empty: %q", move, row, lines)
			}
		}
		selectedRow := -1
		for i, line := range lines[2:] {
			if strings.HasPrefix(line, "› ") {
				selectedRow = i
				break
			}
		}
		if selectedRow != wantRow {
			t.Fatalf("move %d selected item %q row = %d, want %d: %q", move, selected, selectedRow, wantRow, lines)
		}
	}
}

func TestFileBrowserCannotLeaveWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	picker := fileMentionPicker{mode: filePickerSearch, cwd: root}
	_ = picker.openBrowser()

	_, _, _ = picker.updateBrowser(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !samePath(picker.browser.CurrentDirectory, root) {
		t.Fatalf("browser directory = %q, want %q", picker.browser.CurrentDirectory, root)
	}
}

func TestFileBrowserCannotFollowDirectorySymlinkOutsideWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writePickerTestFile(t, filepath.Join(outside, "outside.txt"))
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Skipf("Symlink() error = %v", err)
	}
	picker := fileMentionPicker{mode: filePickerSearch, cwd: root}
	initCmd := picker.openBrowser()
	_, _, _ = picker.updateBrowser(initCmd())

	_, _, _ = picker.updateBrowser(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !samePath(picker.browser.CurrentDirectory, root) {
		t.Fatalf("browser directory = %q, want %q", picker.browser.CurrentDirectory, root)
	}
}

func TestFileCatalogIgnoresStaleGenerationAndCWD(t *testing.T) {
	picker := fileMentionPicker{mode: filePickerSearch, cwd: "/work", generation: 2, loading: true}
	if picker.acceptCatalog(fileCatalogLoadedMsg{cwd: "/work", generation: 1, paths: []string{"stale"}}) {
		t.Fatal("stale generation was accepted")
	}
	if picker.acceptCatalog(fileCatalogLoadedMsg{cwd: "/other", generation: 2, paths: []string{"other"}}) {
		t.Fatal("stale cwd was accepted")
	}
}

func writePickerTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
