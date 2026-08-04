package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// Edit applies exact, non-overlapping replacements to one text file.
type Edit struct {
	CWD string
}

// TextEdit describes one exact text replacement.
type TextEdit struct {
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// EditArgs describes the JSON parameters received by edit.
type EditArgs struct {
	Path  string     `json:"path"`
	Edits []TextEdit `json:"edits"`
}

// Definition returns the model-visible definition for edit.
func (Edit) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "edit",
		Description: "Apply one or more exact, unique, non-overlapping text replacements to a UTF-8 file. Every edit is matched against the original content and the call is all-or-nothing.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, relative to the session working directory or absolute.",
				},
				"edits": map[string]any{
					"type":        "array",
					"minItems":    1,
					"description": "Exact replacements. Each old_text must occur exactly once in the original file.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_text": map[string]any{"type": "string"},
							"new_text": map[string]any{"type": "string"},
						},
						"required": []string{"old_text", "new_text"},
					},
				},
			},
			"required": []string{"path", "edits"},
		},
	}
}

// Run validates and applies all requested replacements to one file.
func (e Edit) Run(ctx context.Context, arguments string) (string, error) {
	var args EditArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid edit arguments: %w", err)
	}
	var fields struct {
		Edits []map[string]json.RawMessage `json:"edits"`
	}
	if err := json.Unmarshal([]byte(arguments), &fields); err != nil {
		return "", fmt.Errorf("invalid edit arguments: %w", err)
	}
	path, err := resolveFilePath(e.CWD, args.Path)
	if err != nil {
		return "", fmt.Errorf("invalid edit arguments: %w", err)
	}
	if len(args.Edits) == 0 {
		return "", fmt.Errorf("invalid edit arguments: edits must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("edit %q: %w", args.Path, err)
	}
	hasBOM := bytes.HasPrefix(data, utf8BOM)
	if hasBOM {
		data = data[len(utf8BOM):]
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("edit %q: file is not valid UTF-8", args.Path)
	}
	newline := primaryNewline(data)
	original := normalizeNewlines(string(data))
	replacements := make([]replacement, 0, len(args.Edits))
	for i, edit := range args.Edits {
		if i >= len(fields.Edits) {
			return "", fmt.Errorf("edit %d: old_text and new_text are required", i+1)
		}
		oldValue, ok := fields.Edits[i]["old_text"]
		if !ok || string(oldValue) == "null" {
			return "", fmt.Errorf("edit %d: old_text is required", i+1)
		}
		newValue, ok := fields.Edits[i]["new_text"]
		if !ok || string(newValue) == "null" {
			return "", fmt.Errorf("edit %d: new_text is required", i+1)
		}
		oldText := normalizeNewlines(edit.OldText)
		newText := normalizeNewlines(edit.NewText)
		if oldText == "" {
			return "", fmt.Errorf("edit %d: old_text must not be empty", i+1)
		}
		if oldText == newText {
			return "", fmt.Errorf("edit %d: replacement does not change the file", i+1)
		}
		if count := countOccurrences(original, oldText); count != 1 {
			return "", fmt.Errorf("edit %d: old_text must match exactly once, found %d matches", i+1, count)
		}
		start := strings.Index(original, oldText)
		replacements = append(replacements, replacement{start: start, end: start + len(oldText), text: newText})
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	for i := 1; i < len(replacements); i++ {
		if replacements[i].start < replacements[i-1].end {
			return "", fmt.Errorf("edits %d and %d overlap", i, i+1)
		}
	}
	var output strings.Builder
	position := 0
	for _, replacement := range replacements {
		output.WriteString(original[position:replacement.start])
		output.WriteString(replacement.text)
		position = replacement.end
	}
	output.WriteString(original[position:])
	result := strings.ReplaceAll(output.String(), "\n", newline)
	if hasBOM {
		result = string(utf8BOM) + result
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(result), fileMode(path)); err != nil {
		return "", fmt.Errorf("edit %q: %w", args.Path, err)
	}
	return fmt.Sprintf("Applied %d edit(s) to %s", len(args.Edits), args.Path), nil
}

type replacement struct {
	start int
	end   int
	text  string
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func countOccurrences(value, pattern string) int {
	count := 0
	for start := 0; start <= len(value)-len(pattern); {
		index := strings.Index(value[start:], pattern)
		if index < 0 {
			break
		}
		count++
		start += index + 1
	}
	return count
}

func primaryNewline(data []byte) string {
	crlf := bytes.Count(data, []byte("\r\n"))
	lf := bytes.Count(data, []byte("\n")) - crlf
	if crlf > lf {
		return "\r\n"
	}
	if crlf == lf && crlf > 0 {
		firstLF := bytes.IndexByte(data, '\n')
		if firstLF > 0 && data[firstLF-1] == '\r' {
			return "\r\n"
		}
	}
	return "\n"
}

func fileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}
