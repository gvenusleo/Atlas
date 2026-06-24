package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
)

const (
	maxACPToolDiffBytes = 512 * 1024
	maxACPToolLocations = 50
	textProbeBytes      = 8 * 1024
)

// isFileTool 判断工具是否需要 ACP 文件展示增强。
func isFileTool(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file", "apply_patch", "glob", "grep":
		return true
	default:
		return false
	}
}

// runFileTool 为文件类工具补充 ACP filesystem 调用和展示 metadata。
func (a *Agent) runFileTool(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	switch call.Name {
	case "read_file":
		return a.runReadFileTool(ctx, sessionID, cwd, call, fallback)
	case "write_file":
		return a.runWriteFileTool(ctx, sessionID, cwd, call, fallback)
	case "edit_file":
		return a.runEditFileTool(ctx, sessionID, cwd, call, fallback)
	case "apply_patch":
		return a.runApplyPatchTool(ctx, cwd, call, fallback)
	case "glob":
		return a.runGlobTool(ctx, cwd, call, fallback)
	case "grep":
		return a.runGrepTool(ctx, cwd, call, fallback)
	default:
		return fallback(ctx, call)
	}
}

// runReadFileTool 优先使用客户端文件系统读取编辑器侧内容。
func (a *Agent) runReadFileTool(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	args, err := tool.ParseReadFileArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	path := absoluteToolPath(cwd, args.Path)
	metadata := model.ToolMetadata{Locations: []model.ToolLocation{{Path: path, Line: lineOrZero(args.Line)}}}
	if a.clientCapabilities.Fs.ReadTextFile && a.fileClient != nil && clientReadTextFileAllowed(path) {
		req := acpsdk.ReadTextFileRequest{SessionId: sessionID, Path: path}
		if args.Line > 0 {
			req.Line = &args.Line
		}
		if args.Limit > 0 {
			req.Limit = &args.Limit
		}
		resp, err := a.fileClient.ReadTextFile(ctx, req)
		if err == nil {
			return tool.RunResult{Content: resp.Content, Metadata: metadata}, nil
		}
	}
	return runLocalReadFileTool(ctx, call, path, args, metadata, fallback)
}

// runWriteFileTool 优先使用客户端文件系统写入，并生成 diff metadata。
func (a *Agent) runWriteFileTool(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	args, err := tool.ParseWriteFileArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	path := absoluteToolPath(cwd, args.Path)
	content := *args.Content
	metadata := model.ToolMetadata{Locations: []model.ToolLocation{{Path: path}}}
	if a.clientCapabilities.Fs.WriteTextFile && a.fileClient != nil && content != "" && clientWriteTextFileAllowed(path) {
		oldText := a.readToolOldText(ctx, sessionID, path)
		_, err := a.fileClient.WriteTextFile(ctx, acpsdk.WriteTextFileRequest{
			SessionId: sessionID,
			Path:      path,
			Content:   content,
		})
		if err == nil {
			metadata.Diff = acpToolDiff(path, oldText, content)
			return tool.RunResult{Content: "wrote " + path, Metadata: metadata}, nil
		}
	}
	return runLocalWriteFileTool(ctx, call, path, content, metadata, fallback)
}

