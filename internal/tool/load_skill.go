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

// LoadSkill 按名称加载本地 skill 的完整 SKILL.md。
type LoadSkill struct {
	Skills *skill.Catalog
}

// Definition 返回 load_skill 的模型可见定义。
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

// Run 使用 JSON 参数中的 name 返回 skill 路径和完整正文。
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
