package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liuyuxin/atlas/internal/model"
)

// WriteFile writes content to a local text file.
type WriteFile struct {
	CWD string
}

// WriteFileArgs is the JSON parameters for write_file.
type WriteFileArgs struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

// Definition returns the model-visible definition for write_file.
func (WriteFile) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "write_file",
		Description: "Write a text file to the local filesystem.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Path to write."},
				"content": map[string]any{"type": "string", "description": "Content to write."},
			},
			"required": []string{"path", "content"},
		},
	}
}

// Run writes a file using the path and content from the JSON parameters.
func (w WriteFile) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseWriteFileArgs(arguments)
	if err != nil {
		return "", err
	}
	return writeFileContent(ctx, resolveToolPath(w.CWD, args.Path), *args.Content)
}

// ParseWriteFileArgs parses and validates write_file parameters.
func ParseWriteFileArgs(arguments string) (WriteFileArgs, error) {
	var args WriteFileArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return WriteFileArgs{}, fmt.Errorf("invalid write_file arguments: %w", err)
	}
	if args.Path == "" {
		return WriteFileArgs{}, fmt.Errorf("write_file path is required")
	}
	if args.Content == nil {
		return WriteFileArgs{}, fmt.Errorf("write_file content is required")
	}
	return args, nil
}

func writeFileContent(ctx context.Context, path, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return "wrote " + path, nil
}
