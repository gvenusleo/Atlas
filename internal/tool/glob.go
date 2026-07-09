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

	"github.com/bmatcuk/doublestar/v4"
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

// globPaths matches files under root against pattern. It prefers ripgrep for
// speed and brace-expansion support, falling back to a doublestar walk when rg
// is unavailable or fails. Only files are returned.
func globPaths(ctx context.Context, root, pattern string, limit int) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("glob path is not a directory: %s", root)
	}

	if matches, ok := rgGlobFiles(ctx, pattern, root, limit); ok {
		sort.Strings(matches)
		return formatGlobMatches(matches, limit, false), nil
	}

	matches, truncated, err := globWithDoublestar(ctx, root, pattern, limit)
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	return formatGlobMatches(matches, limit, truncated), nil
}

// globWithDoublestar walks the tree and matches each file with doublestar,
// supporting brace expansion and ** that path.Match cannot.
func globWithDoublestar(ctx context.Context, root, pattern string, limit int) ([]string, bool, error) {
	ignorer, err := loadGitIgnore(root, true)
	if err != nil {
		return nil, false, err
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
		// Only files are returned, matching ripgrep --files behavior.
		if isDir {
			return nil
		}
		matched, mErr := doublestar.Match(pattern, rel)
		if mErr != nil {
			return nil
		}
		if matched {
			files = append(files, rel)
			if len(files) >= limit {
				truncated = true
				return errStopGlob
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopGlob) {
		return nil, false, err
	}
	return files, truncated, nil
}

func formatGlobMatches(matches []string, limit int, truncated bool) string {
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
		truncated = true
	}
	result := strings.Join(matches, "\n")
	if result == "" {
		result = "No matches found"
	}
	if truncated {
		result += "\n[output truncated]"
	}
	return result
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
