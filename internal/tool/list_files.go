package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultListFilesLimit = 100
	maxListFilesLimit     = 500
)

var errStopListFiles = errors.New("stop listing files")

// ListFiles 列出本地目录中的文件和子目录。
type ListFiles struct{}

// Definition 返回 list_files 的模型可见定义。
func (ListFiles) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "list_files",
		Description: "List files and directories under a local directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to list.",
				},
				"max_files": map[string]any{
					"type":        "integer",
					"description": "Maximum number of file or directory entries to return.",
				},
				"depth": map[string]any{
					"type":        "integer",
					"description": "Maximum directory depth to include. 0 lists only direct children.",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "Optional glob pattern for returned paths.",
				},
				"respect_gitignore": map[string]any{
					"type":        "boolean",
					"description": "Defaults to true. Set to false to include files ignored by .gitignore.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Run 使用 JSON 参数中的 path 列出目录内容。
func (ListFiles) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path             string `json:"path"`
		MaxFiles         int    `json:"max_files"`
		Depth            int    `json:"depth"`
		Include          string `json:"include"`
		RespectGitignore *bool  `json:"respect_gitignore"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid list_files arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("list_files path is required")
	}
	respectGitignore := true
	if args.RespectGitignore != nil {
		respectGitignore = *args.RespectGitignore
	}
	limit := normalizeListFilesLimit(args.MaxFiles)
	depth := normalizeListFilesDepth(args.Depth)
	return listFilePaths(ctx, args.Path, limit, depth, args.Include, respectGitignore)
}

func listFilePaths(ctx context.Context, root string, limit, maxDepth int, include string, respectGitignore bool) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("list_files path is not a directory: %s", root)
	}

	ignorer, err := loadGitIgnore(root, respectGitignore)
	if err != nil {
		return "", err
	}
	if err := validateListInclude(include); err != nil {
		return "", err
	}

	var files []string
	truncated := false
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)
		isDir := entry.IsDir()
		if isGitIgnored(ignorer, rel, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		depth := listPathDepth(rel)
		if depth > maxDepth {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		matched, err := matchesListInclude(rel, include)
		if err != nil {
			return err
		}
		if matched {
			if isDir {
				rel += "/"
			}
			files = append(files, rel)
			if len(files) >= limit {
				truncated = true
				return errStopListFiles
			}
		}
		if isDir && depth >= maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopListFiles) {
		return "", err
	}

	sort.Strings(files)
	result := strings.Join(files, "\n")
	if truncated {
		if result != "" {
			result += "\n"
		}
		result += "[output truncated]"
	}
	return result, nil
}

// validateListInclude 在遍历前校验 include glob，避免空目录吞掉坏参数。
func validateListInclude(include string) error {
	if include == "" {
		return nil
	}
	if _, err := path.Match(include, ""); err != nil {
		return fmt.Errorf("invalid list_files include glob: %w", err)
	}
	return nil
}

// matchesListInclude 用相对路径和文件名两种方式匹配 include glob。
func matchesListInclude(rel, include string) (bool, error) {
	if include == "" {
		return true, nil
	}
	if matched, err := path.Match(include, rel); err != nil {
		return false, fmt.Errorf("invalid list_files include glob: %w", err)
	} else if matched {
		return true, nil
	}
	matched, err := path.Match(include, path.Base(rel))
	if err != nil {
		return false, fmt.Errorf("invalid list_files include glob: %w", err)
	}
	return matched, nil
}

// listPathDepth 返回相对路径距离根目录的层级。
func listPathDepth(rel string) int {
	return strings.Count(strings.Trim(rel, "/"), "/")
}

func normalizeListFilesLimit(limit int) int {
	if limit <= 0 {
		return defaultListFilesLimit
	}
	if limit > maxListFilesLimit {
		return maxListFilesLimit
	}
	return limit
}

// normalizeListFilesDepth 将非法深度收敛到默认的当前目录层级。
func normalizeListFilesDepth(depth int) int {
	if depth < 0 {
		return 0
	}
	return depth
}
