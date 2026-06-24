package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultGrepLimit = 100
)

var errStopGrep = errors.New("stop grep")

// Grep 使用正则表达式搜索本地文本文件。
type Grep struct {
	CWD string
}

// Definition 返回 grep 的模型可见定义。
func (Grep) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "grep",
		Description: "Search local files with a regular expression.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regular expression to search for.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to search. Defaults to the session working directory.",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Optional glob pattern for files to search, such as *.go.",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

// Run 使用 JSON 参数中的 pattern 搜索匹配行。
func (g Grep) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseGrepArgs(arguments)
	if err != nil {
		return "", err
	}
	root := resolveToolPath(g.CWD, args.Path)
	return grep(ctx, root, args.Pattern, args.Include, defaultGrepLimit)
}

// GrepArgs 是 grep 的 JSON 参数。
type GrepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include"`
}

// ParseGrepArgs 解析并校验 grep 参数。
func ParseGrepArgs(arguments string) (GrepArgs, error) {
	var args GrepArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return GrepArgs{}, fmt.Errorf("invalid grep arguments: %w", err)
	}
	if args.Pattern == "" {
		return GrepArgs{}, fmt.Errorf("grep pattern is required")
	}
	return args, nil
}

func grep(ctx context.Context, root, pattern, include string, limit int) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid grep pattern: %w", err)
	}
	if err := validatePathGlob(include, "grep include"); err != nil {
		return "", err
	}
	if !info.IsDir() {
		matched, err := matchesIncludeGlob(filepath.Base(root), include, "grep include")
		if err != nil {
			return "", err
		}
		if !matched {
			return "No matches found", nil
		}
		matches, err := grepFile(filepath.Dir(root), root, re)
		if err != nil {
			return "", err
		}
		return formatGrepMatches(matches, false), nil
	}
	ignorer, err := loadGitIgnore(root, true)
	if err != nil {
		return "", err
	}

	var matches []string
	truncated := false
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if isVCSMetadataPath(path, root) {
				return filepath.SkipDir
			}
			if path != root {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				if isGitIgnored(ignorer, filepath.ToSlash(rel), true) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isGitIgnored(ignorer, rel, false) {
			return nil
		}
		matched, err := matchesIncludeGlob(rel, include, "grep include")
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		fileMatches, err := grepFile(root, path, re)
		if err != nil {
			return nil
		}
		for _, match := range fileMatches {
			matches = append(matches, match)
			if len(matches) >= limit {
				truncated = true
				return errStopGrep
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopGrep) {
		return "", err
	}

	sort.Strings(matches)
	return formatGrepMatches(matches, truncated), nil
}

func grepFile(root, filePath string, re *regexp.Regexp) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	var matches []string
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNumber, line))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}

func formatGrepMatches(matches []string, truncated bool) string {
	result := strings.Join(matches, "\n")
	if result == "" {
		result = "No matches found"
	}
	if truncated {
		if result != "" {
			result += "\n"
		}
		result += "[output truncated]"
	}
	return result
}

func isVCSMetadataPath(pathValue, root string) bool {
	rel, err := filepath.Rel(root, pathValue)
	if err != nil || rel == "." {
		return false
	}
	first, _, _ := strings.Cut(strings.Trim(filepath.ToSlash(rel), "/"), "/")
	switch first {
	case ".git", ".hg", ".svn":
		return true
	default:
		return false
	}
}

func validatePathGlob(pattern, label string) error {
	if pattern == "" {
		return nil
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("invalid %s glob: %w", label, err)
		}
	}
	return nil
}

func matchesPathGlob(rel, pattern, label string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	if err := validatePathGlob(pattern, label); err != nil {
		return false, err
	}
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	relParts := strings.Split(strings.Trim(rel, "/"), "/")
	if matchGlobSegments(patternParts, relParts) {
		return true, nil
	}
	return false, nil
}

func matchesIncludeGlob(rel, pattern, label string) (bool, error) {
	matched, err := matchesPathGlob(rel, pattern, label)
	if err != nil || matched || pattern == "" {
		return matched, err
	}
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(patternParts) == 1 {
		return matchGlobSegments(patternParts, []string{path.Base(rel)}), nil
	}
	return false, nil
}

func matchGlobSegments(pattern, rel []string) bool {
	if len(pattern) == 0 {
		return len(rel) == 0
	}
	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return true
		}
		for i := 0; i <= len(rel); i++ {
			if matchGlobSegments(pattern[1:], rel[i:]) {
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
	return matchGlobSegments(pattern[1:], rel[1:])
}

func resolveToolPath(cwd, pathValue string) string {
	if pathValue == "" {
		if cwd != "" {
			return cwd
		}
		return "."
	}
	if filepath.IsAbs(pathValue) || cwd == "" {
		return filepath.Clean(pathValue)
	}
	return filepath.Clean(filepath.Join(cwd, pathValue))
}
