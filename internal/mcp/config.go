package mcp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gvenusleo/atlas/internal/config"
)

// maxName 限制服务器名的长度：它是工具名的一部分，工具名整体不能超过 64 个字符。
const maxName = 32

// validate 检查一项配置。服务器名只含字母、数字、_ 与 -，且不含 __：工具名用 __ 分隔
// 服务器名与工具名，见 SplitName。
func validate(name string, c config.MCPServer) error {
	if name == "" || len(name) > maxName || strings.Contains(name, "__") ||
		strings.ContainsFunc(name, func(r rune) bool { return !nameRune(r) }) {
		return fmt.Errorf("服务器名 %q 无效：只能含字母、数字、_ 与 -，不含 __，最多 %d 个字符",
			name, maxName)
	}
	switch c.Type {
	case "", "stdio", "http", "streamable-http":
	case "sse":
		return errors.New("不支持 SSE；服务器通常也提供 Streamable HTTP，地址常是 /mcp 而不是 /sse")
	default:
		return fmt.Errorf("type = %q 无效，仅支持 stdio、http、streamable-http", c.Type)
	}
	switch hasCommand, hasURL := c.Command != "", c.URL != ""; {
	case hasCommand == hasURL:
		return errors.New("command 与 url 必须二选一")
	case c.Type == "stdio" && !hasCommand:
		return errors.New("type 为 stdio 时需要 command")
	case c.Type != "" && c.Type != "stdio" && !hasURL:
		return fmt.Errorf("type 为 %s 时需要 url", c.Type)
	case hasURL:
		u, err := url.Parse(c.URL)
		if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("url %q 不是 http 或 https 地址", c.URL)
		}
	}
	if c.Timeout < 0 {
		return errors.New("timeout 不能为负")
	}
	for _, pattern := range slices.Concat(c.IncludeTools, c.ExcludeTools) {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("工具筛选 %q 无效: %w", pattern, err)
		}
	}
	return nil
}

// nameRune 报告 r 是否可以出现在服务器名与工具名里：ASCII 字母、数字、_ 与 -。
func nameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		r == '_' || r == '-'
}

// expandHome 把路径开头的 ~/ 换成主目录；command、args 与 cwd 不经过 shell，要自己展开。
func expandHome(value string) (string, error) {
	if value != "~" && !strings.HasPrefix(value, "~/") {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("定位主目录失败: %w", err)
	}
	return filepath.Join(home, value[1:]), nil
}
