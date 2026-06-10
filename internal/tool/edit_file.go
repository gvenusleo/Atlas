package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// EditFile 替换本地文件中的一个或多个唯一文本块。
type EditFile struct{}

type editFileReplacement struct {
	OldText string  `json:"old_text"`
	NewText *string `json:"new_text"`
}

type editFileMatch struct {
	start   int
	end     int
	newText string
}

// Definition 返回 edit_file 的模型可见定义。
func (EditFile) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "edit_file",
		Description: "Replace one or more unique text blocks in an existing local file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to edit.",
				},
				"edits": map[string]any{
					"type":        "array",
					"description": "Replacements to apply to the original file content. Each old_text must appear exactly once and replacements must not overlap.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_text": map[string]any{
								"type":        "string",
								"description": "Exact text to replace. Must appear exactly once in the original file.",
							},
							"new_text": map[string]any{
								"type":        "string",
								"description": "Replacement text. Use an empty string to delete old_text.",
							},
						},
						"required": []string{"old_text", "new_text"},
					},
				},
			},
			"required": []string{"path", "edits"},
		},
	}
}

// Run 使用 JSON 参数中的 path 和 edits 修改文件。
func (EditFile) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path  string                `json:"path"`
		Edits []editFileReplacement `json:"edits"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid edit_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("edit_file path is required")
	}
	if len(args.Edits) == 0 {
		return "", fmt.Errorf("edit_file edits must contain at least one replacement")
	}
	return editFileContent(ctx, args.Path, args.Edits)
}

func editFileContent(ctx context.Context, path string, edits []editFileReplacement) (string, error) {
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
	matches, err := editFileMatches(content, edits)
	if err != nil {
		return "", err
	}
	updated := applyEditFileMatches(content, matches)
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d blocks in %s", len(matches), path), nil
}

func editFileMatches(content string, edits []editFileReplacement) ([]editFileMatch, error) {
	matches := make([]editFileMatch, 0, len(edits))
	for i, edit := range edits {
		if edit.OldText == "" {
			return nil, fmt.Errorf("edit_file edits[%d].old_text is required", i)
		}
		if edit.NewText == nil {
			return nil, fmt.Errorf("edit_file edits[%d].new_text is required", i)
		}
		start, count := editFileOccurrence(content, edit.OldText)
		if start < 0 {
			return nil, fmt.Errorf("edit_file edits[%d].old_text not found", i)
		}
		if count > 1 {
			return nil, fmt.Errorf("edit_file edits[%d].old_text is not unique", i)
		}
		end := start + len(edit.OldText)
		for _, match := range matches {
			if start < match.end && end > match.start {
				return nil, fmt.Errorf("edit_file edits[%d].old_text overlaps another replacement", i)
			}
		}
		matches = append(matches, editFileMatch{start: start, end: end, newText: *edit.NewText})
	}
	return matches, nil
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

func applyEditFileMatches(content string, matches []editFileMatch) string {
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start < matches[j].start
	})
	updated := content
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		updated = updated[:match.start] + match.newText + updated[match.end:]
	}
	return updated
}
