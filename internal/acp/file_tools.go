package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
)

const (
	maxACPToolDiffBytes = 512 * 1024
	maxACPToolLocations = 50
)

// isFileTool 判断工具是否需要 ACP 文件展示增强。
func isFileTool(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file", "list_files", "search_text":
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
	case "list_files":
		return a.runListFilesTool(ctx, cwd, call, fallback)
	case "search_text":
		return a.runSearchTextTool(ctx, cwd, call, fallback)
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
	metadata := model.ToolMetadata{Locations: []model.ToolLocation{{Path: path, Line: lineOrZero(args.Offset)}}}
	if a.clientCapabilities.Fs.ReadTextFile && a.fileClient != nil {
		req := acpsdk.ReadTextFileRequest{SessionId: sessionID, Path: path}
		if args.Offset > 0 {
			req.Line = &args.Offset
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
	if a.clientCapabilities.Fs.WriteTextFile && a.fileClient != nil && content != "" {
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
		Path:   path,
		Offset: args.Offset,
		Limit:  args.Limit,
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
	if a.clientCapabilities.Fs.ReadTextFile && a.clientCapabilities.Fs.WriteTextFile && a.fileClient != nil {
		oldText, err := a.readClientToolText(ctx, sessionID, path)
		if err != nil {
			return runLocalEditFileTool(ctx, call, path, args, metadata, fallback)
		}
		newText, count, err := tool.ApplyEditFileContent(oldText, args.Edits)
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
				return tool.RunResult{Content: fmt.Sprintf("replaced %d blocks in %s", count, path), Metadata: metadata}, nil
			}
		}
	}
	return runLocalEditFileTool(ctx, call, path, args, metadata, fallback)
}

// runLocalEditFileTool 使用 Atlas 本地工具编辑文件并生成 diff metadata。
func runLocalEditFileTool(ctx context.Context, call model.ToolCall, path string, args tool.EditFileArgs, metadata model.ToolMetadata, fallback tool.RunFunc) (tool.RunResult, error) {
	oldText := readLocalToolOldText(path)
	result, err := fallback(ctx, toolCallWithArgs(call, tool.EditFileArgs{
		Path:  path,
		Edits: args.Edits,
	}))
	if err == nil {
		if newText, ok := readLocalToolText(path); ok {
			metadata.Diff = acpToolDiff(path, oldText, newText)
		}
	}
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runListFilesTool 保持本地列目录实现，同时补充目录位置 metadata。
func (a *Agent) runListFilesTool(ctx context.Context, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	path, ok := toolPathFromArguments(cwd, call.Arguments)
	if !ok {
		return fallback(ctx, call)
	}
	result, err := fallback(ctx, toolCallWithPath(call, path))
	result.Metadata = mergeToolMetadata(result.Metadata, model.ToolMetadata{
		Locations: []model.ToolLocation{{Path: path}},
	})
	return result, err
}

// runSearchTextTool 保持本地搜索实现，同时从匹配行提取跳转位置。
func (a *Agent) runSearchTextTool(ctx context.Context, cwd string, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
	path, ok := toolPathFromArguments(cwd, call.Arguments)
	if !ok {
		return fallback(ctx, call)
	}
	result, err := fallback(ctx, toolCallWithPath(call, path))
	locations := searchResultLocations(path, result.Content)
	if len(locations) == 0 {
		locations = []model.ToolLocation{{Path: path}}
	}
	result.Metadata = mergeToolMetadata(result.Metadata, model.ToolMetadata{Locations: locations})
	return result, err
}

// readToolOldText 读取旧内容用于 diff；读不到时按新文件处理。
func (a *Agent) readToolOldText(ctx context.Context, sessionID acpsdk.SessionId, path string) *string {
	if a.clientCapabilities.Fs.ReadTextFile && a.fileClient != nil {
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

// toolPathFromArguments 从工具参数中提取并标准化 path。
func toolPathFromArguments(cwd, arguments string) (string, bool) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil || args.Path == "" {
		return "", false
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

// searchResultLocations 从 search_text 的 file:line:content 输出中提取可跳转位置。
func searchResultLocations(root, output string) []model.ToolLocation {
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
