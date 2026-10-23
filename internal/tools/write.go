package tools

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
)

// maxOldBytes 是覆盖时随结果附带的旧内容的大小上限；更大的文件不附带。
const maxOldBytes = 256 << 10

// writeArgs 是 write tool 的输入；Content 用指针区分参数与空内容。
type writeArgs struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

// Write 构造 write tool；cwd 应为工作目录绝对路径，相对路径以此为基准。
func Write(cwd string) agent.Tool {
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
		Execute: func(
			ctx context.Context, _ string, params jsontext.Value,
		) (agent.ToolResult, error) {
			return writeFile(ctx, cwd, params)
		},
	}
}

// writeFile 校验参数、创建父目录并完整写入文件。
func writeFile(ctx context.Context, cwd string, params jsontext.Value) (agent.ToolResult, error) {
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

	path := resolvePath(cwd, args.Path)
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return agent.ToolResult{}, fmt.Errorf("%s 检查失败: %w", path, err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return agent.ToolResult{}, fmt.Errorf("%s 不是普通文件", path)
	}
	existed := err == nil
	var old *string
	if existed && info.Size() <= maxOldBytes {
		old = readOld(path)
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

	details, err := json.Marshal(protocol.WriteDetails{Overwrote: existed, Old: old})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		Content: []agent.Content{
			agent.TextContent{Text: fmt.Sprintf("已写入 %s", path)},
		},
		Details: details,
	}, nil
}

// readOld 读出覆盖前的内容；读不出或不是有效的 UTF-8 文本时返回 nil：这只影响界面的对比，
// 不该让写入失败（JSON 编码不接受非法的 UTF-8）。
func readOld(path string) *string {
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return nil
	}
	return new(string(data))
}
