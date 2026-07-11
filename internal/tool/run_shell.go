package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultShellTimeoutSeconds = 30
	maxShellTimeoutSeconds     = 300
	// ShellOutputByteLimit is the maximum command output returned to the model.
	ShellOutputByteLimit = 50 * 1024
	shellOutputEdgeBytes = ShellOutputByteLimit / 2
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
	Command          string `json:"command"`
	CWD              string `json:"cwd"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	SuccessExitCodes []int  `json:"success_exit_codes"`
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
				"success_exit_codes": map[string]any{
					"type":        "array",
					"description": "Exit codes treated as success. Defaults to [0].",
					"items": map[string]any{
						"type":    "integer",
						"minimum": 0,
					},
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
	return runShellCommand(ctx, args.Command, cwd, timeout, args.SuccessExitCodes)
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
	codes, err := normalizeSuccessExitCodes(args.SuccessExitCodes)
	if err != nil {
		return ShellArgs{}, err
	}
	args.SuccessExitCodes = codes
	return args, nil
}

// IsSuccessfulExitCode reports whether code is accepted by run_shell.
func IsSuccessfulExitCode(successCodes []int, code int) bool {
	if len(successCodes) == 0 {
		return code == 0
	}
	for _, successCode := range successCodes {
		if successCode == code {
			return true
		}
	}
	return false
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

func runShellCommand(ctx context.Context, command, workdir string, timeout time.Duration, successExitCodes []int) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spec := DefaultShell()
	args := append([]string(nil), spec.Args...)
	args = append(args, command)
	cmd := exec.CommandContext(runCtx, spec.Command, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	capture, err := newShellOutputCapture()
	if err != nil {
		return "", fmt.Errorf("create command output log: %w", err)
	}
	cmd.Stdout = capture
	cmd.Stderr = capture
	err = cmd.Run()
	content, outputErr := capture.finish()
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
			code := exitErr.ExitCode()
			if IsSuccessfulExitCode(successExitCodes, code) {
				return appendShellStatus(content, fmt.Sprintf("command exited with accepted code %d", code)), outputErr
			}
			return appendShellStatus(content, fmt.Sprintf("command exited with code %d", code)), fmt.Errorf("command exited with code %d", code)
		}
		return content, fmt.Errorf("command failed: %w", err)
	}
	if outputErr != nil {
		return content, outputErr
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

func normalizeSuccessExitCodes(codes []int) ([]int, error) {
	if len(codes) == 0 {
		return []int{0}, nil
	}
	seen := make(map[int]struct{}, len(codes))
	result := make([]int, 0, len(codes))
	for _, code := range codes {
		if code < 0 {
			return nil, fmt.Errorf("run_shell success_exit_codes must not contain negative values")
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result, nil
}

type shellOutputCapture struct {
	mu    sync.Mutex
	file  *os.File
	path  string
	head  []byte
	tail  []byte
	total int64
}

func newShellOutputCapture() (*shellOutputCapture, error) {
	file, err := os.CreateTemp("", "atlas-shell-*.log")
	if err != nil {
		return nil, err
	}
	return &shellOutputCapture{file: file, path: file.Name()}, nil
}

func (c *shellOutputCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, err := c.file.Write(p)
	written := p[:n]
	c.total += int64(n)
	if remaining := shellOutputEdgeBytes - len(c.head); remaining > 0 {
		if remaining > len(written) {
			remaining = len(written)
		}
		c.head = append(c.head, written[:remaining]...)
	}
	if len(written) >= shellOutputEdgeBytes {
		c.tail = append(c.tail[:0], written[len(written)-shellOutputEdgeBytes:]...)
	} else {
		c.tail = append(c.tail, written...)
		if extra := len(c.tail) - shellOutputEdgeBytes; extra > 0 {
			copy(c.tail, c.tail[extra:])
			c.tail = c.tail[:shellOutputEdgeBytes]
		}
	}
	return n, err
}

func (c *shellOutputCapture) finish() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.file.Close(); err != nil {
		_ = os.Remove(c.path)
		return "", fmt.Errorf("close command output log: %w", err)
	}
	if c.total <= ShellOutputByteLimit {
		output, err := os.ReadFile(c.path)
		_ = os.Remove(c.path)
		if err != nil {
			return "", fmt.Errorf("read command output log: %w", err)
		}
		return validShellOutput(output), nil
	}

	omitted := c.total - int64(len(c.head)) - int64(len(c.tail))
	marker := fmt.Sprintf("\n[output truncated: omitted %d bytes; full output: %s]\n", omitted, c.path)
	var output bytes.Buffer
	output.Grow(len(c.head) + len(marker) + len(c.tail))
	output.Write(c.head)
	output.WriteString(marker)
	output.Write(c.tail)
	return validShellOutput(output.Bytes()), nil
}

func validShellOutput(output []byte) string {
	if utf8.Valid(output) {
		return string(output)
	}
	return strings.ToValidUTF8(string(output), "\uFFFD")
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