// runLocalReadFileTool 使用 Atlas 本地工具读取文件并补充 ACP metadata。
func runLocalReadFileTool(ctx context.Context, call model.ToolCall, path string, args tool.ReadFileArgs, metadata model.ToolMetadata, fallback tool.RunFunc) (tool.RunResult, error) {
	result, err := fallback(ctx, toolCallWithArgs(call, tool.ReadFileArgs{
		Path:  path,
		Line:  args.Line,
		Limit: args.Limit,
	}))
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runLocalWriteFileTool 使用 Atlas 本地工具写入文件并生成 diff metadata。
func runLocalWriteFileTool(ctx context.Context, call model.ToolCall, path, content string, metadata model.ToolMetadata, fallback tool.RunFunc) (tool.RunResult, error) {
	oldText := readLocalToolOldText(path)
	result, err := fallback(ctx, toolCallWithArgs(call, tool.WriteFileArgs{
		Path:    path,
		Content: &content,
	}))
	if err == nil {
		metadata.Diff = acpToolDiff(path, oldText, content)
	}
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runEditFileTool 基于客户端当前文件内容应用 Atlas 的唯一块替换规则。
func (a *Agent) runEditFileTool(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	args, err := tool.ParseEditFileArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	path := absoluteToolPath(cwd, args.Path)
	metadata := model.ToolMetadata{Locations: []model.ToolLocation{{Path: path}}}
	if a.clientCapabilities.Fs.ReadTextFile && a.clientCapabilities.Fs.WriteTextFile && a.fileClient != nil && clientReadTextFileAllowed(path) {
		oldText, err := a.readClientToolText(ctx, sessionID, path)
		if err != nil {
			return runLocalEditFileTool(ctx, call, path, args, metadata, fallback)
		}
		newText, err := tool.ApplyEditFileContent(oldText, args.OldText, *args.NewText)
		if err != nil {
			return tool.RunResult{Metadata: metadata}, err
		}
		if newText != "" {
			_, err = a.fileClient.WriteTextFile(ctx, acpsdk.WriteTextFileRequest{
				SessionId: sessionID,
				Path:      path,
				Content:   newText,
			})
			if err == nil {
				metadata.Diff = acpToolDiff(path, &oldText, newText)
				return tool.RunResult{Content: "replaced 1 block in " + path, Metadata: metadata}, nil
			}
		}
	}
	return runLocalEditFileTool(ctx, call, path, args, metadata, fallback)
}

// runLocalEditFileTool 使用 Atlas 本地工具编辑文件并生成 diff metadata。
func runLocalEditFileTool(ctx context.Context, call model.ToolCall, path string, args tool.EditFileArgs, metadata model.ToolMetadata, fallback tool.RunFunc) (tool.RunResult, error) {
	oldText := readLocalToolOldText(path)
	result, err := fallback(ctx, toolCallWithArgs(call, tool.EditFileArgs{
		Path:    path,
		OldText: args.OldText,
		NewText: args.NewText,
	}))
	if err == nil {
		if newText, ok := readLocalToolText(path); ok {
			metadata.Diff = acpToolDiff(path, oldText, newText)
		}
	}
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runApplyPatchTool 应用 patch 并为 ACP 补充位置和单文件 diff metadata。
func (a *Agent) runApplyPatchTool(ctx context.Context, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	args, err := tool.ParseApplyPatchArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	paths := tool.ApplyPatchPaths(args.Patch, cwd)
	oldTexts := make(map[string]*string, len(paths))
	for _, path := range paths {
		oldTexts[path] = readLocalToolOldText(path)
	}
	result, err := fallback(ctx, call)
	metadata := model.ToolMetadata{Locations: toolLocationsForPaths(paths)}
	if err == nil && len(paths) == 1 {
		if newText, ok := readLocalToolText(paths[0]); ok {
			metadata.Diff = acpToolDiff(paths[0], oldTexts[paths[0]], newText)
		} else if oldTexts[paths[0]] != nil {
			metadata.Diff = acpToolDiff(paths[0], oldTexts[paths[0]], "")
		}
	}
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runGlobTool 保持本地查找实现，同时补充目录位置 metadata。
func (a *Agent) runGlobTool(ctx context.Context, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	path, ok := searchRootFromArguments(cwd, call.Arguments)
	if !ok {
		return fallback(ctx, call)
	}
	result, err := fallback(ctx, toolCallWithPath(call, path))
	result.Metadata = mergeToolMetadata(result.Metadata, model.ToolMetadata{
		Locations: []model.ToolLocation{{Path: path}},
	})
	return result, err
}

// runGrepTool 保持本地搜索实现，同时从匹配行提取跳转位置。
func (a *Agent) runGrepTool(ctx context.Context, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	path, ok := searchRootFromArguments(cwd, call.Arguments)
	if !ok {
		return fallback(ctx, call)
	}
	result, err := fallback(ctx, toolCallWithPath(call, path))
	locations := grepResultLocations(path, result.Content)
	if len(locations) == 0 {
		locations = []model.ToolLocation{{Path: path}}
	}
	result.Metadata = mergeToolMetadata(result.Metadata, model.ToolMetadata{Locations: locations})
	return result, err
}

// readToolOldText 读取旧内容用于 diff；读不到时按新文件处理。
func (a *Agent) readToolOldText(ctx context.Context, sessionID acpsdk.SessionId, path string) *string {
	if a.clientCapabilities.Fs.ReadTextFile && a.fileClient != nil && clientOldTextReadAllowed(path) {
		content, err := a.readClientToolText(ctx, sessionID, path)
		if err != nil {
			return nil
		}
		return &content
	}
	return readLocalToolOldText(path)
}

// readLocalToolOldText 读取本地旧内容，缺失时返回 nil 表示新文件。
func readLocalToolOldText(path string) *string {
	if !localTextFile(path) {
		return nil
	}
	content, ok := readLocalToolText(path)
	if !ok {
		return nil
	}
	return &content
}

// readClientToolText 从 ACP 客户端读取文件内容。
func (a *Agent) readClientToolText(ctx context.Context, sessionID acpsdk.SessionId, path string) (string, error) {
	resp, err := a.fileClient.ReadTextFile(ctx, acpsdk.ReadTextFileRequest{SessionId: sessionID, Path: path})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// readLocalToolText 从 Atlas 进程本地文件系统读取文件内容。
func readLocalToolText(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// clientReadTextFileAllowed 避免把目录或二进制文件交给 ACP 客户端读取。
func clientReadTextFileAllowed(path string) bool {
	return localTextFile(path)
}

// clientOldTextReadAllowed 只在路径不存在或本地确认是文本文件时读取客户端旧内容。
func clientOldTextReadAllowed(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return false
	}
	return localFileLooksText(path)
}

// clientWriteTextFileAllowed 避免请求客户端把目录或特殊文件当文本文件写入。
func clientWriteTextFileAllowed(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return false
	}
	return localFileLooksText(path)
}

// localTextFile 判断本地路径是否为可安全交给文本接口处理的普通文本文件。
func localTextFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return false
	}
	return localFileLooksText(path)
}

// localFileLooksText 采样文件开头，排除 NUL 字节和非 UTF-8 内容。
func localFileLooksText(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	buf := make([]byte, textProbeBytes)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	sample := buf[:n]
	return bytes.IndexByte(sample, 0) < 0 && utf8.Valid(sample)
}

// acpToolDiff 在合理大小内生成可持久化的 diff metadata。
func acpToolDiff(path string, oldText *string, newText string) *model.ToolDiff {
	oldLen := 0
	if oldText != nil {
		oldLen = len(*oldText)
	}
	if oldLen+len(newText) > maxACPToolDiffBytes {
		return nil
	}
	return &model.ToolDiff{Path: path, OldText: oldText, NewText: newText}
}

// mergeToolMetadata 保留已有 metadata，只填补缺失字段。
func mergeToolMetadata(base, extra model.ToolMetadata) model.ToolMetadata {
	if len(base.Locations) == 0 {
		base.Locations = extra.Locations
	}
	if base.Diff == nil {
		base.Diff = extra.Diff
	}
	return base
}

// absoluteToolPath 将工具路径解析到 ACP session cwd 下。
func absoluteToolPath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd != "" {
		return filepath.Clean(filepath.Join(cwd, path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// lineOrZero 将非正行号规整为空行号。
func lineOrZero(line int) int {
	if line > 0 {
		return line
	}
	return 0
}

// searchRootFromArguments 从工具参数中提取 path；未传 path 时使用 cwd。
func searchRootFromArguments(cwd, arguments string) (string, bool) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", false
	}
	if args.Path == "" {
		args.Path = cwd
	}
	return absoluteToolPath(cwd, args.Path), true
}

// toolCallWithPath 保留原始参数，只把 path 标准化为 session cwd 下的绝对路径。
func toolCallWithPath(call model.ToolCall, path string) model.ToolCall {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return call
	}
	args["path"] = path
	return toolCallWithArgs(call, args)
}

// toolCallWithArgs 用结构化参数替换工具调用的原始 JSON。
func toolCallWithArgs(call model.ToolCall, args any) model.ToolCall {
	data, err := json.Marshal(args)
	if err != nil {
		return call
	}
	call.Arguments = string(data)
	return call
}

// grepResultLocations 从 grep 的 file:line:content 输出中提取可跳转位置。
func grepResultLocations(root, output string) []model.ToolLocation {
	var locations []model.ToolLocation
	seen := map[string]struct{}{}
	locationRoot := root
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		locationRoot = filepath.Dir(root)
	}
	for _, line := range strings.Split(output, "\n") {
		if line == "" || strings.HasPrefix(line, "[") || line == "No matches found" {
			continue
		}
		pathPart, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		linePart, _, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		lineNumber, err := strconv.Atoi(linePart)
		if err != nil {
			continue
		}
		path := pathPart
		if !filepath.IsAbs(path) {
			path = filepath.Join(locationRoot, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		key := path + "\x00" + strconv.Itoa(lineNumber)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		locations = append(locations, model.ToolLocation{Path: path, Line: lineNumber})
		if len(locations) >= maxACPToolLocations {
			break
		}
	}
	return locations
}

func toolLocationsForPaths(paths []string) []model.ToolLocation {
	if len(paths) == 0 {
		return nil
	}
	locations := make([]model.ToolLocation, 0, len(paths))
	for _, path := range paths {
		locations = append(locations, model.ToolLocation{Path: path})
	}
	return locations
}
