package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// DisplayTitle returns a user-facing display title for a tool call.
func DisplayTitle(call model.ToolCall) string {
	if title := primaryDisplayTitle(call); title != "" {
		return title
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// primaryDisplayTitle extracts the most descriptive field from built-in tool parameters.
func primaryDisplayTitle(call model.ToolCall) string {
	prefix, key := "", ""
	switch call.Name {
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
	return prefix + value
}
