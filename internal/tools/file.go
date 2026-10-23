// Package tools 提供 Agent 使用的文件与命令工具，相对路径均以会话工作目录为基准。
package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// maxFileBytes 是 read 与 edit 处理的单个文件大小上限。
const maxFileBytes = 10 << 20

// resolvePath 将相对路径解析到工作目录下并清理。
func resolvePath(cwd, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// readRegularFile 读取不超过 maxFileBytes 的普通文件；取消后不返回内容。
func readRegularFile(ctx context.Context, path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s 检查失败: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s 不是普通文件", path)
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s 文件大小超过 10 MiB 限制", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s 打开失败: %w", path, err)
	}
	defer file.Close()

	// 文件可能在 Stat 后增长，因此实际读取也必须有上限。
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s 读取失败: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%s 文件大小超过 10 MiB 限制", path)
	}
	return data, nil
}

// validText 判断内容是否为不含 NUL 的 UTF-8 文本。
func validText(data []byte) bool {
	return utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// ReadText 读取一个 UTF-8 文本文件，供界面打开文件：info 是读取之前文件的状态，界面据此发现
// 之后的外部修改，读到它之后文件才出错时也会返回。不是普通文件、超过大小上限、不是 UTF-8
// 文本时返回错误。
func ReadText(ctx context.Context, path string) (text string, info fs.FileInfo, err error) {
	info, err = os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("%s 检查失败: %w", path, err)
	}
	data, err := readRegularFile(ctx, path)
	if err != nil {
		return "", info, err
	}
	if !validText(data) {
		return "", info, fmt.Errorf("%s 不是 UTF-8 文本", path)
	}
	return string(data), info, nil
}
