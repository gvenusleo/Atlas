package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/session"
)

func TestSessionPickerFiltersAndExcludesCurrentSession(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	var picker sessionPicker
	picker.open("/work", "current", now)
	picker.appendPage(session.ListPage{Sessions: []session.Session{
		{ID: "current", Title: "Current session", CWD: "/work", UpdatedAt: now},
		{ID: "older", Title: "Investigate pagination", CWD: "/work", UpdatedAt: now.Add(-time.Hour)},
		{ID: "recent", Title: "Resume project work", CWD: "/work", UpdatedAt: now.Add(-time.Minute)},
	}})

	if len(picker.matches) != 2 || picker.matches[0].ID != "older" || picker.matches[1].ID != "recent" {
		t.Fatalf("initial matches = %#v", picker.matches)
	}
	picker.appendQuery("rpw")
	selected, ok := picker.selectedSession()
	if !ok || selected.ID != "recent" {
		t.Fatalf("fuzzy selected session = %#v, %t", selected, ok)
	}
	picker.appendQuery(" ")
	if picker.query != "rpw " {
		t.Fatalf("query with space = %q", picker.query)
	}
}

func TestSessionPickerRenderUsesForegroundSelectionAndAllScopeMetadata(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	var picker sessionPicker
	picker.open("/work", "", now)
	picker.scope = sessionPickerAll
	picker.appendPage(session.ListPage{Sessions: []session.Session{{
		ID:        "session-1",
		Title:     "Continue Atlas TUI",
		CWD:       "/other/project",
		UpdatedAt: now.Add(-4 * time.Minute),
	}}})

	rendered := picker.render(80, 20, false)
	plain := ansi.Strip(rendered.content)
	lines := strings.Split(plain, "\n")
	if len(lines) < 6 || lines[0] != "  Resume a previous session" || lines[1] != "" {
		t.Fatalf("picker header = %q", plain)
	}
	if !strings.Contains(plain, "[All]") || !strings.Contains(plain, "/other/project") || !strings.Contains(plain, "4m ago") {
		t.Fatalf("picker content = %q", plain)
	}
	if !strings.Contains(plain, "Esc to cancel") {
		t.Fatalf("picker cancel hint = %q", plain)
	}
	if !strings.Contains(rendered.content, lightTheme.highlight.Render("› Continue Atlas TUI")) {
		t.Fatalf("selected row is not foreground highlighted: %q", rendered.content)
	}
	if !reflect.DeepEqual(lightTheme.highlight.GetBackground(), lightTheme.text.GetBackground()) {
		t.Fatal("session selection style has a background color")
	}
}

func TestSessionPickerLoadingAndConfirmationShowCancelHint(t *testing.T) {
	var picker sessionPicker
	picker.openDirect("/work", "current", time.Now())
	if plain := ansi.Strip(picker.render(60, 20, false).content); !strings.Contains(plain, "Esc to cancel") {
		t.Fatalf("loading cancel hint = %q", plain)
	}

	picker.confirm(resumedSession{info: session.Session{ID: "target", Title: "Target", CWD: "/target"}})
	if plain := ansi.Strip(picker.render(60, 20, false).content); !strings.Contains(plain, "Esc to cancel") {
		t.Fatalf("confirmation cancel hint = %q", plain)
	}
}

func TestSessionPickerLoadsMoreNearEndAndInvalidatesClosedRequests(t *testing.T) {
	var picker sessionPicker
	generation := picker.open("/work", "", time.Now())
	picker.appendPage(session.ListPage{
		Sessions:   []session.Session{{ID: "one"}, {ID: "two"}, {ID: "three"}},
		NextCursor: "next",
	})
	if !picker.shouldLoadMore() {
		t.Fatal("picker did not request the next page near the list end")
	}
	picker.close()
	if picker.generation == generation || picker.active() {
		t.Fatalf("closed picker state = generation:%d active:%t", picker.generation, picker.active())
	}
}

func TestSessionPickerScopeChangesOnlyInRequestedDirection(t *testing.T) {
	var picker sessionPicker
	generation := picker.open("/work", "", time.Now())
	if got, changed := picker.setScope(sessionPickerCWD); changed || got != generation {
		t.Fatalf("unchanged scope = generation:%d changed:%t", got, changed)
	}
	if got, changed := picker.setScope(sessionPickerAll); !changed || got == generation || picker.scope != sessionPickerAll {
		t.Fatalf("all scope = generation:%d changed:%t scope:%d", got, changed, picker.scope)
	}
}

func TestResumeSearchLoadsOlderPagesOnlyWithoutMatches(t *testing.T) {
	var picker sessionPicker
	picker.open("/work", "", time.Now())
	picker.appendPage(session.ListPage{
		Sessions:   []session.Session{{ID: "one", Title: "Matching title"}},
		NextCursor: "next",
	})
	m := Model{resumePicker: picker}
	m.resumePicker.setQuery("matching")
	if cmd := m.continueResumeSearchIfNeeded(); cmd != nil {
		t.Fatal("search with a loaded match requested another page")
	}
	m.resumePicker.setQuery("absent")
	if cmd := m.continueResumeSearchIfNeeded(); cmd == nil || !m.resumePicker.pageLoading {
		t.Fatalf("search without matches did not continue: cmd=%v loading=%t", cmd, m.resumePicker.pageLoading)
	}
}

func TestSessionPickerConfirmationShowsDirectoryChange(t *testing.T) {
	var picker sessionPicker
	picker.open("/current", "", time.Now())
	picker.confirm(resumedSession{info: session.Session{ID: "target", Title: "Target", CWD: "/target"}})

	plain := ansi.Strip(picker.render(60, 20, false).content)
	if !strings.Contains(plain, "/current") || !strings.Contains(plain, "→ /target") || !strings.Contains(plain, "Resume and switch directory") {
		t.Fatalf("confirmation = %q", plain)
	}
}

func TestFormatSessionAge(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		updated time.Time
		want    string
	}{
		{updated: now.Add(-30 * time.Second), want: "just now"},
		{updated: now.Add(-4 * time.Minute), want: "4m ago"},
		{updated: now.Add(-3 * time.Hour), want: "3h ago"},
		{updated: now.Add(-2 * 24 * time.Hour), want: "2d ago"},
	}
	for _, test := range tests {
		if got := formatSessionAge(test.updated, now); got != test.want {
			t.Fatalf("formatSessionAge() = %q, want %q", got, test.want)
		}
	}
}
