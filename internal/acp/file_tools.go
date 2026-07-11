package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
)

const (
	maxACPToolDiffBytes = 512 * 1024
	textProbeBytes      = 8 * 1024
)

// isFileTool determines whether a tool needs ACP file display enhancement.
func isFileTool(name string) bool {
	switch name {
	case "read_file", "write_file", "edit_file", "apply_patch":
		return true
	default:
		return false
	}
}

// runFileTool supplements file-type tools with ACP filesystem calls and display metadata.
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
	default:
		return fallback(ctx, call)
	}
}

// runReadFileTool prioritizes reading editor-side content via the client filesystem.
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

// runWriteFileTool prioritizes writing via the client filesystem and generates diff metadata.
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

// runLocalReadFileTool reads a file using Atlas local tools and supplements ACP metadata.
func runLocalReadFileTool(ctx context.Context, call model.ToolCall, path string, args tool.ReadFileArgs, metadata model.ToolMetadata, fallback tool.RunFunc) (tool.RunResult, error) {
	result, err := fallback(ctx, toolCallWithArgs(call, tool.ReadFileArgs{
		Path:  path,
		Line:  args.Line,
		Limit: args.Limit,
	}))
	result.Metadata = mergeToolMetadata(result.Metadata, metadata)
	return result, err
}

// runLocalWriteFileTool writes a file using Atlas local tools and generates diff metadata.
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

// runEditFileTool applies Atlas's unique block replacement rules based on the client's current file content.
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

// runLocalEditFileTool edits a file using Atlas local tools and generates diff metadata.
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

// runApplyPatchTool applies a patch and supplements ACP with location and single-file diff metadata.
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

// readToolOldText reads old content for diff; treats as new file when unreadable.
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

// readLocalToolOldText reads local old content; returns nil for new files.
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

// readClientToolText reads file content from the ACP client.
func (a *Agent) readClientToolText(ctx context.Context, sessionID acpsdk.SessionId, path string) (string, error) {
	resp, err := a.fileClient.ReadTextFile(ctx, acpsdk.ReadTextFileRequest{SessionId: sessionID, Path: path})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// readLocalToolText reads file content from the Atlas process's local filesystem.
func readLocalToolText(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// clientReadTextFileAllowed avoids passing directories or binary files to the ACP client for reading.
func clientReadTextFileAllowed(path string) bool {
	return localTextFile(path)
}

// clientOldTextReadAllowed reads client old content only when the path does not exist or is locally confirmed as a text file.
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

// clientWriteTextFileAllowed avoids requesting the client to write directories or special files as text files.
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

// localTextFile determines whether a local path is a plain text file safe for text interface handling.
func localTextFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return false
	}
	return localFileLooksText(path)
}

// localFileLooksText samples the file header, rejecting NUL bytes and non-UTF-8 content.
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

// acpToolDiff generates persistable diff metadata within a reasonable size limit.
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

// mergeToolMetadata preserves existing metadata and only fills in missing fields.
func mergeToolMetadata(base, extra model.ToolMetadata) model.ToolMetadata {
	if len(base.Locations) == 0 {
		base.Locations = extra.Locations
	}
	if base.Diff == nil {
		base.Diff = extra.Diff
	}
	return base
}

// absoluteToolPath resolves a tool path under the ACP session cwd.
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

// lineOrZero normalizes non-positive line numbers to zero.
func lineOrZero(line int) int {
	if line > 0 {
		return line
	}
	return 0
}

// toolCallWithArgs replaces the raw JSON of a tool call with structured parameters.
func toolCallWithArgs(call model.ToolCall, args any) model.ToolCall {
	data, err := json.Marshal(args)
	if err != nil {
		return call
	}
	call.Arguments = string(data)
	return call
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
