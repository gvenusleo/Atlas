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
)

const maxReadBytes = 256 * 1024

// ReadFile reads a UTF-8 text file from disk.
type ReadFile struct{}

func (ReadFile) Definition() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read a text file from the local filesystem.",
		Parameters: objectSchema(map[string]any{
			"path": stringSchema("Path to read."),
		}, []string{"path"}),
	}
}

func (ReadFile) Execute(_ context.Context, raw json.RawMessage) Result {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err)
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return toolError("read file: %v", err)
	}
	truncated := ""
	if len(data) > maxReadBytes {
		data = data[:maxReadBytes]
		truncated = "\n\n[output truncated]"
	}
	return Result{Content: string(data) + truncated}
}

// WriteFile writes a full file, creating parent directories as needed.
type WriteFile struct{}

func (WriteFile) Definition() Definition {
	return Definition{
		Name:        "write_file",
		Description: "Write a complete text file, replacing existing content.",
		Parameters: objectSchema(map[string]any{
			"path":    stringSchema("Path to write."),
			"content": stringSchema("Complete file content."),
		}, []string{"path", "content"}),
	}
}

func (WriteFile) Execute(_ context.Context, raw json.RawMessage) Result {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(args.Path), 0o755); err != nil {
		return toolError("create parent directory: %v", err)
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return toolError("write file: %v", err)
	}
	return Result{Content: fmt.Sprintf("wrote %s", args.Path)}
}

// ListFiles lists files under a directory with a conservative result cap.
type ListFiles struct{}

func (ListFiles) Definition() Definition {
	return Definition{
		Name:        "list_files",
		Description: "List files recursively under a directory.",
		Parameters: objectSchema(map[string]any{
			"path":      stringSchema("Directory to list."),
			"max_files": numberSchema("Maximum number of files to return."),
		}, []string{"path"}),
	}
}

func (ListFiles) Execute(_ context.Context, raw json.RawMessage) Result {
	var args struct {
		Path     string `json:"path"`
		MaxFiles int    `json:"max_files"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err)
	}
	if args.MaxFiles <= 0 || args.MaxFiles > 500 {
		args.MaxFiles = 200
	}
	var files []string
	err := filepath.WalkDir(args.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if shouldSkipPath(entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		if len(files) >= args.MaxFiles {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return toolError("list files: %v", err)
	}
	sort.Strings(files)
	content := strings.Join(files, "\n")
	if errors.Is(err, errStopWalk) {
		content += "\n[output truncated]"
	}
	return Result{Content: content}
}

var errStopWalk = errors.New("stop walking")

func shouldSkipPath(entry fs.DirEntry) bool {
	name := entry.Name()
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", ".next", ".cache":
		return true
	default:
		return strings.HasPrefix(name, ".DS_Store")
	}
}
