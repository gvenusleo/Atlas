package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultReadLineLimit = 2000
	maxReadLineLimit     = 2000
	readByteLimit        = 50 * 1024
)

// Read reads a bounded range from a UTF-8 text file.
type Read struct {
	CWD string
}

// ReadArgs describes the JSON parameters received by read.
type ReadArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// Definition returns the model-visible definition for read.
func (Read) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "read",
		Description: "Read a bounded range from a UTF-8 text file. Lines are 1-indexed; use offset and limit to continue through large files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, relative to the session working directory or absolute.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Optional 1-indexed line to start reading from. Defaults to 1.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxReadLineLimit,
					"description": "Optional maximum number of lines. Defaults to 2000 and cannot exceed 2000.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Run reads the requested range from a UTF-8 text file.
func (r Read) Run(ctx context.Context, arguments string) (string, error) {
	args, err := parseReadArgs(arguments)
	if err != nil {
		return "", err
	}
	path, err := resolveFilePath(r.CWD, args.Path)
	if err != nil {
		return "", fmt.Errorf("invalid read arguments: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", args.Path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("read %q: %w", args.Path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read %q: path is a directory", args.Path)
	}
	return readTextRange(ctx, file, args)
}

func parseReadArgs(arguments string) (ReadArgs, error) {
	var args ReadArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ReadArgs{}, fmt.Errorf("invalid read arguments: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &fields); err != nil {
		return ReadArgs{}, fmt.Errorf("invalid read arguments: %w", err)
	}
	if args.Path == "" {
		return ReadArgs{}, fmt.Errorf("invalid read arguments: path is required")
	}
	if _, provided := fields["offset"]; !provided {
		args.Offset = 1
	}
	if args.Offset < 1 {
		return ReadArgs{}, fmt.Errorf("invalid read arguments: offset must be at least 1")
	}
	if _, provided := fields["limit"]; !provided {
		args.Limit = defaultReadLineLimit
	}
	if args.Limit < 1 || args.Limit > maxReadLineLimit {
		return ReadArgs{}, fmt.Errorf("invalid read arguments: limit must be between 1 and %d", maxReadLineLimit)
	}
	return args, nil
}

func readTextRange(ctx context.Context, source io.Reader, args ReadArgs) (string, error) {
	reader := bufio.NewReaderSize(source, readByteLimit+1)
	var output bytes.Buffer
	lineNumber := 0
	readLines := 0
	truncated := false
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return "", fmt.Errorf("line %d exceeds the %d-byte read limit; use run_shell for byte-oriented inspection", lineNumber+1, readByteLimit)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read file: %w", err)
		}
		if len(line) == 0 && errors.Is(err, io.EOF) {
			break
		}
		lineNumber++
		if !utf8.Valid(line) {
			return "", fmt.Errorf("line %d is not valid UTF-8", lineNumber)
		}
		if lineNumber >= args.Offset {
			if readLines >= args.Limit {
				truncated = true
				break
			}
			if output.Len()+len(line) > readByteLimit {
				if readLines == 0 {
					return "", fmt.Errorf("line %d exceeds the %d-byte read limit; use run_shell for byte-oriented inspection", lineNumber, readByteLimit)
				}
				truncated = true
				break
			}
			output.Write(line)
			readLines++
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if readLines == 0 && lineNumber < args.Offset && !(lineNumber == 0 && args.Offset == 1) {
		return "", fmt.Errorf("offset %d is beyond end of file", args.Offset)
	}
	if truncated {
		if output.Len() > 0 && output.Bytes()[output.Len()-1] != '\n' {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "\n[Showing lines %d-%d. Use offset=%d to continue.]", args.Offset, args.Offset+readLines-1, args.Offset+readLines)
	}
	return output.String(), nil
}
