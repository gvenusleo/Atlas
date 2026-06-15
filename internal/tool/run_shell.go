package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultShellTimeoutSeconds = 30
	maxShellTimeoutSeconds     = 300
	maxShellOutputBytes        = 128 * 1024
)

// ShellSpec 描述 Atlas 在当前平台上使用的默认 shell。
type ShellSpec struct {
	DisplayName string
	Command     string
	Args        []string
}

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
					"description": "Command to execute with the platform default shell.",
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

// DefaultShell 返回当前平台默认 shell 的执行参数。
func DefaultShell() ShellSpec {
	return defaultShellSpec(runtime.GOOS, exec.LookPath)
}

// CheckDefaultShell 检查当前平台默认 shell 是否可用。
func CheckDefaultShell() (ShellSpec, error) {
	spec := DefaultShell()
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath(spec.Command); err != nil {
			return spec, fmt.Errorf("PowerShell not found: pwsh or powershell.exe")
		}
		return spec, nil
	}
	info, err := os.Stat(spec.Command)
	if err != nil {
		return spec, err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return spec, fmt.Errorf("%s is not executable", spec.Command)
	}
	return spec, nil
}

func runShellCommand(ctx context.Context, command, workdir string, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spec := DefaultShell()
	args := append([]string(nil), spec.Args...)
	args = append(args, command)
	cmd := exec.CommandContext(runCtx, spec.Command, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	output, err := cmd.CombinedOutput()
	content := truncateShellOutput(output)
	if runCtx.Err() == context.DeadlineExceeded {
		status := fmt.Sprintf("command timed out after %s", timeout)
		return appendShellStatus(content, status), fmt.Errorf("%s", status)
	}
	if ctx.Err() != nil {
		return content, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return appendShellStatus(content, fmt.Sprintf("command exited with code %d", exitErr.ExitCode())), fmt.Errorf("command exited with code %d", exitErr.ExitCode())
		}
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
	return "[output truncated]\n" + string(output[len(output)-maxShellOutputBytes:])
}

func appendShellStatus(output, status string) string {
	status = "[" + status + "]"
	if output == "" {
		return status
	}
	if strings.HasSuffix(output, "\n") {
		return output + status
	}
	return output + "\n" + status
}

func defaultShellSpec(goos string, lookPath func(string) (string, error)) ShellSpec {
	if goos == "windows" {
		for _, command := range []string{"pwsh", "powershell.exe"} {
			if _, err := lookPath(command); err == nil {
				return powerShellSpec(command)
			}
		}
		return powerShellSpec("pwsh")
	}
	return ShellSpec{
		DisplayName: "/bin/sh",
		Command:     "/bin/sh",
		Args:        []string{"-c"},
	}
}

func powerShellSpec(command string) ShellSpec {
	return ShellSpec{
		DisplayName: "PowerShell (" + command + ")",
		Command:     command,
		Args:        []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command"},
	}
}
