package tool

import (
	"path/filepath"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestDisplayTitleUsesPrimaryArgument(t *testing.T) {
	cwd := "/Users/alice/project"
	tests := []struct {
		name string
		call model.ToolCall
		cwd  string
		want string
	}{
		{name: "read file relative", call: model.ToolCall{Name: "read_file", Arguments: `{"path":"README.md"}`}, cwd: "", want: "Read: README.md"},
		{name: "read file under cwd", call: model.ToolCall{Name: "read_file", Arguments: `{"path":"/Users/alice/project/src/main.go"}`}, cwd: cwd, want: "Read: src/main.go"},
		{name: "read file outside cwd", call: model.ToolCall{Name: "read_file", Arguments: `{"path":"/etc/hosts"}`}, cwd: cwd, want: "Read: /etc/hosts"},
		{name: "read file empty cwd", call: model.ToolCall{Name: "read_file", Arguments: `{"path":"/Users/alice/project/main.go"}`}, cwd: "", want: "Read: /Users/alice/project/main.go"},
		{name: "write file under cwd", call: model.ToolCall{Name: "write_file", Arguments: `{"path":"/Users/alice/project/notes.txt"}`}, cwd: cwd, want: "Write: notes.txt"},
		{name: "edit file under cwd", call: model.ToolCall{Name: "edit_file", Arguments: `{"path":"/Users/alice/project/main.go"}`}, cwd: cwd, want: "Edit: main.go"},
		{name: "apply patch", call: model.ToolCall{Name: "apply_patch", Arguments: `{"patch":"--- a/a.txt\n+++ b/a.txt"}`}, cwd: "", want: "Patch: --- a/a.txt"},
		{name: "glob", call: model.ToolCall{Name: "glob", Arguments: `{"pattern":"**/*.go"}`}, cwd: "", want: "Glob: **/*.go"},
		{name: "grep", call: model.ToolCall{Name: "grep", Arguments: `{"pattern":"Tool:"}`}, cwd: "", want: "Grep: Tool:"},
		{name: "web search", call: model.ToolCall{Name: "web_search", Arguments: `{"query":"atlas acp"}`}, cwd: "", want: "WebSearch: atlas acp"},
		{name: "web fetch", call: model.ToolCall{Name: "web_fetch", Arguments: `{"url":"https://example.com"}`}, cwd: "", want: "WebFetch: https://example.com"},
		{name: "run shell", call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"just check"}`}, cwd: "", want: "Run: just check"},
		{name: "invalid arguments", call: model.ToolCall{Name: "run_shell", Arguments: `{`}, cwd: "", want: "Tool: run_shell"},
		{name: "empty argument", call: model.ToolCall{Name: "run_shell", Arguments: `{"command":"   "}`}, cwd: "", want: "Tool: run_shell"},
		{name: "unknown tool", call: model.ToolCall{Name: "custom", Arguments: `{"command":"ignored"}`}, cwd: "", want: "Tool: custom"},
		{name: "missing name", call: model.ToolCall{}, cwd: "", want: "Tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayTitle(tt.call, tt.cwd); got != tt.want {
				t.Fatalf("DisplayTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	cwd := "/Users/alice/project"
	tests := []struct {
		name string
		path string
		cwd  string
		want string
	}{
		{name: "under cwd", path: filepath.Join(cwd, "src/main.go"), cwd: cwd, want: "src/main.go"},
		{name: "cwd itself", path: cwd, cwd: cwd, want: "."},
		{name: "outside cwd", path: "/etc/hosts", cwd: cwd, want: "/etc/hosts"},
		{name: "sibling dir", path: "/Users/alice/other/x.go", cwd: cwd, want: "/Users/alice/other/x.go"},
		{name: "empty cwd", path: "/Users/alice/project/main.go", cwd: "", want: "/Users/alice/project/main.go"},
		{name: "relative path", path: "src/main.go", cwd: cwd, want: "src/main.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenPath(tt.path, tt.cwd); got != tt.want {
				t.Fatalf("shortenPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
