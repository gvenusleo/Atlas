package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// DisplayTitle 返回工具调用适合展示给用户的标题。
// cwd 用于缩短文件路径：如果文件在 cwd 下，只显示相对路径。
func DisplayTitle(call model.ToolCall, cwd string) string {
	if title := primaryDisplayTitle(call, cwd); title != "" {
		return title
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// primaryDisplayTitle 从内置工具参数中提取最能说明动作的字段。
func primaryDisplayTitle(call model.ToolCall, cwd string) string {
	prefix, key := "", ""
	switch call.Name {
	case "read_file":
		prefix, key = "Read: ", "path"
	case "write_file":
		prefix, key = "Write: ", "path"
	case "edit_file":
		prefix, key = "Edit: ", "path"
	case "apply_patch":
		prefix, key = "Patch: ", "patch"
	case "glob":
		prefix, key = "Glob: ", "pattern"
	case "grep":
		prefix, key = "Grep: ", "pattern"
	case "web_search":
		prefix, key = "WebSearch: ", "query"
	case "web_fetch":
		prefix, key = "WebFetch: ", "url"
	case "load_skill":
		prefix, key = "LoadSkill: ", "name"
	case "todo_write":
		prefix, key = "Todo: ", "todos"
	case "run_shell":
		prefix, key = "Run: ", "command"
	default:
		return ""
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return ""
	}
	if call.Name == "todo_write" {
		items, ok := args["todos"].([]any)
		if !ok || len(items) == 0 {
			return ""
		}
		return fmt.Sprintf("%s%d items", prefix, len(items))
	}
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return ""
	}
	if call.Name == "apply_patch" {
		value, _, _ = strings.Cut(value, "\n")
	}
	if isFileTool(call.Name) {
		value = shortenPath(value, cwd)
	}
	return prefix + value
}

// isFileTool 判断工具是否使用文件路径作为主参数。
func isFileTool(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file":
		return true
	}
	return false
}

// shortenPath 将绝对路径缩短为基于 cwd 的相对路径（如果文件在 cwd 下）。
func shortenPath(path, cwd string) string {
	if cwd == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
