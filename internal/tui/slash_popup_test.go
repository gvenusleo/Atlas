package tui

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

func TestSlashPopupFiltersSkillsByPrefix(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "hunt", Description: "Find root causes"},
		{Name: "think", Description: "Plan work"},
	})
	syncSlashPopupValue(&popup, "/th")

	command, ok := popup.selectedCommand()
	if !ok || command.name != "think" {
		t.Fatalf("selected command = %+v, %t", command, ok)
	}
	rendered := ansi.Strip(popup.render(40, maxSlashPopupRows, nil, false))
	if !strings.Contains(rendered, "/think") || strings.Contains(rendered, "/hunt") || strings.Contains(rendered, "/model") {
		t.Fatalf("filtered popup = %q", rendered)
	}
}

func TestSlashPopupRanksExactPrefixSubstringAndFuzzyMatches(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "command-it", Description: "Fuzzy match"},
		{Name: "git-commit", Description: "Substring match"},
		{Name: "commit-helper", Description: "Prefix match"},
		{Name: "commit", Description: "Exact match"},
	})
	syncSlashPopupValue(&popup, "/CoMmIt")

	var names []string
	for _, command := range popup.matches {
		names = append(names, command.name)
	}
	want := []string{"commit", "commit-helper", "git-commit", "command-it"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("matched commands = %v, want %v", names, want)
	}
}

func TestSlashPopupMatchesOrderedCharactersOutsidePrefix(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "git-commit", Description: "Commit changes"},
		{Name: "find-skills", Description: "Discover skills"},
	})

	syncSlashPopupValue(&popup, "/gc")
	command, ok := popup.selectedCommand()
	if !ok || command.name != "git-commit" {
		t.Fatalf("selected command = %+v, %t", command, ok)
	}

	syncSlashPopupValue(&popup, "/fs")
	command, ok = popup.selectedCommand()
	if !ok || command.name != "find-skills" {
		t.Fatalf("selected command = %+v, %t", command, ok)
	}
}

func TestSlashPopupResetsSelectionWhenQueryChanges(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "hunt", Description: "Find root causes"},
		{Name: "think", Description: "Plan work"},
	})
	syncSlashPopupValue(&popup, "/")
	popup.move(2)
	syncSlashPopupValue(&popup, "/")
	if popup.selected != 2 {
		t.Fatalf("selection after unchanged query = %d, want 2", popup.selected)
	}

	syncSlashPopupValue(&popup, "/th")
	if popup.selected != 0 {
		t.Fatalf("selection after changed query = %d, want 0", popup.selected)
	}
}

func TestSlashPopupFiltersInvalidAndReservedSkills(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: modelCommandName, Description: "shadow built-in"},
		{Name: resumeCommandName, Description: "shadow built-in"},
		{Name: "compact", Description: "shadow built-in"},
		{Name: "quit", Description: "shadow built-in"},
		{Name: "browser:control", Description: "invalid command name"},
		{Name: "valid-skill", Description: "valid command name"},
	})
	syncSlashPopupValue(&popup, "/")

	rendered := ansi.Strip(popup.render(80, maxSlashPopupRows, nil, false))
	if strings.Count(rendered, "/model") != 1 || strings.Count(rendered, "/resume") != 1 || strings.Count(rendered, "/compact") != 1 || strings.Count(rendered, "/quit") != 1 || strings.Contains(rendered, "browser:control") || !strings.Contains(rendered, "/valid-skill") {
		t.Fatalf("popup catalog = %q", rendered)
	}
}

func TestSelectedSkillNamesMatchesSlashTokens(t *testing.T) {
	got := selectedSkillNames("/think review this with /hunt /think /resume /compact /quit and /invalid:name")
	want := []string{"think", "hunt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("selectedSkillNames() = %v, want %v", got, want)
	}
}

func TestSlashPopupReopensAfterDismissedDraftChanges(t *testing.T) {
	popup := newSlashPopup()
	syncSlashPopupValue(&popup, "/")
	popup.dismiss("/")
	syncSlashPopupValue(&popup, "")
	syncSlashPopupValue(&popup, "/")

	if !popup.active() {
		t.Fatal("popup did not reopen after the draft changed")
	}
}

func TestSlashPopupDescriptionStaysOnOneLine(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{{
		Name:        "think",
		Description: "Plan\n\x1b[31mwork\x1b[0m",
	}})
	syncSlashPopupValue(&popup, "/think")

	rendered := ansi.Strip(popup.render(40, maxSlashPopupRows, nil, false))
	if strings.Contains(rendered, "\n") || !strings.Contains(rendered, "Plan work") {
		t.Fatalf("popup description = %q", rendered)
	}
}

