package tui

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

const (
	modelCommandName   = "model"
	resumeCommandName  = "resume"
	compactCommandName = "compact"
	quitCommandName    = "quit"
	maxSlashPopupRows  = 5
)

type slashCommand struct {
	name        string
	description string
	skill       bool
}

type slashCompletionTarget struct {
	line       int
	start      int
	end        int
	token      string
	query      string
	skillsOnly bool
}

type slashPopup struct {
	commands       []slashCommand
	matches        []slashCommand
	selected       int
	dismissedValue string
	dismissed      slashCompletionTarget
	target         slashCompletionTarget
	query          string
}

func newSlashPopup() slashPopup {
	return slashPopup{commands: []slashCommand{
		{
			name:        modelCommandName,
			description: "Choose a model and reasoning effort",
		},
		{
			name:        resumeCommandName,
			description: "Resume a saved session",
		},
		{
			name:        compactCommandName,
			description: "Compact earlier context",
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
	seen := map[string]bool{modelCommandName: true, resumeCommandName: true, compactCommandName: true, quitCommandName: true}
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

// sync updates matches for the slash token at the textarea cursor.
func (p *slashPopup) sync(input textarea.Model) {
	value := input.Value()
	target, ok := currentSlashCompletionTarget(input)
	if p.dismissedValue != "" && (value != p.dismissedValue || target != p.dismissed) {
		p.dismissedValue = ""
		p.dismissed = slashCompletionTarget{}
	}
	if !ok || (value == p.dismissedValue && target == p.dismissed) {
		p.matches = nil
		p.selected = 0
		p.target = slashCompletionTarget{}
		p.query = ""
		return
	}

	query := strings.ToLower(target.query)
	targetChanged := target != p.target
	p.target = target
	p.query = query
	commands := p.commands
	if target.skillsOnly {
		commands = make([]slashCommand, 0, len(p.commands))
		for _, command := range p.commands {
			if command.skill {
				commands = append(commands, command)
			}
		}
	}
	p.matches = rankSlashCommands(commands, query)
	if targetChanged {
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
	p.dismissed = p.target
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

// currentSlashCompletionTarget locates the whitespace-delimited slash token at the cursor.
func currentSlashCompletionTarget(input textarea.Model) (slashCompletionTarget, bool) {
	lines := strings.Split(input.Value(), "\n")
	lineIndex := input.Line()
	if lineIndex < 0 || lineIndex >= len(lines) {
		return slashCompletionTarget{}, false
	}

	line := []rune(lines[lineIndex])
	column := min(max(input.Column(), 0), len(line))
	if column > 0 && unicode.IsSpace(line[column-1]) {
		return slashCompletionTarget{}, false
	}
	start := column
	for start > 0 && !unicode.IsSpace(line[start-1]) {
		start--
	}
	end := column
	for end < len(line) && !unicode.IsSpace(line[end]) {
		end++
	}
	if start >= end || line[start] != '/' {
		return slashCompletionTarget{}, false
	}
	token := string(line[start:end])
	query := strings.TrimPrefix(token, "/")
	if query != "" && !validSlashCommandName(query) {
		return slashCompletionTarget{}, false
	}

	remainingLine := string(line[:start]) + string(line[end:])
	remaining := append([]string(nil), lines...)
	remaining[lineIndex] = remainingLine
	return slashCompletionTarget{
		line:       lineIndex,
		start:      start,
		end:        end,
		token:      token,
		query:      query,
		skillsOnly: strings.TrimSpace(strings.Join(remaining, "\n")) != "",
	}, true
}

// replaceSlashCompletion replaces only the active slash token and preserves the draft.
func replaceSlashCompletion(input *textarea.Model, target slashCompletionTarget, name string) bool {
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

	suffix := line[target.end:]
	input.SetCursorColumn(target.end)
	for range target.end - target.start {
		updated, _ := input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		*input = updated
	}
	input.InsertString("/" + name)
	if len(suffix) == 0 {
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

// selectedSkillNames preserves ACP's whitespace-delimited slash command semantics.
func selectedSkillNames(text string) []string {
	var names []string
	seen := make(map[string]bool)
	for field := range strings.FieldsSeq(text) {
		name, ok := slashCommandName(field)
		if !ok || name == modelCommandName || name == resumeCommandName || name == compactCommandName || name == quitCommandName || seen[name] {
			continue
		}
		names = append(names, name)
		seen[name] = true
	}
	return names
}

// resumeCommandSessionID parses the built-in command and its optional exact session ID.
func resumeCommandSessionID(text string) (string, bool) {
	text = strings.TrimSpace(text)
	prefix := "/" + resumeCommandName
	if text == prefix {
		return "", true
	}
	if strings.HasPrefix(text, prefix+" ") || strings.HasPrefix(text, prefix+"\t") || strings.HasPrefix(text, prefix+"\n") {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
	}
	return "", false
}

// compactCommandInstruction parses the built-in command and its optional instruction.
func compactCommandInstruction(text string) (string, bool) {
	text = strings.TrimSpace(text)
	prefix := "/" + compactCommandName
	if text == prefix {
		return "", true
	}
	if strings.HasPrefix(text, prefix+" ") || strings.HasPrefix(text, prefix+"\t") || strings.HasPrefix(text, prefix+"\n") {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
	}
	return "", false
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
