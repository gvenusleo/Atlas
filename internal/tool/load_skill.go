package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/skill"
)

// LoadSkill loads the full SKILL.md for a local skill by name.
type LoadSkill struct {
	Skills *skill.Catalog
}

// Definition returns the model-visible definition for load_skill.
func (l LoadSkill) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "load_skill",
		Description: l.description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to load.",
				},
			},
			"required": []string{"name"},
		},
	}
}

// Run returns the skill path and full content using the name from the JSON parameters.
func (l LoadSkill) Run(ctx context.Context, arguments string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid load_skill arguments: %w", err)
	}
	args.Name = strings.TrimSpace(args.Name)
	if args.Name == "" {
		return "", fmt.Errorf("load_skill name is required")
	}
	found, ok := l.Skills.Lookup(args.Name)
	if !ok {
		return "", fmt.Errorf("skill not found: %s", args.Name)
	}
	return fmt.Sprintf(
		"Skill: %s\nDirectory: %s\nPath: %s\n\n%s",
		found.Name,
		filepath.ToSlash(found.Dir),
		filepath.ToSlash(found.Path),
		found.Content,
	), nil
}

func (l LoadSkill) description() string {
	var builder strings.Builder
	builder.WriteString("Load the full SKILL.md instructions for one available Atlas skill before using it.")
	summaries := l.Skills.Summaries()
	if len(summaries) == 0 {
		builder.WriteString("\n\nNo skills are currently available.")
		return builder.String()
	}
	builder.WriteString("\n\nAvailable skills:")
	for _, summary := range summaries {
		builder.WriteString("\n- ")
		builder.WriteString(summary.Name)
		builder.WriteString(": ")
		builder.WriteString(summary.Description)
	}
	return builder.String()
}
