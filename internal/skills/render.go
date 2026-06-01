package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMetadataBudget = 8000
	defaultSkillBodyLimit = 32000
)

// BuildPromptContext renders available skills and injects explicitly selected bodies.
func BuildPromptContext(catalog Catalog, userInput string) PromptContext {
	ctx := PromptContext{}
	ctx.Available = RenderAvailable(catalog, defaultMetadataBudget)
	mentioned := SelectMentioned(catalog.Skills, userInput)
	for _, skill := range mentioned {
		contents, err := os.ReadFile(skill.Path)
		if err != nil {
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("Failed to load skill %s at %s: %v", skill.Name, skill.Path, err))
			continue
		}
		body := string(contents)
		if len([]rune(body)) > defaultSkillBodyLimit {
			body = string([]rune(body)[:defaultSkillBodyLimit]) + "\n\n[Skill content truncated]\n"
		}
		ctx.Injected = append(ctx.Injected, Injection{Name: skill.Name, Path: skill.Path, Contents: body})
	}
	return ctx
}

// RenderAvailable returns the model-visible skills discovery block.
func RenderAvailable(catalog Catalog, budget int) string {
	if len(catalog.Skills) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines,
		"<skills_instructions>",
		"## Skills",
		"A skill is a local instruction set stored in a `SKILL.md` file. Available skills for this session:",
	)
	used := 0
	omitted := 0
	for _, skill := range catalog.Skills {
		line := renderSkillLine(skill)
		cost := len([]rune(line)) + 1
		if budget > 0 && used+cost > budget {
			minimum := fmt.Sprintf("- %s: (file: %s)", skill.Name, skill.Path)
			minimumCost := len([]rune(minimum)) + 1
			if used+minimumCost > budget {
				omitted++
				continue
			}
			line = minimum
			cost = minimumCost
		}
		used += cost
		lines = append(lines, line)
	}
	if omitted > 0 {
		lines = append(lines, fmt.Sprintf("Warning: %d additional skill(s) omitted because the skills list exceeded the context budget.", omitted))
	}
	lines = append(lines,
		"Use a skill when the user names it with `$skill-name` or when it clearly matches the task. When using a skill, read only the needed parts of its SKILL.md and referenced files.",
		"</skills_instructions>",
	)
	return strings.Join(lines, "\n")
}

// RenderInjections returns full skill instruction blocks for the current turn.
func RenderInjections(injections []Injection, warnings []string) []string {
	var blocks []string
	for _, warning := range warnings {
		blocks = append(blocks, "<skill_warning>\n"+warning+"\n</skill_warning>")
	}
	for _, injection := range injections {
		blocks = append(blocks, fmt.Sprintf("<skill>\n<name>%s</name>\n<path>%s</path>\n%s\n</skill>",
			injection.Name,
			injection.Path,
			strings.TrimSpace(injection.Contents),
		))
	}
	return blocks
}

func renderSkillLine(skill Skill) string {
	if strings.TrimSpace(skill.Description) == "" {
		return fmt.Sprintf("- %s: (file: %s)", skill.Name, skill.Path)
	}
	return fmt.Sprintf("- %s: %s (file: %s)", skill.Name, skill.Description, skill.Path)
}

// DefaultRoots returns the local skills roots Atlas scans by default.
func DefaultRoots(workdir string) []string {
	var roots []string
	if strings.TrimSpace(workdir) != "" {
		roots = append(roots, filepath.Join(workdir, ".agents", "skills"))
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		roots = append(roots,
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".atlas", "skills"),
		)
	}
	return roots
}
