package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// DisplayTitle returns a user-facing display title for a tool call.
// cwd is used to shorten file paths: if a file is under cwd, only the relative path is shown.
func DisplayTitle(call model.ToolCall, cwd string) string {
	if title := primaryDisplayTitle(call, cwd); title != "" {
		return title
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// primaryDisplayTitle extracts the most descriptive field from built-in tool parameters.
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

// isFileTool determines whether a tool uses a file path as its primary parameter.
func isFileTool(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file":
		return true
	}
	return false
}

// shortenPath shortens an absolute path to a cwd-relative path (if the file is under cwd).
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
