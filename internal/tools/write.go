package tools

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gvenusleo/atlas/internal/agent"
)

// writeArgs 是 write tool 的输入；Content 用指针区分参数与空内容。
type writeArgs struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

// Write 构造 write tool；workdir 应为工作目录绝对路径，相对路径以此为基准。
func Write(workdir string) agent.Tool {
	return agent.Tool{
		Name: "write",
		Description: "创建或完整覆盖 UTF-8 文本文件，自动创建父目录。" +
			"相对路径以工作目录为基准，支持绝对路径。" +
			"content 可以为空字符串，用于创建空文件或清空内容。" +
			"仅用于新文件或完整重写；局部修改使用 edit。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "文件路径，支持相对路径和绝对路径。",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "完整文件内容，允许空字符串。",
				},
			},
			"required": []string{"path", "content"},
		},
		Execute: func(ctx context.Context, _ string, params jsontext.Value) (agent.ToolResult, error) {
			return writeFile(ctx, workdir, params)
		},
	}
}

// writeFile 校验参数、创建父目录并完整写入文件。
func writeFile(ctx context.Context, workdir string, params jsontext.Value) (agent.ToolResult, error) {
	var args writeArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return agent.ToolResult{}, fmt.Errorf("参数无效: %w", err)
	}
	if args.Path == "" {
		return agent.ToolResult{}, errors.New("参数无效: path 不能为空")
	}
	if args.Content == nil {
		return agent.ToolResult{}, errors.New("参数无效: content 必须是字符串")
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	path := resolvePath(workdir, args.Path)
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return agent.ToolResult{}, fmt.Errorf("%s 检查失败: %w", path, err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return agent.ToolResult{}, fmt.Errorf("%s 不是普通文件", path)
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return agent.ToolResult{}, fmt.Errorf("%s 创建父目录失败: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	if err := os.WriteFile(path, []byte(*args.Content), 0644); err != nil {
		return agent.ToolResult{}, fmt.Errorf("%s 写入失败: %w", path, err)
	}

	return agent.ToolResult{
		Content: []agent.Content{
			agent.TextContent{
				Text: fmt.Sprintf("已写入 %s", path),
			},
		},
	}, nil
}
