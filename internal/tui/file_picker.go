package tui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxFilePopupRows = 5
	maxIndexedFiles  = 50_000
)

type filePickerMode uint8

const (
	filePickerClosed filePickerMode = iota
	filePickerSearch
	filePickerBrowse
)

type fileMentionTarget struct {
	line  int
	start int
	end   int
	token string
	query string
}

type fileCatalogLoadedMsg struct {
	cwd        string
	generation uint64
	paths      []string
	truncated  bool
	err        error
}

// fileMentionPicker owns inline @ completion and the optional directory browser.
type fileMentionPicker struct {
	mode           filePickerMode
	cwd            string
	target         fileMentionTarget
	dismissedValue string
	dismissed      fileMentionTarget
	catalog        []string
	matches        []string
	selected       int
	loading        bool
	truncated      bool
	err            string
	generation     uint64
	scanCancel     context.CancelFunc
	browser        filepicker.Model
	browserRoot    string
	browserEntries []os.DirEntry
}

func (p fileMentionPicker) active() bool {
	return p.mode != filePickerClosed
}

func (p fileMentionPicker) browsing() bool {
	return p.mode == filePickerBrowse
}

func (p *fileMentionPicker) close() {
	if p.scanCancel != nil {
		p.scanCancel()
	}
	p.scanCancel = nil
	p.generation++
	p.mode = filePickerClosed
	p.catalog = nil
	p.matches = nil
	p.selected = 0
	p.loading = false
	p.truncated = false
	p.err = ""
	p.browserRoot = ""
	p.browserEntries = nil
}

func (p *fileMentionPicker) reset() {
	p.dismissedValue = ""
	p.dismissed = fileMentionTarget{}
	p.close()
}

func (p *fileMentionPicker) dismiss(value string) {
	p.dismissedValue = value
	p.dismissed = p.target
	p.close()
}

func (p *fileMentionPicker) sync(target fileMentionTarget, value, cwd string) (uint64, bool) {
	if p.dismissedValue != "" {
		if value == p.dismissedValue && target == p.dismissed {
			return 0, false
		}
		p.dismissedValue = ""
		p.dismissed = fileMentionTarget{}
	}

	cwd = cleanWorkingDirectory(cwd)
	if p.mode == filePickerSearch && p.cwd == cwd && p.target.line == target.line && p.target.start == target.start {
		queryChanged := p.target.query != target.query
		p.target = target
		if queryChanged {
			p.selected = 0
		}
		p.filter()
		return 0, false
	}

	p.close()
	p.mode = filePickerSearch
	p.cwd = cwd
	p.target = target
	p.loading = true
	p.generation++
	return p.generation, true
}

func (p *fileMentionPicker) setScanCancel(cancel context.CancelFunc) {
	p.scanCancel = cancel
}

func (p *fileMentionPicker) acceptCatalog(msg fileCatalogLoadedMsg) bool {
	if !p.active() || msg.generation != p.generation || filepath.Clean(msg.cwd) != filepath.Clean(p.cwd) {
		return false
	}
	p.scanCancel = nil
	p.loading = false
	p.catalog = msg.paths
	p.truncated = msg.truncated
	p.err = ""
	if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
		p.err = "Unable to search files"
	}
	p.filter()
	return true
}

func (p *fileMentionPicker) filter() {
	p.matches = nil
	if p.mode != filePickerSearch || p.target.query == "" || len(p.catalog) == 0 {
		p.selected = 0
		return
	}

	targets := make([]string, len(p.catalog))
	for i, path := range p.catalog {
		targets[i] = strings.ToLower(path)
	}
	for _, rank := range list.DefaultFilter(strings.ToLower(p.target.query), targets) {
		p.matches = append(p.matches, p.catalog[rank.Index])
	}
	p.selected = min(p.selected, max(len(p.matches)-1, 0))
}

func (p *fileMentionPicker) move(delta int) {
	if p.mode != filePickerSearch || len(p.matches) == 0 {
		return
	}
	p.selected = min(max(p.selected+delta, 0), len(p.matches)-1)
}

func (p fileMentionPicker) selectedPath() (string, bool) {
	if p.mode != filePickerSearch || p.target.query == "" || p.selected < 0 || p.selected >= len(p.matches) {
		return "", false
	}
	return p.matches[p.selected], true
}

