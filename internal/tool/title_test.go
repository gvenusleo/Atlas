package tool

import (
	"path/filepath"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestDisplayTitleUsesPrimaryArgument(t *testing.T) {
	tests := []struct {
		name string
		call model.ToolCall
		cwd  string
		want string
	}{
		{name: "apply patch", call: model.ToolCall{Name: "apply_patch", Arguments: `{"patch":"*** Begin Patch\n*** Add File: a.txt\n+new\n*** End Patch"}`}, cwd: "", want: "Patch: a.txt"},
		{name: "apply multi-file patch", call: model.ToolCall{Name: "apply_patch", Arguments: `{"patch":"*** Begin Patch\n*** Add File: a.txt\n+a\n*** Add File: b.txt\n+b\n*** End Patch"}`}, cwd: "", want: "Patch: 2 files"},
		{name: "web search", call: model.ToolCall{Name: "web_search", Arguments: `{"query":"atlas acp"}`}, cwd: "", want: "WebSearch: atlas acp"},
		{name: "web fetch", call: model.ToolCall{Name: "web_fetch", Arguments: `{"url":"https://example.com"}`}, cwd: "", want: "WebFetch: https://example.com"},
		{name: "load skill", call: model.ToolCall{Name: "load_skill", Arguments: `{"name":"think"}`}, cwd: "", want: "LoadSkill: think"},
		{name: "todo write", call: model.ToolCall{Name: "todo_write", Arguments: `{"todos":[{"content":"A","status":"pending"},{"content":"B","status":"completed"}]}`}, cwd: "", want: "Todo: 2 items"},
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
