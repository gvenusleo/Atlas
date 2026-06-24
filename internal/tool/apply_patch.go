package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// ApplyPatch 应用 unified diff patch。
type ApplyPatch struct {
	CWD string
}

// ApplyPatchArgs 是 apply_patch 的 JSON 参数。
type ApplyPatchArgs struct {
	Patch string `json:"patch"`
}

// Definition 返回 apply_patch 的模型可见定义。
func (ApplyPatch) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "apply_patch",
		Description: "Apply a unified diff patch to one or more local text files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "Unified diff patch text.",
				},
			},
			"required": []string{"patch"},
		},
	}
}

// Run 使用 JSON 参数中的 patch 应用 unified diff。
func (a ApplyPatch) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseApplyPatchArgs(arguments)
	if err != nil {
		return "", err
	}
	paths := ApplyPatchPaths(args.Patch, a.CWD)
	if err := runGitApply(ctx, a.CWD, args.Patch, "--check"); err != nil {
		return "", err
	}
	if err := runGitApply(ctx, a.CWD, args.Patch); err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "applied patch", nil
	}
	return "applied patch to " + strings.Join(paths, ", "), nil
}

// ParseApplyPatchArgs 解析并校验 apply_patch 参数。
func ParseApplyPatchArgs(arguments string) (ApplyPatchArgs, error) {
	var args ApplyPatchArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ApplyPatchArgs{}, fmt.Errorf("invalid apply_patch arguments: %w", err)
	}
	if strings.TrimSpace(args.Patch) == "" {
		return ApplyPatchArgs{}, fmt.Errorf("apply_patch patch is required")
	}
	return args, nil
}

// ApplyPatchPaths 从 unified diff 中提取涉及的目标路径。
func ApplyPatchPaths(patch, cwd string) []string {
	seen := map[string]struct{}{}
	var paths []string
	var oldPath string
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = parsePatchPath(strings.TrimPrefix(line, "--- "))
		case strings.HasPrefix(line, "+++ "):
			newPath := parsePatchPath(strings.TrimPrefix(line, "+++ "))
			pathValue := newPath
			if pathValue == "" {
				pathValue = oldPath
			}
			oldPath = ""
			if pathValue == "" {
				continue
			}
			pathValue = resolveToolPath(cwd, pathValue)
			if _, ok := seen[pathValue]; ok {
				continue
			}
			seen[pathValue] = struct{}{}
			paths = append(paths, pathValue)
		}
	}
	return paths
}

func parsePatchPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/dev/null" {
		return ""
	}
	return trimPatchPathPrefix(strings.Fields(value)[0])
}

func runGitApply(ctx context.Context, cwd, patch string, args ...string) error {
	cmdArgs := append([]string{"apply"}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("apply_patch failed: %s", message)
	}
	return nil
}

func trimPatchPathPrefix(pathValue string) string {
	pathValue = filepath.ToSlash(strings.TrimSpace(pathValue))
	for _, prefix := range []string{"a/", "b/"} {
		if strings.HasPrefix(pathValue, prefix) {
			return strings.TrimPrefix(pathValue, prefix)
		}
	}
	return pathValue
}
