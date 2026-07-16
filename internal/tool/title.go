package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// DisplayTitle returns a user-facing display title for a tool call.
func DisplayTitle(call model.ToolCall) string {
	if detail := DisplayDetail(call); detail != "" {
		prefix := map[string]string{
			"web_search": "WebSearch: ",
			"web_fetch":  "WebFetch: ",
			"load_skill": "LoadSkill: ",
			"todo_write": "Todo: ",
			"run_shell":  "Run: ",
		}[call.Name]
		return prefix + detail
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// DisplayDetail extracts the primary user-facing argument from a built-in tool call.
func DisplayDetail(call model.ToolCall) string {
	key := ""
	switch call.Name {
	case "web_search":
		key = "query"
	case "web_fetch":
		key = "url"
	case "load_skill":
		key = "name"
	case "todo_write":
		key = "todos"
	case "run_shell":
		key = "command"
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
		return fmt.Sprintf("%d items", len(items))
	}
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}
