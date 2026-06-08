package tool

import (
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
	defaultListFilesLimit = 100
	maxListFilesLimit     = 500
)

var errStopListFiles = errors.New("stop listing files")

// ListFiles 递归列出本地目录中的文件。
type ListFiles struct{}

// Definition 返回 list_files 的模型可见定义。
func (ListFiles) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "list_files",
		Description: "List files recursively under a local directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to list.",
				},
				"max_files": map[string]any{
					"type":        "integer",
					"description": "Maximum number of files to return.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Run 使用 JSON 参数中的 path 递归列出文件。
func (ListFiles) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path     string `json:"path"`
		MaxFiles int    `json:"max_files"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid list_files arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("list_files path is required")
	}
	limit := normalizeListFilesLimit(args.MaxFiles)
	return listFilePaths(ctx, args.Path, limit)
}

func listFilePaths(ctx context.Context, root string, limit int) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("list_files path is not a directory: %s", root)
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
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= limit {
			truncated = true
			return errStopListFiles
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

func normalizeListFilesLimit(limit int) int {
	if limit <= 0 {
		return defaultListFilesLimit
	}
	if limit > maxListFilesLimit {
		return maxListFilesLimit
	}
	return limit
}
