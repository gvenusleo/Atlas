package tool

import (
	"encoding/json"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// DisplayTitle 返回工具调用适合展示给用户的标题。
func DisplayTitle(call model.ToolCall) string {
	if title := primaryDisplayTitle(call); title != "" {
		return title
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// primaryDisplayTitle 从内置工具参数中提取最能说明动作的字段。
func primaryDisplayTitle(call model.ToolCall) string {
	prefix, key := "", ""
	switch call.Name {
	case "read_file":
		prefix, key = "Read: ", "path"
	case "write_file":
		prefix, key = "Write: ", "path"
	case "edit_file":
		prefix, key = "Edit: ", "path"
	case "list_files":
		prefix, key = "List: ", "path"
	case "search_text":
		prefix, key = "Search: ", "query"
	case "web_search":
		prefix, key = "WebSearch: ", "query"
	case "web_fetch":
		prefix, key = "WebFetch: ", "url"
	case "run_shell":
		prefix, key = "Run: ", "command"
	default:
		return ""
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return ""
	}
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}
