package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
)

const resumePageSize = 25

type sessionPickerStage uint8

const (
	sessionPickerClosed sessionPickerStage = iota
	sessionPickerList
	sessionPickerLoading
	sessionPickerConfirm
)

type sessionPickerScope uint8

const (
	sessionPickerCWD sessionPickerScope = iota
	sessionPickerAll
)

type resumedSession struct {
	info          session.Session
	messages      []model.Message
	contextTokens int
}

type sessionPageLoadedMsg struct {
	generation uint64
	page       session.ListPage
	err        error
}

type resumedSessionLoadedMsg struct {
	generation uint64
	session    resumedSession
	err        error
}

// sessionPicker owns the modal list, loading, and directory-confirmation state.
type sessionPicker struct {
	stage            sessionPickerStage
	scope            sessionPickerScope
	cwd              string
	currentSessionID string
	sessions         []session.Session
	matches          []session.Session
	selected         int
	query            string
	nextCursor       string
	pageLoading      bool
	generation       uint64
	referenceTime    time.Time
	direct           bool
	pending          *resumedSession
	err              string
}

func (p *sessionPicker) open(cwd, currentSessionID string, now time.Time) uint64 {
	p.generation++
	p.stage = sessionPickerList
	p.scope = sessionPickerCWD
	p.cwd = cwd
	p.currentSessionID = currentSessionID
	p.sessions = nil
	p.matches = nil
	p.selected = 0
	p.query = ""
	p.nextCursor = ""
	p.pageLoading = true
	p.referenceTime = now
	p.direct = false
	p.pending = nil
	p.err = ""
	return p.generation
}

func (p *sessionPicker) openDirect(cwd, currentSessionID string, now time.Time) uint64 {
	generation := p.open(cwd, currentSessionID, now)
	p.stage = sessionPickerLoading
	p.pageLoading = false
	p.direct = true
	return generation
}

func (p *sessionPicker) active() bool {
	return p.stage != sessionPickerClosed
}

func (p *sessionPicker) close() {
	p.generation++
	p.stage = sessionPickerClosed
	p.pageLoading = false
	p.pending = nil
	p.err = ""
}

func (p *sessionPicker) setScope(scope sessionPickerScope) (uint64, bool) {
	if p.scope == scope {
		return p.generation, false
	}
	p.scope = scope
	p.generation++
	p.stage = sessionPickerList
	p.sessions = nil
	p.matches = nil
	p.selected = 0
	p.nextCursor = ""
	p.pageLoading = true
	p.pending = nil
	p.err = ""
	return p.generation, true
}

func (p *sessionPicker) appendPage(page session.ListPage) {
	p.pageLoading = false
	p.nextCursor = page.NextCursor
	for _, candidate := range page.Sessions {
		if candidate.ID == p.currentSessionID {
			continue
		}
		p.sessions = append(p.sessions, candidate)
	}
	p.filter()
}

func (p *sessionPicker) filter() {
	if p.query == "" {
		p.matches = append(p.matches[:0], p.sessions...)
	} else {
		targets := make([]string, len(p.sessions))
		for i, candidate := range p.sessions {
			targets[i] = strings.ToLower(strings.Join([]string{candidate.Title, candidate.ID, candidate.CWD}, " "))
		}
		p.matches = p.matches[:0]
		for _, rank := range list.DefaultFilter(strings.ToLower(p.query), targets) {
			p.matches = append(p.matches, p.sessions[rank.Index])
		}
	}
	p.selected = min(p.selected, max(len(p.matches)-1, 0))
}

func (p *sessionPicker) setQuery(query string) {
	if p.query == query {
		return
	}
	p.query = query
	p.selected = 0
	p.err = ""
	p.filter()
}

func (p *sessionPicker) appendQuery(text string) {
	text = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
	if text == "" {
		return
	}
	p.setQuery(p.query + text)
}

func (p *sessionPicker) deleteQueryRune() {
	runes := []rune(p.query)
	if len(runes) == 0 {
		return
	}
	p.setQuery(string(runes[:len(runes)-1]))
}

