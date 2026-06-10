package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	maxReadFileBytes       = 256 * 1024
	defaultReadFileLineMax = 2000
)

// ReadFile 读取本地文本文件内容。
type ReadFile struct{}

// Definition 返回 read_file 的模型可见定义。
func (ReadFile) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "read_file",
		Description: "Read a text file from the local filesystem.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to read.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Optional 1-based line number to start reading from.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Optional maximum number of lines to read.",
				},
			},
			"required": []string{"path"},
		},
	}
}

// Run 使用 JSON 参数中的 path、offset 和 limit 读取文件。
func (ReadFile) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid read_file arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("read_file path is required")
	}
	if args.Offset < 0 {
		return "", fmt.Errorf("read_file offset must be positive")
	}
	if args.Limit < 0 {
		return "", fmt.Errorf("read_file limit must be positive")
	}
	return readFileContent(ctx, args.Path, args.Offset, args.Limit)
}

func readFileContent(ctx context.Context, path string, offset, limit int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("read_file path is a directory: %s", path)
	}

	startLine := offset
	if startLine == 0 {
		startLine = 1
	}
	lineLimit := limit
	if lineLimit == 0 {
		lineLimit = defaultReadFileLineMax
	}
	return readFileLines(ctx, file, startLine, lineLimit)
}

func readFileLines(ctx context.Context, file *os.File, startLine, lineLimit int) (string, error) {
	reader := bufio.NewReader(file)
	var content []byte
	lineNumber := 0
	lastReturnedLine := 0
	truncatedByBytes := false
	hasMore := false

	for {
		line, lineTooLong, err := readLimitedLine(reader, maxReadFileBytes)
		if err == io.EOF && len(line) == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		lineNumber++
		if lineNumber < startLine {
			if err == io.EOF {
				break
			}
			continue
		}
		if lineNumber >= startLine+lineLimit {
			hasMore = true
			break
		}
		if lineTooLong || len(content)+len(line) > maxReadFileBytes {
			truncatedByBytes = true
			hasMore = true
			if lastReturnedLine == 0 {
				content = append(content, fmt.Sprintf("[Line %d exceeds %d byte output limit. Use run_shell for a narrower slice.]", lineNumber, maxReadFileBytes)...)
				lastReturnedLine = lineNumber
			}
			break
		}
		content = append(content, line...)
		lastReturnedLine = lineNumber
		if err == io.EOF {
			break
		}
	}

	if lineNumber == 0 && startLine > 1 {
		return "", fmt.Errorf("read_file offset %d is beyond end of file (0 lines total)", startLine)
	}
	if lineNumber == 0 {
		return "", nil
	}
	if startLine > lineNumber {
		return "", fmt.Errorf("read_file offset %d is beyond end of file (%d lines total)", startLine, lineNumber)
	}

	result := string(content)
	if lastReturnedLine == 0 {
		return result, nil
	}
	if hasMore {
		reason := ""
		if truncatedByBytes {
			reason = fmt.Sprintf(" (%d byte output limit)", maxReadFileBytes)
		}
		return result + fmt.Sprintf("\n\n[Showing lines %d-%d%s. Use offset=%d to continue.]", startLine, lastReturnedLine, reason, lastReturnedLine+1), nil
	}
	return result, nil
}

func readLimitedLine(reader *bufio.Reader, limit int) ([]byte, bool, error) {
	var line []byte
	lineTooLong := false
	for {
		part, err := reader.ReadSlice('\n')
		if !lineTooLong {
			if len(line)+len(part) > limit {
				lineTooLong = true
				remaining := limit - len(line)
				if remaining > 0 {
					line = append(line, part[:remaining]...)
				}
			} else {
				line = append(line, part...)
			}
		}
		switch err {
		case nil:
			return line, lineTooLong, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			return line, lineTooLong, io.EOF
		default:
			return nil, false, err
		}
	}
}
