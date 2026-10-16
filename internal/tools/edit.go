package tools

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gvenusleo/atlas/internal/agent"
)

// editArgs 是 edit tool 的输入；所有编辑替换都针对同一份原文件。
type editArgs struct {
	Path  string     `json:"path"`
	Edits []textEdit `json:"edits"`
}

// textEdit 是一次替换，NewText 用指针区分缺失参数和删除内容。
type textEdit struct {
	OldText string  `json:"oldText"`
	NewText *string `json:"newText"`
}

// EditDetails 是 edit 随结果附带的结构化数据，供界面渲染，不发给模型。
type EditDetails struct {
	// Starts 是各处替换在原文件中的起始行（从 1 起），顺序与 edits 一致。
	Starts []int `json:"starts"`
}

// matchedEdit 保存原文中的字节区间与输入下标。
type matchedEdit struct {
	index   int
	start   int
	end     int
	newText string
}

// Edit 构造 edit tool；workdir 应为工作目录绝对路径，相对路径以此为基准。
func Edit(workdir string) agent.Tool {
	return agent.Tool{
		Name: "edit",
		Description: "对现有 UTF-8 文本文件进行精确文本替换。" +
			"相对路径以工作目录为基准，支持绝对路径。文件最多 10 MiB。" +
			"每个 oldText 必须在原文件中唯一匹配，包括所有空格和换行。" +
			"同一次调用的替换区域不能重叠；相邻改动可合并为一次替换。" +
			"所有替换都匹配原文件，全部校验通过后一次写回。" +
			"newText 可以为空字符串，表示删除匹配内容。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "文件路径，支持相对路径和绝对路径。",
				},
				"edits": map[string]any{
					"type":     "array",
					"minItems": 1,
					"description": "一个或多个针对原文件的替换。" +
						"每个 oldText 必须唯一，替换区域不能重叠。",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"oldText": map[string]any{
								"type":        "string",
								"minLength":   1,
								"description": "待替换的原文，必须精确匹配且唯一。",
							},
							"newText": map[string]any{
								"type":        "string",
								"description": "替换后的文本，允许空字符串。",
							},
						},
						"required": []string{"oldText", "newText"},
					},
				},
			},
			"required": []string{"path", "edits"},
		},
		Execute: func(ctx context.Context, _ string, params jsontext.Value) (agent.ToolResult, error) {
			return editFile(ctx, workdir, params)
		},
	}
}

// parseEditArgs 解码并校验 edit 参数。
func parseEditArgs(params jsontext.Value) (editArgs, error) {
	var args editArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return args, fmt.Errorf("参数无效: %w", err)
	}
	if args.Path == "" {
		return args, errors.New("参数无效: path 不能为空")
	}
	if len(args.Edits) == 0 {
		return args, errors.New("参数无效: edits 至少包含一个替换")
	}
	for i, edit := range args.Edits {
		if edit.OldText == "" {
			return args, fmt.Errorf("参数无效: edits[%d].oldText 不能为空", i)
		}
		if edit.NewText == nil {
			return args, fmt.Errorf("参数无效: edits[%d].newText 必须是字符串", i)
		}
	}
	return args, nil
}

// editFile 读取现有文件，全部替换校验通过后写回；内容按字节精确匹配。
func editFile(ctx context.Context, workdir string, params jsontext.Value) (agent.ToolResult, error) {
	args, err := parseEditArgs(params)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	path := resolvePath(workdir, args.Path)
	data, err := readRegularFile(ctx, path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !validText(data) {
		return agent.ToolResult{}, fmt.Errorf("%s 不是支持的 UTF-8 文本", path)
	}

	output, starts, err := applyEdits(string(data), args.Edits)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("%s 替换失败: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}

	if err := os.WriteFile(path, []byte(output), 0644); err != nil {
		return agent.ToolResult{}, fmt.Errorf("%s 写入失败: %w", path, err)
	}

	details, err := json.Marshal(EditDetails{Starts: starts})
	if err != nil {
		return agent.ToolResult{}, err
	}
	text := fmt.Sprintf("已替换 %s 中的 %d 个文本块", path, len(args.Edits))
	return agent.ToolResult{
		Content: []agent.Content{agent.TextContent{Text: text}},
		Details: details,
	}, nil
}

// applyEdits 接收已校验的参数，在同一原文中定位替换；
// 同时返回各处替换在原文中的起始行（从 1 起，按 edits 的顺序）。
func applyEdits(text string, edits []textEdit) (string, []int, error) {
	matches := make([]matchedEdit, 0, len(edits))
	for i, edit := range edits {
		start := strings.Index(text, edit.OldText)
		if start < 0 {
			return "", nil, fmt.Errorf(
				"未找到 edits[%d].oldText，请确认内容、空格和换行与原文一致", i,
			)
		}

		// 从匹配起点后一个字节继续查找，重叠出现也算重复。
		if strings.Contains(text[start+1:], edit.OldText) {
			return "", nil, fmt.Errorf(
				"edits[%d].oldText 匹配多处，请补充上下文使其唯一", i,
			)
		}

		matches = append(matches, matchedEdit{
			index:   i,
			start:   start,
			end:     start + len(edit.OldText),
			newText: *edit.NewText,
		})
	}

	slices.SortFunc(matches, func(a, b matchedEdit) int {
		return cmp.Compare(a.start, b.start)
	})
	for i := 1; i < len(matches); i++ {
		previous := matches[i-1]
		current := matches[i]
		if previous.end > current.start {
			return "", nil, fmt.Errorf(
				"edits[%d] 与 edits[%d] 的区域重叠，请合并为一次替换",
				previous.index, current.index,
			)
		}
	}

	var output strings.Builder
	starts := make([]int, len(edits))
	cursor := 0
	for _, match := range matches {
		starts[match.index] = strings.Count(text[:match.start], "\n") + 1
		output.WriteString(text[cursor:match.start])
		output.WriteString(match.newText)
		cursor = match.end
	}
	output.WriteString(text[cursor:])

	result := output.String()
	if result == text {
		return "", nil, errors.New("替换后的内容与原文件相同")
	}
	return result, starts, nil
}