func (p *sessionPicker) move(delta int) {
	if len(p.matches) == 0 {
		return
	}
	p.selected = min(max(p.selected+delta, 0), len(p.matches)-1)
}

func (p *sessionPicker) selectFirst() {
	if len(p.matches) > 0 {
		p.selected = 0
	}
}

func (p *sessionPicker) selectLast() {
	if len(p.matches) > 0 {
		p.selected = len(p.matches) - 1
	}
}

func (p sessionPicker) selectedSession() (session.Session, bool) {
	if p.stage != sessionPickerList || p.selected < 0 || p.selected >= len(p.matches) {
		return session.Session{}, false
	}
	return p.matches[p.selected], true
}

func (p *sessionPicker) startSessionLoad() {
	p.stage = sessionPickerLoading
	p.pending = nil
	p.err = ""
}

func (p *sessionPicker) confirm(session resumedSession) {
	p.stage = sessionPickerConfirm
	p.pending = &session
	p.err = ""
}

func (p *sessionPicker) failLoad(err error) {
	p.pending = nil
	p.err = err.Error()
	if p.direct {
		p.stage = sessionPickerClosed
		return
	}
	p.stage = sessionPickerList
}

func (p sessionPicker) shouldLoadMore() bool {
	if p.stage != sessionPickerList || p.pageLoading || p.nextCursor == "" {
		return false
	}
	return len(p.matches) == 0 || len(p.matches)-p.selected <= 3
}

func (p *sessionPicker) markPageLoading() {
	p.pageLoading = true
}

func (p sessionPicker) sameCWD(candidate string) bool {
	return filepath.Clean(candidate) == filepath.Clean(p.cwd)
}

type sessionPickerRender struct {
	content    string
	cursorX    int
	cursorY    int
	showCursor bool
}

// render draws the list, loading, or cross-directory confirmation stage.
func (p sessionPicker) render(width, height int, hasDarkBackground bool) sessionPickerRender {
	theme := themeFor(hasDarkBackground)
	width = max(width, 1)
	contentWidth := max(width-2, 1)
	title := "  " + theme.text.Bold(true).Render(ansi.Truncate("Resume a previous session", contentWidth, "…"))
	cancelHint := "  " + theme.muted.Render("Esc to cancel")

	switch p.stage {
	case sessionPickerLoading:
		label := "Loading session…"
		return sessionPickerRender{content: strings.Join([]string{title, "", "  " + theme.muted.Render(label), "", cancelHint}, "\n")}
	case sessionPickerConfirm:
		return p.renderConfirmation(title, cancelHint, width, theme)
	}

	searchPrefix := "  Search: "
	searchValue := p.query
	if searchValue == "" {
		searchValue = theme.muted.Render("Type to search")
	}
	search := theme.text.Render(searchPrefix) + searchValue
	filter := p.renderScope(theme)
	gap := max(width-ansi.StringWidth(search)-ansi.StringWidth(filter)-2, 1)
	toolbar := ansi.Truncate(search+strings.Repeat(" ", gap)+filter, width, "…")
	lines := []string{title, "", toolbar, ""}

	footerRows := 2
	if p.err != "" || p.pageLoading {
		footerRows++
	}
	visibleItems := max((height-len(lines)-footerRows)/3, 1)
	start := pickerWindowStart(len(p.matches), p.selected, visibleItems)
	end := min(start+visibleItems, len(p.matches))
	for index := start; index < end; index++ {
		candidate := p.matches[index]
		label := candidate.Title
		if label == "" {
			label = candidate.ID
		}
		prefix := "  "
		style := theme.text
		if index == p.selected {
			prefix = "› "
			style = theme.highlight
		}
		lines = append(lines, style.Render(prefix+ansi.Truncate(singleLineDisplayText(label), max(width-2, 1), "…")))
		metadata := sessionPickerMetadata(candidate, p.scope == sessionPickerAll, contentWidth, p.referenceTime)
		lines = append(lines, "  "+theme.muted.Render(metadata))
		if index+1 < end {
			lines = append(lines, "")
		}
	}

	if len(p.matches) == 0 && !p.pageLoading {
		empty := "No saved sessions in this directory."
		if p.scope == sessionPickerAll {
			empty = "No saved sessions."
		}
		if p.query != "" {
			empty = "No matching sessions."
		}
		lines = append(lines, "  "+theme.muted.Render(empty))
	}
	if p.err != "" {
		lines = append(lines, "  "+theme.error.Render(ansi.Truncate(p.err, contentWidth, "…")))
	} else if p.pageLoading {
		lines = append(lines, "  "+theme.muted.Render("Loading sessions…"))
	}
	lines = append(lines, "", cancelHint)

	return sessionPickerRender{
		content:    strings.Join(lines, "\n"),
		cursorX:    min(ansi.StringWidth(searchPrefix)+ansi.StringWidth(p.query), max(width-1, 0)),
		cursorY:    2,
		showCursor: true,
	}
}

