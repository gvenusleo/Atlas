package tool

import (
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestDisplayTitleUsesPrimaryArgument(t *testing.T) {
	tests := []struct {
		name string
		call model.ToolCall
		want string
	}{
		{name: "read file", call: model.ToolCall{Name: "read_file", Arguments: `{"path":"README.md"}`}, want: "Read: README.md"},
		{name: "write file", call: model.ToolCall{Name: "write_file", Arguments: `{"path":"notes.txt"}`}, want: "Write: notes.txt"},
		{name: "edit file", call: model.ToolCall{Name: "edit_file", Arguments: `{"path":"main.go"}`}, want: "Edit: main.go"},
		{name: "list files", call: model.ToolCall{Name: "list_files", Arguments: `{"path":"internal"}`}, want: "List: internal"},
		{name: "search text", call: model.ToolCall{Name: "search_text", Arguments: `{"query":"Tool:"}`}, want: "Search: Tool:"},
		{name: "web search", call: model.ToolCall{Name: "web_search", Arguments: `{"query":"atlas acp"}`}, want: "WebSearch: atlas acp"},
		{name: "web fetch", call: model.ToolCall{Name: "web_fetch", Arguments: `{"url":"https://example.com"}`}, want: "WebFetch: https://example.com"},
		{name: "run shell", call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"just check"}`}, want: "Run: just check"},
		{name: "invalid arguments", call: model.ToolCall{Name: "run_shell", Arguments: `{`}, want: "Tool: run_shell"},
		{name: "empty argument", call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"   "}`}, want: "Tool: run_shell"},
		{name: "unknown tool", call: model.ToolCall{Name: "custom", Arguments: `{"command":"ignored"}`}, want: "Tool: custom"},
		{name: "missing name", call: model.ToolCall{}, want: "Tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayTitle(tt.call); got != tt.want {
				t.Fatalf("DisplayTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
