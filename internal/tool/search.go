package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchText searches text files for a literal query.
type SearchText struct{}

func (SearchText) Definition() Definition {
	return Definition{
		Name:        "search_text",
		Description: "Search files under a directory for a literal text query.",
		Parameters: objectSchema(map[string]any{
			"path":      stringSchema("Directory to search."),
			"query":     stringSchema("Literal text to find."),
			"max_lines": numberSchema("Maximum matching lines to return."),
		}, []string{"path", "query"}),
	}
}

func (SearchText) Execute(ctx context.Context, raw json.RawMessage) Result {
	var args struct {
		Path     string `json:"path"`
		Query    string `json:"query"`
		MaxLines int    `json:"max_lines"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err)
	}
	if args.MaxLines <= 0 || args.MaxLines > 500 {
		args.MaxLines = 100
	}
	var matches []string
	err := filepath.WalkDir(args.Path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if shouldSkipPath(entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if strings.Contains(scanner.Text(), args.Query) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNo, scanner.Text()))
				if len(matches) >= args.MaxLines {
					return errStopWalk
				}
			}
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return toolError("search text: %v", err)
	}
	content := strings.Join(matches, "\n")
	if err == errStopWalk {
		content += "\n[output truncated]"
	}
	return Result{Content: content}
}
