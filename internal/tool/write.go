package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liuyuxin/atlas/internal/model"
)

// Write creates or completely replaces a file.
type Write struct {
	CWD string
}

// WriteArgs describes the JSON parameters received by write.
type WriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Definition returns the model-visible definition for write.
func (Write) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "write",
		Description: "Create a new file or completely replace an existing file. Parent directories are created automatically.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, relative to the session working directory or absolute.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Complete file content. An empty string creates or truncates the file.",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

// Run creates or completely replaces the requested file.
func (w Write) Run(ctx context.Context, arguments string) (string, error) {
	var args WriteArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid write arguments: %w", err)
	}
	var fields struct {
		Content *string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &fields); err != nil {
		return "", fmt.Errorf("invalid write arguments: %w", err)
	}
	if fields.Content == nil {
		return "", fmt.Errorf("invalid write arguments: content is required")
	}
	path, err := resolveFilePath(w.CWD, args.Path)
	if err != nil {
		return "", fmt.Errorf("invalid write arguments: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create parent directories for %q: %w", args.Path, err)
	}
	if err := os.WriteFile(path, []byte(*fields.Content), 0o644); err != nil {
		return "", fmt.Errorf("write %q: %w", args.Path, err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(*fields.Content), args.Path), nil
}
