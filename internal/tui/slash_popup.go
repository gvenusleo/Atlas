package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

const (
	modelCommandName  = "model"
	maxSlashPopupRows = 5
)

type slashCommand struct {
	name        string
	description string
}

type slashPopup struct {
	commands       []slashCommand
	matches        []slashCommand
	selected       int
	dismissedValue string
}

func newSlashPopup() slashPopup {
	return slashPopup{commands: []slashCommand{{
		name:        modelCommandName,
		description: "Choose a model and reasoning effort",
	}}}
}

// setSkills rebuilds the suggestion catalog while reserving built-in commands.
func (p *slashPopup) setSkills(summaries []runtime.SkillSummary) {
	commands := newSlashPopup().commands
	seen := map[string]bool{modelCommandName: true}
	for _, summary := range summaries {
		if !validSlashCommandName(summary.Name) || seen[summary.Name] {
			continue
		}
		seen[summary.Name] = true
		commands = append(commands, slashCommand{
			name:        summary.Name,
			description: summary.Description,
		})
	}
	p.commands = commands
	p.matches = nil
	p.selected = 0
}

// sync updates the visible matches for a command token at the start of the draft.
func (p *slashPopup) sync(value string) {
	if p.dismissedValue != "" && value != p.dismissedValue {
		p.dismissedValue = ""
	}
	query, ok := slashCommandQuery(value)
	if !ok || value == p.dismissedValue {
		p.matches = nil
		p.selected = 0
		return
	}

	query = strings.ToLower(query)
	p.matches = p.matches[:0]
	for _, command := range p.commands {
		if strings.HasPrefix(strings.ToLower(command.name), query) {
			p.matches = append(p.matches, command)
		}
	}
	p.selected = min(p.selected, max(len(p.matches)-1, 0))
}

func (p slashPopup) active() bool {
	return len(p.matches) > 0
}

func (p *slashPopup) move(delta int) {
	if !p.active() {
		return
	}
	p.selected = min(max(p.selected+delta, 0), len(p.matches)-1)
}

func (p slashPopup) selectedCommand() (slashCommand, bool) {
	if !p.active() || p.selected < 0 || p.selected >= len(p.matches) {
		return slashCommand{}, false
	}
	return p.matches[p.selected], true
}

func (p *slashPopup) dismiss(value string) {
	p.dismissedValue = value
	p.matches = nil
	p.selected = 0
}

// render displays a bounded command list with descriptions truncated to the terminal width.
func (p slashPopup) render(width, maxRows int) string {
	if !p.active() || maxRows <= 0 {
		return ""
	}
	maxRows = min(maxRows, len(p.matches))
	start := pickerWindowStart(len(p.matches), p.selected, maxRows)
	end := min(start+maxRows, len(p.matches))
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		command := p.matches[i]
		prefix := "  "
		nameStyle := messageStyle
		if i == p.selected {
			prefix = userStyle.Render("› ")
			nameStyle = userStyle
		}
		name := ansi.Truncate("/"+command.name, max(width-2, 1), "…")
		line := prefix + nameStyle.Render(name)
		remaining := max(width-ansi.StringWidth(prefix)-ansi.StringWidth(name)-2, 0)
		description := singleLineDisplayText(command.description)
		if description != "" && remaining > 0 {
			line += "  " + mutedStyle.Render(ansi.Truncate(description, remaining, "…"))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func slashCommandQuery(value string) (string, bool) {
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\r\n") {
		return "", false
	}
	query := strings.TrimPrefix(value, "/")
	if query != "" && !validSlashCommandName(query) {
		return "", false
	}
	return query, true
}

// selectedSkillNames preserves ACP's whitespace-delimited slash command semantics.
func selectedSkillNames(text string) []string {
	var names []string
	seen := make(map[string]bool)
	for field := range strings.FieldsSeq(text) {
		name, ok := slashCommandName(field)
		if !ok || name == modelCommandName || seen[name] {
			continue
		}
		names = append(names, name)
		seen[name] = true
	}
	return names
}

func slashCommandName(text string) (string, bool) {
	name := strings.TrimPrefix(text, "/")
	if name == text || !validSlashCommandName(name) {
		return "", false
	}
	return name, true
}

func validSlashCommandName(name string) bool {
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return name != ""
}