func (p *fileMentionPicker) openBrowser() tea.Cmd {
	root := cleanWorkingDirectory(p.cwd)
	fp := filepicker.New()
	fp.CurrentDirectory = root
	fp.AutoHeight = false
	fp.SetHeight(maxFilePopupRows - 1)
	fp.ShowPermissions = false
	fp.ShowSize = false
	fp.ShowHidden = false
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.Cursor = userStyle.Render("›")
	fp.KeyMap.Back.SetKeys("left", "backspace")
	fp.Styles.Cursor = messageStyle
	fp.Styles.Selected = userStyle
	fp.Styles.Directory = subtleStyle
	fp.Styles.File = messageStyle
	fp.Styles.Symlink = subtleStyle
	fp.Styles.EmptyDirectory = subtleStyle.SetString("  No files")
	p.browser = fp
	p.browserRoot = root
	p.browserEntries = browserDirectoryEntries(root)
	p.mode = filePickerBrowse
	return p.browser.Init()
}

func (p *fileMentionPicker) returnToSearch() {
	p.mode = filePickerSearch
}

func (p *fileMentionPicker) updateBrowser(msg tea.Msg) (tea.Cmd, string, bool) {
	if !p.browsing() {
		return nil, "", false
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		if (key.String() == "left" || key.String() == "backspace") && samePath(p.browser.CurrentDirectory, p.browserRoot) {
			return nil, "", false
		}
		if (key.String() == "enter" || key.String() == "right" || key.String() == "l") && !browserCanOpen(p.browserRoot, p.browser.HighlightedPath()) {
			return nil, "", false
		}
	}

	previousDirectory := p.browser.CurrentDirectory
	var cmd tea.Cmd
	p.browser, cmd = p.browser.Update(msg)
	if !samePath(previousDirectory, p.browser.CurrentDirectory) {
		p.browserEntries = browserDirectoryEntries(p.browser.CurrentDirectory)
	}
	if selected, path := p.browser.DidSelectFile(msg); selected && pathWithinRoot(p.browserRoot, path) {
		relative, err := filepath.Rel(p.browserRoot, path)
		if err == nil {
			return cmd, filepath.ToSlash(relative), true
		}
	}
	return cmd, "", false
}

func (p fileMentionPicker) render(width, maxRows int, background color.Color) string {
	if !p.active() || maxRows <= 0 {
		return ""
	}
	if p.browsing() {
		return p.renderBrowser(width, maxRows, background)
	}

	contentWidth := max(width-2, 1)
	if p.target.query == "" {
		return userStyle.Render(ansi.Truncate("› Browse files…", width, "…"))
	}
	if len(p.matches) == 0 {
		label := "No matching files"
		if p.loading {
			label = "Loading files…"
		} else if p.err != "" {
			label = p.err
		}
		return "  " + subtleStyle.Render(ansi.Truncate(label, contentWidth, "…"))
	}

	rows := min(maxRows, len(p.matches))
	start := pickerWindowStart(len(p.matches), p.selected, rows)
	end := min(start+rows, len(p.matches))
	lines := make([]string, 0, rows)
	for i := start; i < end; i++ {
		path := ansi.Truncate(p.matches[i], contentWidth, "…")
		if i == p.selected {
			lines = append(lines, userStyle.Render("› "+path))
		} else {
			lines = append(lines, "  "+messageStyle.Render(path))
		}
	}
	return strings.Join(lines, "\n")
}

func (p fileMentionPicker) renderBrowser(width, maxRows int, background color.Color) string {
	relative, err := filepath.Rel(p.browserRoot, p.browser.CurrentDirectory)
	if err != nil || relative == "" {
		relative = "."
	}
	headerStyle := fileBrowserStyle(messageStyle.Bold(true), background)
	header := "  " + headerStyle.Render(ansi.Truncate("Browse · "+filepath.ToSlash(relative), max(width-2, 1), "…"))
	if maxRows == 1 {
		return header
	}
	if maxRows == 2 {
		return header + "\n"
	}

	fp := p.browser
	fp.Styles.Selected = fileBrowserStyle(fp.Styles.Selected, background)
	fp.Styles.Directory = fileBrowserStyle(fp.Styles.Directory, background)
	fp.Styles.File = fileBrowserStyle(fp.Styles.File, background)
	fp.Styles.Symlink = fileBrowserStyle(fp.Styles.Symlink, background)
	fp.Styles.EmptyDirectory = fileBrowserStyle(fp.Styles.EmptyDirectory, background)
	itemRows := maxRows - 2
	selected := 0
	selectedName := filepath.Base(p.browser.HighlightedPath())
	for i, entry := range p.browserEntries {
		if entry.Name() == selectedName {
			selected = i
			break
		}
	}
	start := pickerWindowStart(len(p.browserEntries), selected, itemRows)
	end := min(start+itemRows, len(p.browserEntries))
	items := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, renderBrowserEntry(fp, p.browser.CurrentDirectory, p.browserEntries[i], i == selected))
	}
	if len(items) == 0 {
		items = append(items, fp.Styles.EmptyDirectory.String())
	}
	for i := range items {
		items[i] = ansi.Truncate(items[i], width, "…")
	}
	return strings.Join(append([]string{header, ""}, items...), "\n")
}

