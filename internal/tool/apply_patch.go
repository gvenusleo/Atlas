package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// ApplyPatch applies Codex-style text file patches.
type ApplyPatch struct {
	CWD string
}

// ApplyPatchArgs is the JSON parameters for apply_patch.
type ApplyPatchArgs struct {
	Patch string `json:"patch"`
}

// Definition returns the model-visible definition for apply_patch.
func (ApplyPatch) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name: "apply_patch",
		Description: "Apply a Codex-style text patch. The patch must start with '*** Begin Patch' and end with '*** End Patch', " +
			"and may contain Add File, Update File, Delete File, and Move to operations.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "Codex-style patch text. Add File content lines start with '+'. Update File uses @@ sections whose lines start with ' ', '+', or '-'; an optional Move to line follows the Update File header.",
				},
			},
			"required": []string{"patch"},
		},
	}
}

// Run applies a patch and returns its text summary.
func (a ApplyPatch) Run(ctx context.Context, arguments string) (string, error) {
	result, err := a.RunResult(ctx, arguments)
	return result.Content, err
}

// RunResult applies a patch and returns its summary and actual file changes.
func (a ApplyPatch) RunResult(ctx context.Context, arguments string) (RunResult, error) {
	args, err := ParseApplyPatchArgs(arguments)
	if err != nil {
		return RunResult{}, err
	}
	actions, err := parsePatch(args.Patch)
	if err != nil {
		return RunResult{}, err
	}
	return applyPatchActions(ctx, a.CWD, actions)
}

// ParseApplyPatchArgs parses and validates apply_patch parameters.
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

// ApplyPatchPaths extracts resolved paths from a valid Codex-style patch.
func ApplyPatchPaths(patch, cwd string) []string {
	actions, err := parsePatch(patch)
	if err != nil {
		return nil
	}
	return resolvedPatchPaths(actions, cwd)
}
