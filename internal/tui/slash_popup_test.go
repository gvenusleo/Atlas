package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

func TestSlashPopupFiltersSkillsByPrefix(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: "hunt", Description: "Find root causes"},
		{Name: "think", Description: "Plan work"},
	})
	popup.sync("/th")

	command, ok := popup.selectedCommand()
	if !ok || command.name != "think" {
		t.Fatalf("selected command = %+v, %t", command, ok)
	}
	rendered := ansi.Strip(popup.render(40, maxSlashPopupRows))
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
	popup.sync("/CoMmIt")

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

	popup.sync("/gc")
	command, ok := popup.selectedCommand()
	if !ok || command.name != "git-commit" {
		t.Fatalf("selected command = %+v, %t", command, ok)
	}

	popup.sync("/fs")
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
	popup.sync("/")
	popup.move(2)
	popup.sync("/")
	if popup.selected != 2 {
		t.Fatalf("selection after unchanged query = %d, want 2", popup.selected)
	}

	popup.sync("/th")
	if popup.selected != 0 {
		t.Fatalf("selection after changed query = %d, want 0", popup.selected)
	}
}

func TestSlashPopupFiltersInvalidAndReservedSkills(t *testing.T) {
	popup := newSlashPopup()
	popup.setSkills([]runtime.SkillSummary{
		{Name: modelCommandName, Description: "shadow built-in"},
		{Name: "browser:control", Description: "invalid command name"},
		{Name: "valid-skill", Description: "valid command name"},
	})
	popup.sync("/")

	rendered := ansi.Strip(popup.render(80, maxSlashPopupRows))
	if strings.Count(rendered, "/model") != 1 || strings.Contains(rendered, "browser:control") || !strings.Contains(rendered, "/valid-skill") {
		t.Fatalf("popup catalog = %q", rendered)
	}
}

func TestSelectedSkillNamesMatchesSlashTokens(t *testing.T) {
	got := selectedSkillNames("/think review this with /hunt /think and /invalid:name")
	want := []string{"think", "hunt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("selectedSkillNames() = %v, want %v", got, want)
	}
}

func TestSlashPopupReopensAfterDismissedDraftChanges(t *testing.T) {
	popup := newSlashPopup()
	popup.sync("/")
	popup.dismiss("/")
	popup.sync("")
	popup.sync("/")

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
	popup.sync("/think")

	rendered := ansi.Strip(popup.render(40, maxSlashPopupRows))
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
	popup.sync("/")
	popup.move(1)

	rawLines := strings.Split(popup.render(80, maxSlashPopupRows), "\n")
	lines := make([]string, len(rawLines))
	for index, line := range rawLines {
		lines[index] = ansi.Strip(line)
	}
	if len(lines) != 3 {
		t.Fatalf("popup lines = %q", lines)
	}
	descriptions := []string{"Choose a model", "[Skill] Short description", "[Skill] Long description"}
	descriptionColumn := -1
	for index, description := range descriptions {
		column := ansi.StringWidth(lines[index][:strings.Index(lines[index], description)])
		if descriptionColumn < 0 {
			descriptionColumn = column
		} else if column != descriptionColumn {
			t.Fatalf("description columns = %d and %d: %q", descriptionColumn, column, lines)
		}
	}
	if rawLines[1] != userStyle.Render(lines[1]) {
		t.Fatalf("selected row does not use one foreground style: %q", rawLines[1])
	}
	if !strings.Contains(rawLines[2], subtleStyle.Render("[Skill] Long description")) {
		t.Fatalf("unselected description does not use subtle style: %q", rawLines[2])
	}
	if strings.Contains(lines[0], "[Skill]") {
		t.Fatalf("built-in command is labeled as a skill: %q", lines[0])
	}
	if !reflect.DeepEqual(userStyle.GetBackground(), messageStyle.GetBackground()) {
		t.Fatal("selected slash style has a background color")
	}
}
