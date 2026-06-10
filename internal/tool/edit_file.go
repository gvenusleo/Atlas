package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// EditFile replaces one unique text block in an existing local file.
type EditFile struct{}

// Definition 返回 edit_file 的模型可见定义。
func (EditFile) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "edit_file",
		Description: "Replace one unique text block in an existing local file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to edit.",
				},
				"old_text": map[string]any{
					"type":        "string",
					"description": "Exact text to replace. Must appear exactly once.",
				},
				"new_text": map[string]any{
					"type":        "string",
					"description": "Replacement text. Use an empty string to delete old_text.",
				},
			},
			"required": []string{"path", "old_text", "new_text"},
		},
	}
}

// Run 使用 JSON 参数中的 path、old_text 和 new_text 修改文件。
func (EditFile) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path    string  `json:"path"`
		OldText string  `json:"old_text"`
		NewText *string `json:"new_text"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid edit_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("edit_file path is required")
	}
	if args.OldText == "" {
		return "", fmt.Errorf("edit_file old_text is required")
	}
	if args.NewText == nil {
		return "", fmt.Errorf("edit_file new_text is required")
	}
	return editFileContent(ctx, args.Path, args.OldText, *args.NewText)
}

func editFileContent(ctx context.Context, path, oldText, newText string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("edit_file path is a directory: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, oldText)
	if count == 0 {
		return "", fmt.Errorf("edit_file old_text not found")
	}
	if count > 1 {
		return "", fmt.Errorf("edit_file old_text is not unique: found %d matches", count)
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return "replaced 1 block in " + path, nil
}
