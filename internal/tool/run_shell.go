package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultShellTimeoutSeconds = 30
	maxShellTimeoutSeconds     = 300
	maxShellOutputBytes        = 128 * 1024
)

// RunShell 执行本地 shell 命令。
type RunShell struct{}

// Definition 返回 run_shell 的模型可见定义。
func (RunShell) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "run_shell",
		Description: "Run a local shell command and return combined output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute with /bin/sh -c.",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Optional working directory.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds.",
				},
			},
			"required": []string{"command"},
		},
	}
}

// Run 使用 JSON 参数执行一次本地 shell 命令。
func (RunShell) Run(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Command        string `json:"command"`
		Workdir        string `json:"workdir"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid run_shell arguments: %w", err)
	}
	if args.Command == "" {
		return "", fmt.Errorf("run_shell command is required")
	}
	timeout := normalizeShellTimeout(args.TimeoutSeconds)
	return runShellCommand(ctx, args.Command, args.Workdir, timeout)
}

func runShellCommand(ctx context.Context, command, workdir string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}
	output, err := cmd.CombinedOutput()
	content := truncateShellOutput(output)
	if ctx.Err() == context.DeadlineExceeded {
		return content, fmt.Errorf("command timed out: %w", ctx.Err())
	}
	if err != nil {
		return content, fmt.Errorf("command failed: %w", err)
	}
	return content, nil
}

func normalizeShellTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultShellTimeoutSeconds
	}
	if seconds > maxShellTimeoutSeconds {
		seconds = maxShellTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func truncateShellOutput(output []byte) string {
	if len(output) <= maxShellOutputBytes {
		return string(output)
	}
	return string(output[:maxShellOutputBytes]) + "\n[output truncated]"
}
