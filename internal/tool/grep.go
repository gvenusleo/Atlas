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

	"github.com/bmatcuk/doublestar/v4"
	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultGrepLimit = 100
)

var errStopGrep = errors.New("stop grep")

// Grep searches local text files using a regular expression.
type Grep struct {
	CWD string
}

// Definition returns the model-visible definition for grep.
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

// Run searches for matching lines using the pattern from the JSON parameters.
func (g Grep) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseGrepArgs(arguments)
	if err != nil {
		return "", err
	}
	root := resolveToolPath(g.CWD, args.Path)
	return grep(ctx, root, args.Pattern, args.Include, defaultGrepLimit)
}

// GrepArgs is the JSON parameters for grep.
type GrepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include"`
}

// ParseGrepArgs parses and validates grep parameters.
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

// grep searches root for pattern. Directories prefer ripgrep for speed and
// brace-expansion support in the include filter, falling back to a Go regexp
// walk when rg is unavailable or fails. Single files always use the Go path.
// Output is "rel:line:content" lines, sorted, so ACP can parse locations.
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
		if !matchIncludePattern(include, filepath.Base(root)) {
			return "No matches found", nil
		}
		matches, err := grepFile(filepath.Dir(root), root, re)
		if err != nil {
			return "", err
		}
		return formatGrepMatches(matches, limit, false), nil
	}

	if rgMatches, ok := rgGrepSearch(ctx, pattern, root, include, limit); ok {
		lines := make([]string, 0, len(rgMatches))
		for _, m := range rgMatches {
			lines = append(lines, fmt.Sprintf("%s:%d:%s", m.rel, m.line, m.text))
		}
		sort.Strings(lines)
		return formatGrepMatches(lines, limit, false), nil
	}

	return grepWithRegex(ctx, root, re, include, limit)
}

// grepWithRegex walks root and matches each file with the compiled regexp,
// used as the fallback when ripgrep is unavailable.
func grepWithRegex(ctx context.Context, root string, re *regexp.Regexp, include string, limit int) (string, error) {
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
		if !matchIncludePattern(include, rel) {
			return nil
		}
		fileMatches, err := grepFile(root, path, re)
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		if len(matches) >= limit {
			truncated = true
			return errStopGrep
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopGrep) {
		return "", err
	}
	sort.Strings(matches)
	return formatGrepMatches(matches, limit, truncated), nil
}

// matchIncludePattern reports whether rel matches the include glob. A single
// segment pattern (e.g. "*.go") matches by basename at any level, matching
// ripgrep's --glob behavior without a leading slash; a multi-segment pattern
// (e.g. "src/*.go") matches the full relative path.
func matchIncludePattern(include, rel string) bool {
	if include == "" {
		return true
	}
	if matched, err := doublestar.Match(include, rel); err == nil && matched {
		return true
	}
	if !strings.Contains(include, "/") {
		if matched, err := doublestar.Match(include, filepath.Base(rel)); err == nil && matched {
			return true
		}
	}
	return false
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

func formatGrepMatches(matches []string, limit int, truncated bool) string {
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

// validatePathGlob checks each segment of a glob pattern for basic syntax
// validity via path.Match. Brace expansion ({js,json}) is treated as literal
// by path.Match and therefore passes; doublestar handles it during matching.
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