func (p sessionPicker) renderScope(theme tuiTheme) string {
	cwd := theme.muted.Render(" Cwd ")
	all := theme.muted.Render(" All ")
	if p.scope == sessionPickerCWD {
		cwd = theme.highlight.Render("[Cwd]")
	} else {
		all = theme.highlight.Render("[All]")
	}
	return theme.muted.Render("Filter: ") + cwd + " " + all
}

func (p sessionPicker) renderConfirmation(title, cancelHint string, width int, theme tuiTheme) sessionPickerRender {
	if p.pending == nil {
		return sessionPickerRender{content: strings.Join([]string{title, "", "  " + theme.error.Render("Session is unavailable."), "", cancelHint}, "\n")}
	}
	contentWidth := max(width-2, 1)
	label := p.pending.info.Title
	if label == "" {
		label = p.pending.info.ID
	}
	lines := []string{
		title,
		"",
		"  " + theme.text.Bold(true).Render(ansi.Truncate(singleLineDisplayText(label), contentWidth, "…")),
		"  " + theme.muted.Render(ansi.Truncate(p.pending.info.ID, contentWidth, "…")),
		"",
		"  " + theme.muted.Render("Working directory"),
		"  " + theme.text.Render(ansi.Truncate(p.cwd, contentWidth, "…")),
		"  " + theme.muted.Render("→ ") + theme.text.Render(ansi.Truncate(p.pending.info.CWD, max(contentWidth-2, 1), "…")),
		"",
		theme.highlight.Render("› Resume and switch directory"),
		"",
		cancelHint,
	}
	if p.err != "" {
		lines = append(lines, "", "  "+theme.error.Render(ansi.Truncate(p.err, contentWidth, "…")))
	}
	return sessionPickerRender{content: strings.Join(lines, "\n")}
}

func sessionPickerMetadata(candidate session.Session, showCWD bool, width int, now time.Time) string {
	parts := []string{formatSessionAge(candidate.UpdatedAt, now), candidate.ID}
	if showCWD {
		parts = append(parts, candidate.CWD)
	}
	return ansi.Truncate(strings.Join(parts, " · "), max(width, 1), "…")
}

func formatSessionAge(updatedAt, now time.Time) string {
	if updatedAt.IsZero() {
		return "unknown"
	}
	if now.IsZero() {
		now = time.Now()
	}
	age := max(now.Sub(updatedAt), 0)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age/time.Hour))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age/(24*time.Hour)))
	default:
		return updatedAt.Local().Format("2006-01-02")
	}
}

// loadSessionPage retrieves one picker page without blocking Bubble Tea's update loop.
func loadSessionPage(ctx context.Context, rt *runtime.Runtime, cwd string, scope sessionPickerScope, cursor string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		var page session.ListPage
		var err error
		if scope == sessionPickerCWD {
			page, err = rt.ListSessionsForCWDPage(ctx, cwd, cursor, resumePageSize)
		} else {
			page, err = rt.ListSessionsPage(ctx, cursor, resumePageSize)
		}
		return sessionPageLoadedMsg{generation: generation, page: page, err: err}
	}
}
