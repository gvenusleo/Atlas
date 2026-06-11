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
	defaultSearchTextLimit = 100
	maxSearchTextLimit     = 500
)

var errStopSearchText = errors.New("stop searching text")

// SearchText 在本地文件或目录中搜索文本。
type SearchText struct{}

// Definition 返回 search_text 的模型可见定义。
func (SearchText) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "search_text",
		Description: "Search files under a local path for literal text or a regular expression.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to search.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Literal text or regular expression to find.",
				},
				"max_lines": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matching lines to return.",
				},
				"regex": map[string]any{
					"type":        "boolean",
					"description": "When true, treat query as a Go regular expression.",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Optional glob pattern for files to search.",
				},
			},
			"required": []string{"path", "query"},
		},
	}
}

// Run 使用 JSON 参数中的 path 和 query 搜索匹配行。
func (SearchText) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path     string `json:"path"`
		Query    string `json:"query"`
		MaxLines int    `json:"max_lines"`
		Regex    bool   `json:"regex"`
		Include  string `json:"include"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid search_text arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("search_text path is required")
	}
	if args.Query == "" {
		return "", fmt.Errorf("search_text query is required")
	}
	limit := normalizeSearchTextLimit(args.MaxLines)
	return searchText(ctx, args.Path, args.Query, limit, args.Regex, args.Include)
}

func searchText(ctx context.Context, root, query string, limit int, useRegex bool, include string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}

	matcher, err := newTextMatcher(query, useRegex)
	if err != nil {
		return "", err
	}
	if err := validateSearchInclude(include); err != nil {
		return "", err
	}
	if !info.IsDir() {
		matched, err := matchesSearchInclude(filepath.Base(root), include)
		if err != nil {
			return "", err
		}
		if !matched {
			return "No matches found", nil
		}
		matches, err := searchTextFile(filepath.Dir(root), root, matcher)
		if err != nil {
			return "", err
		}
		return formatSearchTextMatches(matches, false), nil
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
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		matched, err := matchesSearchInclude(filepath.ToSlash(rel), include)
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		fileMatches, err := searchTextFile(root, path, matcher)
		if err != nil {
			return nil
		}
		for _, match := range fileMatches {
			matches = append(matches, match)
			if len(matches) >= limit {
				truncated = true
				return errStopSearchText
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopSearchText) {
		return "", err
	}

	sort.Strings(matches)
	return formatSearchTextMatches(matches, truncated), nil
}

type textMatcher func(string) bool

// newTextMatcher 创建字面量或正则匹配器。
func newTextMatcher(query string, useRegex bool) (textMatcher, error) {
	if !useRegex {
		return func(line string) bool {
			return strings.Contains(line, query)
		}, nil
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid search_text regex: %w", err)
	}
	return re.MatchString, nil
}

// searchTextFile 返回单个文件中的匹配行。
func searchTextFile(root, path string, matcher textMatcher) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	rel, err := filepath.Rel(root, path)
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
		if matcher(line) {
			matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNumber, line))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}

// validateSearchInclude 在遍历前校验 include glob，避免空目录吞掉坏参数。
func validateSearchInclude(include string) error {
	if include == "" {
		return nil
	}
	if _, err := path.Match(include, ""); err != nil {
		return fmt.Errorf("invalid search_text include glob: %w", err)
	}
	return nil
}

// matchesSearchInclude 用相对路径和文件名两种方式匹配 include glob。
func matchesSearchInclude(rel, include string) (bool, error) {
	if include == "" {
		return true, nil
	}
	if matched, err := path.Match(include, rel); err != nil {
		return false, fmt.Errorf("invalid search_text include glob: %w", err)
	} else if matched {
		return true, nil
	}
	matched, err := path.Match(include, path.Base(rel))
	if err != nil {
		return false, fmt.Errorf("invalid search_text include glob: %w", err)
	}
	return matched, nil
}

// formatSearchTextMatches 统一格式化匹配结果、空结果和截断提示。
func formatSearchTextMatches(matches []string, truncated bool) string {
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

func normalizeSearchTextLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchTextLimit
	}
	if limit > maxSearchTextLimit {
		return maxSearchTextLimit
	}
	return limit
}
