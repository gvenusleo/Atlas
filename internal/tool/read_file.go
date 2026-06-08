package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/liuyuxin/atlas/internal/model"
)

const maxReadFileBytes = 256 * 1024

// ReadFile 读取本地文本文件内容。
type ReadFile struct{}

// Definition 返回 read_file 的模型可见定义。
func (ReadFile) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "read_file",
		Description: "Read a text file from the local filesystem.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to read.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Run 使用 JSON 参数中的 path 读取文件。
func (ReadFile) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("read_file path is required")
	}
	return readFileContent(ctx, args.Path)
}

func readFileContent(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxReadFileBytes {
		return "", fmt.Errorf("file is too large: %d bytes", info.Size())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