// browserDirectoryEntries mirrors the file picker ordering without its internal viewport.
func browserDirectoryEntries(directory string) []os.DirEntry {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	visible := entries[:0]
	for _, entry := range entries {
		hidden, _ := filepicker.IsHidden(entry.Name())
		if !hidden {
			visible = append(visible, entry)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		if visible[i].IsDir() == visible[j].IsDir() {
			return visible[i].Name() < visible[j].Name()
		}
		return visible[i].IsDir()
	})
	return visible
}

func renderBrowserEntry(fp filepicker.Model, directory string, entry os.DirEntry, selected bool) string {
	name := entry.Name()
	style := fp.Styles.File
	if entry.IsDir() {
		style = fp.Styles.Directory
	} else if entry.Type()&os.ModeSymlink != 0 {
		style = fp.Styles.Symlink
		if target, err := filepath.EvalSymlinks(filepath.Join(directory, name)); err == nil {
			name += " → " + target
		}
	}
	if selected {
		return fp.Styles.Cursor.Render(fp.Cursor) + fp.Styles.Selected.Render(" "+name)
	}
	return fp.Styles.Cursor.Render(" ") + " " + style.Render(name)
}

func fileBrowserStyle(style lipgloss.Style, background color.Color) lipgloss.Style {
	if background == nil {
		return style
	}
	return style.Background(background)
}

// currentFileMentionTarget locates the whitespace-delimited @ token at the cursor.
func currentFileMentionTarget(input textarea.Model) (fileMentionTarget, bool) {
	lines := strings.Split(input.Value(), "\n")
	lineIndex := input.Line()
	if lineIndex < 0 || lineIndex >= len(lines) {
		return fileMentionTarget{}, false
	}

	line := []rune(lines[lineIndex])
	column := min(max(input.Column(), 0), len(line))
	if column > 0 && unicode.IsSpace(line[column-1]) {
		return fileMentionTarget{}, false
	}
	start := column
	for start > 0 && !unicode.IsSpace(line[start-1]) {
		start--
	}
	end := column
	for end < len(line) && !unicode.IsSpace(line[end]) {
		end++
	}
	if start >= end || line[start] != '@' {
		return fileMentionTarget{}, false
	}
	token := string(line[start:end])
	return fileMentionTarget{
		line:  lineIndex,
		start: start,
		end:   end,
		token: token,
		query: strings.TrimPrefix(token, "@"),
	}, true
}

// replaceFileMention replaces only the active token while preserving surrounding draft text.
func replaceFileMention(input *textarea.Model, target fileMentionTarget, path string) bool {
	if input.Line() != target.line {
		return false
	}
	lines := strings.Split(input.Value(), "\n")
	if target.line < 0 || target.line >= len(lines) {
		return false
	}
	line := []rune(lines[target.line])
	if target.start < 0 || target.end > len(line) || target.start >= target.end || string(line[target.start:target.end]) != target.token {
		return false
	}

	inserted := filepath.ToSlash(path)
	if strings.IndexFunc(inserted, unicode.IsSpace) >= 0 && !strings.ContainsRune(inserted, '"') {
		inserted = `"` + inserted + `"`
	}
	suffix := line[target.end:]
	input.SetCursorColumn(target.end)
	for range target.end - target.start {
		updated, _ := input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		*input = updated
	}
	input.InsertString(inserted)

	if len(suffix) == 0 || suffix[0] == '\n' || suffix[0] == '\r' {
		input.InsertRune(' ')
		return true
	}
	if unicode.IsSpace(suffix[0]) {
		input.SetCursorColumn(input.Column() + 1)
		return true
	}
	input.InsertRune(' ')
	return true
}

