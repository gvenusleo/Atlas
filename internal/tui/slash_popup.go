package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

const (
	modelCommandName  = "model"
	quitCommandName   = "quit"
	maxSlashPopupRows = 5
)

type slashCommand struct {
	name        string
	description string
	skill       bool
}

type slashPopup struct {
	commands       []slashCommand
	matches        []slashCommand
	selected       int
	dismissedValue string
	query          string
}

func newSlashPopup() slashPopup {
	return slashPopup{commands: []slashCommand{
		{
			name:        modelCommandName,
			description: "Choose a model and reasoning effort",
		},
		{
			name:        quitCommandName,
			description: "Quit Atlas",
		},
	}}
}

// setSkills rebuilds the suggestion catalog while reserving built-in commands.
func (p *slashPopup) setSkills(summaries []runtime.SkillSummary) {
	commands := newSlashPopup().commands
	seen := map[string]bool{modelCommandName: true, quitCommandName: true}
	for _, summary := range summaries {
		if !validSlashCommandName(summary.Name) || seen[summary.Name] {
			continue
		}
		seen[summary.Name] = true
		commands = append(commands, slashCommand{
			name:        summary.Name,
			description: summary.Description,
			skill:       true,
		})
	}
	p.commands = commands
	p.matches = nil
	p.selected = 0
	p.query = ""
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
		p.query = ""
		return
	}

	query = strings.ToLower(query)
	queryChanged := query != p.query
	p.query = query
	p.matches = rankSlashCommands(p.commands, query)
	if queryChanged {
		p.selected = 0
	} else {
		p.selected = min(p.selected, max(len(p.matches)-1, 0))
	}
}

// rankSlashCommands keeps obvious matches ahead of looser fuzzy matches.
func rankSlashCommands(commands []slashCommand, query string) []slashCommand {
	if query == "" {
		return append([]slashCommand(nil), commands...)
	}

	targets := make([]string, len(commands))
	for index, command := range commands {
		targets[index] = strings.ToLower(command.name)
	}

	var groups [4][]slashCommand
	for _, rank := range list.DefaultFilter(query, targets) {
		name := targets[rank.Index]
		group := 3
		switch {
		case name == query:
			group = 0
		case strings.HasPrefix(name, query):
			group = 1
		case strings.Contains(name, query):
			group = 2
		}
		groups[group] = append(groups[group], commands[rank.Index])
	}

	matches := make([]slashCommand, 0, len(commands))
	for _, group := range groups {
		matches = append(matches, group...)
	}
	return matches
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
	p.query = ""
}

// render displays a bounded command list with descriptions truncated to the terminal width.
func (p slashPopup) render(width, maxRows int) string {
	if !p.active() || maxRows <= 0 {
		return ""
	}
	maxRows = min(maxRows, len(p.matches))
	start := pickerWindowStart(len(p.matches), p.selected, maxRows)
	end := min(start+maxRows, len(p.matches))
	nameColumnWidth := 1
	for _, command := range p.matches[start:end] {
		nameColumnWidth = max(nameColumnWidth, ansi.StringWidth("/"+command.name))
	}
	nameColumnWidth = min(nameColumnWidth, max((width-2)/2, 1))
	descriptionWidth := max(width-2-nameColumnWidth-2, 0)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		command := p.matches[i]
		name := ansi.Truncate("/"+command.name, nameColumnWidth, "…")
		name += strings.Repeat(" ", max(nameColumnWidth-ansi.StringWidth(name), 0))
		description := singleLineDisplayText(command.description)
		if command.skill {
			description = "[Skill] " + description
		}
		if descriptionWidth > 0 {
			description = ansi.Truncate(description, descriptionWidth, "…")
		} else {
			description = ""
		}

		if i == p.selected {
			line := "› " + name
			if description != "" {
				line += "  " + description
			}
			lines = append(lines, userStyle.Render(line))
			continue
		}

		line := "  " + messageStyle.Render(name)
		if description != "" {
			line += "  " + subtleStyle.Render(description)
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
		if !ok || name == modelCommandName || name == quitCommandName || seen[name] {
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