func TestSlashPopupAlignsDescriptionsAndHighlightsSelection(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "short", Description: "Short description"},
		{Name: "long-command", Description: "Long description"},
	})
	syncSlashPopupValue(&popup, "/")
	popup.move(5)

	rawLines := strings.Split(popup.render(80, 6, nil, false), "\n")
	lines := make([]string, len(rawLines))
	for index, line := range rawLines {
		lines[index] = ansi.Strip(line)
	}
	if len(lines) != 6 {
		t.Fatalf("popup lines = %q", lines)
	}
	descriptions := []string{"Choose a model", "Resume a saved session", "Compact earlier context", "Quit Atlas", "[Skill] Short description", "[Skill] Long description"}
	descriptionColumn := -1
	for index, description := range descriptions {
		column := ansi.StringWidth(lines[index][:strings.Index(lines[index], description)])
		if descriptionColumn < 0 {
			descriptionColumn = column
		} else if column != descriptionColumn {
			t.Fatalf("description columns = %d and %d: %q", descriptionColumn, column, lines)
		}
	}
	if rawLines[5] != lightTheme.highlight.Render(lines[5]) {
		t.Fatalf("selected row does not use one foreground style: %q", rawLines[5])
	}
	if !strings.Contains(rawLines[4], lightTheme.muted.Render("  [Skill] Short description")) {
		t.Fatalf("unselected description does not use subtle style: %q", rawLines[4])
	}
	for _, line := range lines[:4] {
		if strings.Contains(line, "[Skill]") {
			t.Fatalf("built-in command is labeled as a skill: %q", lines[:4])
		}
	}
	if !reflect.DeepEqual(lightTheme.highlight.GetBackground(), lightTheme.text.GetBackground()) {
		t.Fatal("selected slash style has a background color")
	}
}

func TestSlashPopupStylesInheritComposerBackground(t *testing.T) {
	popup := newSlashPopup()
	syncSlashPopupValue(&popup, "/")
	background := color.RGBA{R: 72, G: 78, B: 90, A: 255}

	for _, dark := range []bool{false, true} {
		theme := themeFor(dark)
		for name, style := range map[string]lipgloss.Style{
			"highlight": slashPopupStyle(theme.highlight, background),
			"text":      slashPopupStyle(theme.text, background),
			"muted":     slashPopupStyle(theme.muted, background),
		} {
			if !reflect.DeepEqual(style.GetBackground(), background) {
				t.Fatalf("dark=%t %s background = %#v, want %#v", dark, name, style.GetBackground(), background)
			}
		}

		rendered := popup.render(80, maxSlashPopupRows, background, dark)
		if !strings.Contains(rendered, slashPopupStyle(theme.highlight, background).Render("› /model    Choose a model and reasoning effort")) {
			t.Fatalf("dark=%t selected row does not inherit composer background: %q", dark, rendered)
		}
		if !strings.Contains(rendered, slashPopupStyle(theme.muted, background).Render("  Resume a saved session")) {
			t.Fatalf("dark=%t description does not inherit composer background: %q", dark, rendered)
		}
	}
}

func TestInlineSlashPopupShowsOnlySkills(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "hunt", Description: "Find root causes"},
		{Name: "think", Description: "Plan work"},
	})
	input := textarea.New()
	input.SetValue("review with /th please")
	input.MoveToEnd()
	input.SetCursorColumn(len("review with /th"))
	popup.sync(input)

	rendered := ansi.Strip(popup.render(80, maxSlashPopupRows, nil, false))
	if !strings.Contains(rendered, "/think") || strings.Contains(rendered, "/model") || strings.Contains(rendered, "/resume") || strings.Contains(rendered, "/compact") || strings.Contains(rendered, "/quit") {
		t.Fatalf("inline popup = %q", rendered)
	}
	if !popup.target.skillsOnly {
		t.Fatal("inline popup did not enter skills-only mode")
	}
}

func TestSlashPopupIgnoresEmbeddedSlash(t *testing.T) {
	for _, value := range []string{"https://example.com", "path/to/file", "word/"} {
		popup := newSlashPopup()
		syncSlashPopupValue(&popup, value)
		if popup.active() {
			t.Fatalf("popup opened for %q", value)
		}
	}
}

func TestReplaceSlashCompletionPreservesMultilineDraft(t *testing.T) {
	input := textarea.New()
	_ = input.Focus()
	input.SetValue("first line\nreview /th please")
	input.MoveToEnd()
	input.SetCursorColumn(len("review /th"))
	target, ok := currentSlashCompletionTarget(input)
	if !ok || !target.skillsOnly {
		t.Fatalf("target = %#v, %t", target, ok)
	}

	if !replaceSlashCompletion(&input, target, "think") {
		t.Fatal("replaceSlashCompletion() = false")
	}
	if got := input.Value(); got != "first line\nreview /think please" {
		t.Fatalf("input value = %q", got)
	}
	if got := input.Column(); got != len("review /think ") {
		t.Fatalf("cursor column = %d", got)
	}
}

func syncSlashPopupValue(popup *slashPopup, value string) {
	input := textarea.New()
	input.SetValue(value)
	input.MoveToEnd()
	popup.sync(input)
}

func TestResumeCommandSessionID(t *testing.T) {
	tests := []struct {
		input string
		id    string
		ok    bool
	}{
		{input: "/resume", ok: true},
		{input: "/resume work", id: "work", ok: true},
		{input: "/resume\nwork", id: "work", ok: true},
		{input: "/resumed"},
	}
	for _, test := range tests {
		id, ok := resumeCommandSessionID(test.input)
		if id != test.id || ok != test.ok {
			t.Fatalf("resumeCommandSessionID(%q) = %q, %t; want %q, %t", test.input, id, ok, test.id, test.ok)
		}
	}
}
