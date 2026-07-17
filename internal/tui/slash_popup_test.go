package tui

import (
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
