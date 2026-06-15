package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liuyuxin/atlas/internal/model"
)

// WriteFile 写入本地文本文件内容。
type WriteFile struct{}

// WriteFileArgs 是 write_file 的 JSON 参数。
type WriteFileArgs struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

// Definition 返回 write_file 的模型可见定义。
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

// Run 使用 JSON 参数中的 path 和 content 写入文件。
func (WriteFile) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseWriteFileArgs(arguments)
	if err != nil {
		return "", err
	}
	return writeFileContent(ctx, args.Path, *args.Content)
}

// ParseWriteFileArgs 解析并校验 write_file 参数。
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
