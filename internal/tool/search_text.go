package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultSearchTextLimit = 100
	maxSearchTextLimit     = 500
)

var errStopSearchText = errors.New("stop searching text")

// SearchText 在本地目录中搜索字面量文本。
type SearchText struct{}

// Definition 返回 search_text 的模型可见定义。
func (SearchText) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "search_text",
		Description: "Search files under a local directory for literal text.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Literal text to find.",
				},
				"max_lines": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matching lines to return.",
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
	return searchText(ctx, args.Path, args.Query, limit)
}

func searchText(ctx context.Context, root, query string, limit int) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("search_text path is not a directory: %s", root)
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
		fileMatches, err := searchTextFile(root, path, query)
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
	result := strings.Join(matches, "\n")
	if truncated {
		if result != "" {
			result += "\n"
		}
		result += "[output truncated]"
	}
	return result, nil
}

func searchTextFile(root, path, query string) ([]string, error) {
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
		if strings.Contains(line, query) {
			matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNumber, line))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matches, nil
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
