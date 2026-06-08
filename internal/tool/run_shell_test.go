package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunShellRun(t *testing.T) {
	got, err := (RunShell{}).Run(context.Background(), `{"command":"printf hello"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("Run() = %q, want %q", got, "hello")
	}
}

func TestRunShellRunFailure(t *testing.T) {
	got, err := (RunShell{}).Run(context.Background(), `{"command":"printf fail; exit 7"}`)
	if err == nil {
		t.Fatal("Run() error = nil, want command failure")
	}
	if got != "fail" {
		t.Fatalf("Run() = %q, want %q", got, "fail")
	}
}

func TestRunShellRunWorkdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (RunShell{}).Run(context.Background(), `{"command":"pwd; ls note.txt","workdir":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, dir) || !strings.Contains(got, "note.txt") {
		t.Fatalf("Run() = %q, want workdir output", got)
	}
}

func TestRunShellRunInvalidArguments(t *testing.T) {
	if _, err := (RunShell{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestRunShellRunMissingCommand(t *testing.T) {
	if _, err := (RunShell{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}

func TestRunShellRunTimeout(t *testing.T) {
	_, err := (RunShell{}).Run(context.Background(), `{"command":"sleep 2","timeout_seconds":1}`)
	if err == nil {
		t.Fatal("Run() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Run() error = %v, want timeout error", err)
	}
}

func TestRunShellRunTruncatesOutput(t *testing.T) {
	command := "yes x | head -c " + quoteShellInt(maxShellOutputBytes+1)
	got, err := runShellCommand(context.Background(), command, "", normalizeShellTimeout(0))
	if err != nil {
		t.Fatalf("runShellCommand() error = %v", err)
	}
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("runShellCommand() = %q, want truncated marker", got[len(got)-64:])
	}
}

func TestRunShellDefinition(t *testing.T) {
	def := (RunShell{}).Definition()
	if def.Name != "run_shell" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "run_shell")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func quoteShellInt(value int) string {
	return strconv.Itoa(value)
}
