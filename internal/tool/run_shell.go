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

// ShellSpec describes the default shell used by Atlas on the current platform.
type ShellSpec struct {
	DisplayName string
	Command     string
	Args        []string
}

// RunShell executes a local shell command.
type RunShell struct {
	CWD string
}

// ShellArgs describes the JSON parameters received by run_shell.
type ShellArgs struct {
	Command        string `json:"command"`
	CWD            string `json:"cwd"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// Definition returns the model-visible definition for run_shell.
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
				"cwd": map[string]any{
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

// Run executes a local shell command using the JSON parameters.
func (r RunShell) Run(ctx context.Context, arguments string) (string, error) {
	args, err := ParseShellArgs(arguments)
	if err != nil {
		return "", err
	}
	timeout := ShellTimeout(args.TimeoutSeconds)
	cwd := args.CWD
	if cwd == "" {
		cwd = r.CWD
	}
	return runShellCommand(ctx, args.Command, cwd, timeout)
}

// ParseShellArgs parses and validates the JSON parameters for run_shell.
func ParseShellArgs(arguments string) (ShellArgs, error) {
	var args ShellArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ShellArgs{}, fmt.Errorf("invalid run_shell arguments: %w", err)
	}
	if args.Command == "" {
		return ShellArgs{}, fmt.Errorf("run_shell command is required")
	}
	return args, nil
}

// ShellTimeout returns the actual timeout duration used by run_shell.
func ShellTimeout(seconds int) time.Duration {
	return normalizeShellTimeout(seconds)
}

// DefaultShell returns the execution parameters for the default shell on the current platform.
func DefaultShell() ShellSpec {
	return defaultShellSpec(runtime.GOOS, exec.LookPath)
}

// CheckDefaultShell checks whether the default shell for the current platform is available.
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
