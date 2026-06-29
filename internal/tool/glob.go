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
	defaultGlobLimit = 100
)

var errStopGlob = errors.New("stop glob")

// Glob finds local files and directories matching a glob pattern.
type Glob struct {
	CWD string
}

// Definition returns the model-visible definition for glob.
func (Glob) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "glob",
		Description: "Find local files and directories matching a glob pattern.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match, such as **/*.go.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory to search. Defaults to the session working directory.",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

// Run finds paths using the pattern from the JSON parameters.
func (g Glob) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseGlobArgs(arguments)
	if err != nil {
		return "", err
	}
	root := resolveToolPath(g.CWD, args.Path)
	return globPaths(ctx, root, args.Pattern, defaultGlobLimit)
}

// GlobArgs is the JSON parameters for glob.
type GlobArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// ParseGlobArgs parses and validates glob parameters.
func ParseGlobArgs(arguments string) (GlobArgs, error) {
	var args GlobArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return GlobArgs{}, fmt.Errorf("invalid glob arguments: %w", err)
	}
	if args.Pattern == "" {
		return GlobArgs{}, fmt.Errorf("glob pattern is required")
	}
	if err := validatePathGlob(args.Pattern, "pattern"); err != nil {
		return GlobArgs{}, err
	}
	return args, nil
}

func globPaths(ctx context.Context, root, pattern string, limit int) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("glob path is not a directory: %s", root)
	}
	ignorer, err := loadGitIgnore(root, true)
	if err != nil {
		return "", err
	}

	var files []string
	truncated := false
	err = filepath.WalkDir(root, func(pathValue string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, pathValue)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)
		isDir := entry.IsDir()
		if isGlobSkippedPath(rel) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isGitIgnored(ignorer, rel, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		matched, err := matchesPathGlob(rel, pattern, "pattern")
		if err != nil {
			return err
		}
		if matched {
			item := rel
			if isDir {
				item += "/"
			}
			files = append(files, item)
			if len(files) >= limit {
				truncated = true
				return errStopGlob
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopGlob) {
		return "", err
	}

	sort.Strings(files)
	result := strings.Join(files, "\n")
	if result == "" {
		result = "No matches found"
	}
	if truncated {
		result += "\n[output truncated]"
	}
	return result, nil
}

func isGlobSkippedPath(rel string) bool {
	first, _, _ := strings.Cut(strings.Trim(rel, "/"), "/")
	switch first {
	case ".git", ".hg", ".svn":
		return true
	default:
		return false
	}
}
