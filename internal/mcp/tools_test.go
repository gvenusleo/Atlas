package mcp

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// 工具名会随消息存进会话，命名规则要稳定：特殊字符换成 _，过长的截断并加固定的哈希后缀，
// 换了字符后重名的，后到的加哈希，与服务器列出工具的顺序无关；名字能拆回服务器与工具。
func TestToolNames(t *testing.T) {
	if got := toolName("git", "search.code"); got != "mcp__git__search_code" {
		t.Errorf("toolName = %q，期望 mcp__git__search_code", got)
	}
	if server, tool, ok := SplitName("mcp__git__search_code"); !ok || server != "git" ||
		tool != "search_code" {
		t.Errorf("SplitName = %q、%q、%v", server, tool, ok)
	}
	if _, _, ok := SplitName("read"); ok {
		t.Error("内置工具的名字不应被当作 MCP 工具")
	}
	long := strings.Repeat("x", 100)
	got := toolName("git", long)
	if len(got) != maxToolName || got != hashedName("git", long) ||
		!strings.HasPrefix(got, "mcp__git__xxx") {
		t.Errorf("过长的名字 = %q，期望长度 %d、前缀不变且每次相同", got, maxToolName)
	}

	s := &server{name: "git"}
	for _, order := range [][]string{{"a.b", "a_b"}, {"a_b", "a.b"}} {
		var tools []*sdk.Tool
		for _, name := range order {
			tools = append(tools, &sdk.Tool{Name: name})
		}
		entries := s.entriesOf(tools)
		if entries[0].tool.Name != "a.b" || entries[0].name != "mcp__git__a_b" ||
			entries[1].tool.Name != "a_b" || entries[1].name != hashedName("git", "a_b") {
			t.Errorf("顺序 %v 取名 = %+v，期望 a.b 用 mcp__git__a_b，a_b 加哈希", order, entries)
		}
	}
}
