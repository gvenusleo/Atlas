package mcp

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"maps"
	"path"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gvenusleo/atlas/internal/agent"
)

const (
	// maxToolName 是 provider 允许的工具名长度。
	maxToolName = 64
	// namePrefix 与 nameSep 构成工具名 mcp__<服务器>__<工具>，见 toolName 与 SplitName。
	namePrefix = "mcp__"
	nameSep    = "__"
)

// agentTools 把服务器暴露的工具转成 agent 的工具。
func (s *server) agentTools() []agent.Tool {
	s.mu.Lock()
	entries := s.entries
	s.mu.Unlock()
	tools := make([]agent.Tool, len(entries))
	for i, e := range entries {
		tools[i] = agent.Tool{
			Name:        e.name,
			Description: cmp.Or(e.tool.Description, e.tool.Title, e.tool.Name),
			Parameters:  schemaOf(e.tool.InputSchema),
			Execute: func(
				ctx context.Context, _ string, params jsontext.Value,
			) (agent.ToolResult, error) {
				return s.call(ctx, e.tool.Name, params)
			},
		}
	}
	return tools
}

// visible 按配置的 include_tools 与 exclude_tools 判断工具是否暴露给模型。
func (s *server) visible(tool string) bool {
	match := func(patterns []string) bool {
		return slices.ContainsFunc(patterns, func(pattern string) bool {
			ok, _ := path.Match(pattern, tool)
			return ok
		})
	}
	return (len(s.cfg.IncludeTools) == 0 || match(s.cfg.IncludeTools)) && !match(s.cfg.ExcludeTools)
}

// entriesOf 给工具起暴露给模型的名字。按服务器给的名字排序后取名，重名只发生在名字里的
// 特殊字符被替换之后，后到的加哈希后缀，所以结果与服务器列出工具的顺序无关。
func (s *server) entriesOf(tools []*sdk.Tool) []entry {
	slices.SortFunc(tools, func(a, b *sdk.Tool) int { return cmp.Compare(a.Name, b.Name) })
	used := make(map[string]bool, len(tools))
	entries := make([]entry, len(tools))
	for i, tool := range tools {
		name := toolName(s.name, tool.Name)
		if used[name] {
			name = hashedName(s.name, tool.Name)
		}
		used[name] = true
		entries[i] = entry{name: name, tool: tool}
	}
	return entries
}

// toolName 是工具暴露给模型的名字：mcp__<服务器>__<工具>，工具名里 ASCII 字母、数字、_ 与 -
// 之外的字符换成 _。名字超过 64 个字符时改用 hashedName。
func toolName(server, tool string) string {
	if name := plainName(server, tool); len(name) <= maxToolName {
		return name
	}
	return hashedName(server, tool)
}

// plainName 是不考虑长度时的工具名。
func plainName(server, tool string) string {
	return namePrefix + server + nameSep + strings.Map(func(r rune) rune {
		if nameRune(r) {
			return r
		}
		return '_'
	}, tool)
}

// hashedName 是截断并加上固定哈希后缀的工具名，用于过长或重名的工具。
func hashedName(server, tool string) string {
	name := plainName(server, tool)
	sum := sha256.Sum256([]byte(server + "\x00" + tool))
	suffix := "_" + hex.EncodeToString(sum[:4])
	return name[:min(len(name), maxToolName-len(suffix))] + suffix
}

// SplitName 拆开工具名里的服务器名与工具名；不是 MCP 工具的名字时 ok 为假。工具名可能已经
// 被截断或替换过字符，只适合显示。
func SplitName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, namePrefix)
	if !ok {
		return "", "", false
	}
	return strings.Cut(rest, nameSep)
}

// schemaOf 返回工具的参数 schema：provider 要求是带 properties 的对象。
func schemaOf(input any) map[string]any {
	schema, _ := input.(map[string]any)
	schema = maps.Clone(schema)
	if schema == nil {
		schema = map[string]any{}
	}
	schema["type"] = "object"
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	return schema
}
