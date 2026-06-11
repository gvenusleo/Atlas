package tool

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// gitIgnore 保存一个 .gitignore 文件解析出的规则。
type gitIgnore struct {
	patterns []gitIgnorePattern
}

// gitIgnorePattern 表示一行有效的 .gitignore 规则。
type gitIgnorePattern struct {
	pattern  string
	segments []string
	negated  bool
	dirOnly  bool
	hasSlash bool
}

// loadGitIgnore 按需读取待列出目录下的 .gitignore。
func loadGitIgnore(root string, respect bool) (*gitIgnore, error) {
	if !respect {
		return nil, nil
	}
	ignorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(ignorePath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return compileGitIgnoreFile(ignorePath)
}

// isGitIgnored 判断相对路径是否被已加载的 .gitignore 忽略。
func isGitIgnored(ignorer *gitIgnore, rel string, isDir bool) bool {
	return ignorer != nil && ignorer.matches(rel, isDir)
}

// compileGitIgnoreFile 读取并解析一个 .gitignore 文件。
func compileGitIgnoreFile(filename string) (*gitIgnore, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return compileGitIgnoreLines(lines)
}

// compileGitIgnoreLines 解析 .gitignore 行，保留后出现规则覆盖前面规则的顺序。
func compileGitIgnoreLines(lines []string) (*gitIgnore, error) {
	ignorer := &gitIgnore{}
	for i, line := range lines {
		pattern, ok, err := parseGitIgnoreLine(line)
		if err != nil {
			return nil, fmt.Errorf("invalid .gitignore line %d: %w", i+1, err)
		}
		if ok {
			ignorer.patterns = append(ignorer.patterns, pattern)
		}
	}
	return ignorer, nil
}

// parseGitIgnoreLine 将单行规则转成匹配结构，空行和注释返回 ok=false。
func parseGitIgnoreLine(line string) (gitIgnorePattern, bool, error) {
	line = strings.TrimSuffix(line, "\r")
	line = trimGitIgnoreTrailingSpaces(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return gitIgnorePattern{}, false, nil
	}

	negated := false
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "!") {
		negated = true
		line = line[1:]
	}

	dirOnly := strings.HasSuffix(line, "/")
	if dirOnly {
		line = strings.TrimRight(line, "/")
	}
	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return gitIgnorePattern{}, false, nil
	}

	segments := strings.Split(line, "/")
	for _, segment := range segments {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return gitIgnorePattern{}, false, err
		}
	}

	return gitIgnorePattern{
		pattern:  line,
		segments: segments,
		negated:  negated,
		dirOnly:  dirOnly,
		hasSlash: anchored || strings.Contains(line, "/"),
	}, true, nil
}

// trimGitIgnoreTrailingSpaces 丢弃未转义的行尾空格。
func trimGitIgnoreTrailingSpaces(line string) string {
	for strings.HasSuffix(line, " ") {
		backslashes := 0
		for i := len(line) - 2; i >= 0 && line[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			return line[:len(line)-2] + " "
		}
		line = line[:len(line)-1]
	}
	return line
}

// matches 按 .gitignore 顺序判断相对路径最终是否被忽略。
func (g *gitIgnore) matches(rel string, isDir bool) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == "." {
		return false
	}

	ignored := false
	for _, pattern := range g.patterns {
		if pattern.matches(rel, isDir) {
			ignored = !pattern.negated
		}
	}
	return ignored
}

// matches 判断单条规则是否命中相对路径。
func (p gitIgnorePattern) matches(rel string, isDir bool) bool {
	if p.hasSlash {
		return p.matchesPath(rel, isDir)
	}
	return p.matchesName(rel, isDir)
}

// matchesName 处理不含斜杠的规则，这类规则可命中任意层级的路径段。
func (p gitIgnorePattern) matchesName(rel string, isDir bool) bool {
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		matched, err := path.Match(p.pattern, segment)
		if err != nil || !matched {
			continue
		}
		if p.dirOnly && i == len(segments)-1 && !isDir {
			return false
		}
		return true
	}
	return false
}

// matchesPath 处理根相对规则，并允许已命中的目录规则继续覆盖子路径。
func (p gitIgnorePattern) matchesPath(rel string, isDir bool) bool {
	segments := strings.Split(rel, "/")
	for end := 1; end <= len(segments); end++ {
		if p.dirOnly && end == len(segments) && !isDir {
			continue
		}
		if matchGitIgnoreSegments(p.segments, segments[:end]) {
			return true
		}
	}
	return false
}

// matchGitIgnoreSegments 用路径段匹配规则，其中 ** 可匹配零个或多个路径段。
func matchGitIgnoreSegments(pattern, rel []string) bool {
	if len(pattern) == 0 {
		return len(rel) == 0
	}
	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return len(rel) > 0
		}
		for i := 0; i <= len(rel); i++ {
			if matchGitIgnoreSegments(pattern[1:], rel[i:]) {
				return true
			}
		}
		return false
	}
	if len(rel) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], rel[0])
	if err != nil || !matched {
		return false
	}
	return matchGitIgnoreSegments(pattern[1:], rel[1:])
}
