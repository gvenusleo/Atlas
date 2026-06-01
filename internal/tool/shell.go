package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const maxShellOutputBytes = 128 * 1024

// RunShell executes a local shell command with full user permissions.
type RunShell struct{}

func (RunShell) Definition() Definition {
	return Definition{
		Name:        "run_shell",
		Description: "Run a local shell command with full access.",
		Parameters: objectSchema(map[string]any{
			"command": stringSchema("Command to execute with /bin/sh -c."),
			"workdir": stringSchema("Optional working directory."),
			"timeout_seconds": numberSchema(
				"Optional timeout in seconds. Defaults to 30 and caps at 300.",
			),
		}, []string{"command"}),
	}
}

func (RunShell) Execute(ctx context.Context, raw json.RawMessage) Result {
	var args struct {
		Command        string `json:"command"`
		Workdir        string `json:"workdir"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err)
	}
	if args.TimeoutSeconds <= 0 || args.TimeoutSeconds > 300 {
		args.TimeoutSeconds = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(args.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", args.Command)
	if args.Workdir != "" {
		cmd.Dir = args.Workdir
	}
	output, err := cmd.CombinedOutput()
	if len(output) > maxShellOutputBytes {
		output = append(output[:maxShellOutputBytes], []byte("\n[output truncated]")...)
	}
	content := string(output)
	if ctx.Err() == context.DeadlineExceeded {
		return Result{Content: content + "\ncommand timed out", Error: true}
	}
	if err != nil {
		return Result{Content: fmt.Sprintf("%s\ncommand failed: %v", content, err), Error: true}
	}
	return Result{Content: content}
}