func cleanWorkingDirectory(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	absolute, err := filepath.Abs(cwd)
	if err != nil {
		return filepath.Clean(cwd)
	}
	return filepath.Clean(absolute)
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func pathWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func browserCanOpen(root, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return true
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	return err == nil && pathWithinRoot(resolvedRoot, resolvedPath)
}

func loadWorkspaceFiles(ctx context.Context, cwd string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		paths, truncated, err := workspaceFiles(ctx, cwd, maxIndexedFiles)
		return fileCatalogLoadedMsg{cwd: cwd, generation: generation, paths: paths, truncated: truncated, err: err}
	}
}

func workspaceFiles(ctx context.Context, cwd string, limit int) ([]string, bool, error) {
	root := cleanWorkingDirectory(cwd)
	if paths, truncated, ok := gitWorkspaceFiles(ctx, root, limit); ok {
		return paths, truncated, nil
	}
	return walkWorkspaceFiles(ctx, root, limit)
}

// gitWorkspaceFiles returns tracked and unignored untracked files when cwd is in a Git worktree.
func gitWorkspaceFiles(ctx context.Context, root string, limit int) ([]string, bool, bool) {
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, false
	}
	if err := cmd.Start(); err != nil {
		return nil, false, false
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(splitNUL)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	paths := make([]string, 0, min(limit, 1024))
	truncated := false
	for scanner.Scan() {
		if len(paths) >= limit {
			truncated = true
			cancel()
			break
		}
		path := filepath.ToSlash(scanner.Text())
		if path != "" {
			paths = append(paths, path)
		}
	}
	waitErr := cmd.Wait()
	if truncated {
		sort.Strings(paths)
		return paths, true, true
	}
	if scanner.Err() != nil || waitErr != nil {
		return nil, false, false
	}
	sort.Strings(paths)
	return paths, false, true
}

// walkWorkspaceFiles provides a Git-independent fallback without descending into VCS metadata.
func walkWorkspaceFiles(ctx context.Context, root string, limit int) ([]string, bool, error) {
	paths := make([]string, 0, min(limit, 1024))
	truncated := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if samePath(path, root) {
				return walkErr
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if !samePath(path, root) && isVCSDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(paths) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(paths)
	return paths, truncated, err
}

func isVCSDirectory(name string) bool {
	return name == ".git" || name == ".hg" || name == ".svn"
}

func splitNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexByte(data, 0); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (m *Model) syncFileMention() tea.Cmd {
	if m.filePicker.browsing() {
		return nil
	}
	target, ok := currentFileMentionTarget(m.input)
	if !ok {
		if m.filePicker.dismissedValue != "" && m.input.Value() != m.filePicker.dismissedValue {
			m.filePicker.dismissedValue = ""
			m.filePicker.dismissed = fileMentionTarget{}
		}
		if m.filePicker.active() {
			m.filePicker.close()
		}
		return nil
	}
	generation, started := m.filePicker.sync(target, m.input.Value(), m.cwd)
	if !started {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.filePicker.setScanCancel(cancel)
	return loadWorkspaceFiles(ctx, m.filePicker.cwd, generation)
}

// handleFileMentionKey routes selection keys while the inline search is visible.
func (m *Model) handleFileMentionKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.filePicker.mode != filePickerSearch {
		return false, nil
	}
	switch msg.String() {
	case "up":
		m.filePicker.move(-1)
		m.rebuild()
		return true, nil
	case "down":
		m.filePicker.move(1)
		m.rebuild()
		return true, nil
	case "esc":
		m.filePicker.dismiss(m.input.Value())
		m.rebuild()
		return true, nil
	case "tab", "enter":
		if m.filePicker.target.query == "" {
			cmd := m.filePicker.openBrowser()
			m.input.Blur()
			m.rebuild()
			return true, cmd
		}
		if path, ok := m.filePicker.selectedPath(); ok {
			m.insertSelectedFile(path)
			m.rebuild()
			return true, nil
		}
		if msg.String() == "tab" {
			return true, nil
		}
		m.filePicker.dismiss(m.input.Value())
		return false, nil
	}
	return false, nil
}

// handleFileBrowserKey routes modal navigation and restores inline search on escape.
func (m Model) handleFileBrowserKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, nil
	case "esc":
		m.filePicker.returnToSearch()
		m.input.Focus()
		m.rebuild()
		return m, nil
	}
	cmd, path, selected := m.filePicker.updateBrowser(msg)
	if selected {
		m.input.Focus()
		m.insertSelectedFile(path)
	}
	m.rebuild()
	return m, cmd
}

func (m *Model) insertSelectedFile(path string) {
	target := m.filePicker.target
	if replaceFileMention(&m.input, target, path) {
		m.filePicker.reset()
		m.slashPopup.sync(m.input.Value())
	}
}
