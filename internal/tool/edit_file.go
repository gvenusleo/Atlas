package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// EditFile replaces a unique text block in a local file.
type EditFile struct {
	CWD string
}

// EditFileArgs is the JSON parameters for edit_file.
type EditFileArgs struct {
	Path    string  `json:"path"`
	OldText string  `json:"old_text"`
	NewText *string `json:"new_text"`
}

// Definition returns the model-visible definition for edit_file.
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
					"description": "Exact text to replace. Must appear exactly once in the file.",
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

// Run modifies a file using the path, old_text, and new_text from the JSON parameters.
func (e EditFile) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseEditFileArgs(arguments)
	if err != nil {
		return "", err
	}
	return editFileContent(ctx, resolveToolPath(e.CWD, args.Path), args.OldText, *args.NewText)
}

// ParseEditFileArgs parses and validates edit_file parameters.
func ParseEditFileArgs(arguments string) (EditFileArgs, error) {
	var args EditFileArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return EditFileArgs{}, fmt.Errorf("invalid edit_file arguments: %w", err)
	}
	if args.Path == "" {
		return EditFileArgs{}, fmt.Errorf("edit_file path is required")
	}
	if args.OldText == "" {
		return EditFileArgs{}, fmt.Errorf("edit_file old_text is required")
	}
	if args.NewText == nil {
		return EditFileArgs{}, fmt.Errorf("edit_file new_text is required")
	}
	return args, nil
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
	updated, err := ApplyEditFileContent(string(data), oldText, newText)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced 1 block in %s", path), nil
}

// ApplyEditFileContent applies the edit_file replacement rules to the original text.
func ApplyEditFileContent(content, oldText, newText string) (string, error) {
	start, count := editFileOccurrence(content, oldText)
	if start < 0 {
		return "", fmt.Errorf("edit_file old_text not found")
	}
	if count > 1 {
		return "", fmt.Errorf("edit_file old_text is not unique")
	}
	return content[:start] + newText + content[start+len(oldText):], nil
}

func editFileOccurrence(content, oldText string) (int, int) {
	first := -1
	count := 0
	offset := 0
	for {
		index := strings.Index(content[offset:], oldText)
		if index < 0 {
			return first, count
		}
		start := offset + index
		if first < 0 {
			first = start
		}
		count++
		offset = start + 1
	}
}
