package tools

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 定义文本读取预算与默认行数。
const (
	readMaxTextBytes = 50 << 10
	readMaxLines     = 2000
)

// readArgs 是 read tool 的输入；offset 和 limit 省略时有执行函数设置默认值。
type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// ReadDetails 是 read 读取文本时随结果附带的结构化数据，供界面渲染，不发给模型。
// 它是实际返回的窗口，而不是请求的：默认值与返回预算都已计入。
type ReadDetails struct {
	// Start 与 End 是实际返回的第一行与最后一行的行号（从 1 起）；空文件时 End 为 0。
	Start int `json:"start"`
	End   int `json:"end"`
	// Total 是文件的总行数。
	Total int `json:"total"`
}

// Read 构造 read tool；workdir 应为工作目录绝对路径，相对路径以此为基准。
func Read(workdir string) agent.Tool {
	return agent.Tool{
		Name: "read",
		Description: "读取 UTF-8 文本或图片（JPEG、PNG、GIF、Webp）。" +
			"相对路径以工作目录为基准。单个文件最多 10 MiB。" +
			"文本最多返回 2000 行或 50 KiB，保留完整行；" +
			"内容未读完时按提示使用 offset 继续。" +
			"offset 从 1 开始，limit 是最大行数；图片不使用这两个参数。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "文件路径，支持相对路径和绝对路径",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "起始行号，从 1 开始，默认为 1",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "最大返回行数，默认为 2000",
				},
			},
			"required": []string{"path"},
		},
		Execute: func(ctx context.Context, _ string, params jsontext.Value) (agent.ToolResult, error) {
			return readFile(ctx, workdir, params)
		},
	}
}

// readFile 校验参数并读取文件，文本按行读取，图片返回附件。
func readFile(ctx context.Context, workdir string, params jsontext.Value) (agent.ToolResult, error) {
	args := readArgs{Offset: 1, Limit: readMaxLines}
	if err := json.Unmarshal(params, &args); err != nil {
		return agent.ToolResult{}, fmt.Errorf("参数无效: %w", err)
	}
	if args.Path == "" {
		return agent.ToolResult{}, fmt.Errorf("参数无效: path 不能为空")
	}
	if args.Offset < 1 {
		return agent.ToolResult{}, fmt.Errorf("参数无效: offset 必须大于等于 1")
	}
	if args.Limit < 1 {
		return agent.ToolResult{}, fmt.Errorf("参数无效: limit 必须大于等于 1")
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	path := resolvePath(workdir, args.Path)
	data, err := readRegularFile(ctx, path)
	if err != nil {
		return agent.ToolResult{}, err
	}

	mime := http.DetectContentType(data)
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return agent.ToolResult{
			Content: []agent.Content{
				agent.TextContent{Text: fmt.Sprintf("读取图片 %s (%s)", path, mime)},
				agent.ImageContent{
					Data:     base64.StdEncoding.EncodeToString(data),
					MIMEType: mime,
				},
			},
		}, nil
	}

	if !validText(data) {
		return agent.ToolResult{}, fmt.Errorf("%s 不是支持的图片或 UTF-8 文本", path)
	}

	text, window, err := readText(string(data), args.Offset, args.Limit)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("%s 读取失败: %w", path, err)
	}
	details, err := json.Marshal(window)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		Content: []agent.Content{
			agent.TextContent{Text: text},
		},
		Details: details,
	}, nil
}

// readText 按行选取文本，超过返回预算时只保留完整行并给出续读位置；
// 同时返回实际返回的窗口。
func readText(text string, offset, limit int) (string, ReadDetails, error) {
	if text == "" {
		if offset != 1 {
			return "", ReadDetails{}, errors.New("文件为空, offset 只能为 1")
		}
		return "", ReadDetails{Start: 1}, nil
	}

	total := strings.Count(text, "\n") + 1
	// 文件末尾的换行符不额外计为一行。
	if strings.HasSuffix(text, "\n") {
		total--
	}
	if offset > total {
		return "", ReadDetails{}, fmt.Errorf("offset=%d 超出文件范围，共 %d 行", offset, total)
	}

	count := min(limit, readMaxLines, total-offset+1)
	selected := make([]string, 0, count)
	lineNumber := 0
	size := 0

	// 只保存本次返回的行，避免为整个文件分配行切片。
	for line := range strings.SplitSeq(text, "\n") {
		lineNumber++
		if lineNumber < offset {
			continue
		}
		if len(selected) == count {
			break
		}
		added := len(line)
		if len(selected) > 0 {
			added++ // Join 插入的换行符也占用返回预算。
		}
		if size+added > readMaxTextBytes {
			break
		}
		size += added
		selected = append(selected, line)
	}

	if len(selected) == 0 {
		return "", ReadDetails{}, fmt.Errorf("第 %d 行超过 50 KiB", offset)
	}

	end := offset + len(selected) - 1
	output := strings.Join(selected, "\n")
	if end < total {
		output += fmt.Sprintf(
			"\n\n[显示第 %d–%d 行，共 %d 行。使用 offset=%d 继续读取。]",
			offset, end, total, end+1,
		)
	}
	return output, ReadDetails{Start: offset, End: end, Total: total}, nil
}
