package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunShellRun(t *testing.T) {
	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellEchoCommand("hello"))+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(got) != "hello" {
		t.Fatalf("Run() = %q, want %q", got, "hello")
	}
}

func TestRunShellRunFailure(t *testing.T) {
	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellFailCommand("fail", 7))+`}`)
	if err == nil {
		t.Fatal("Run() error = nil, want command failure")
	}
	if !strings.Contains(got, "fail") || !strings.Contains(got, "command exited with code 7") {
		t.Fatalf("Run() = %q, want output and exit code", got)
	}
}

func TestRunShellRunWorkdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellWorkdirCommand())+`,"workdir":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, filepath.Base(dir)) || !strings.Contains(got, "note.txt") {
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
	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellTimeoutCommand())+`,"timeout_seconds":1}`)
	if err == nil {
		t.Fatal("Run() error = nil, want timeout error")
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "command timed out") {
		t.Fatalf("Run() = %q, want output and timeout status", got)
	}
}

func TestRunShellRunTruncatesOutput(t *testing.T) {
	got := truncateShellOutput([]byte("prefix" + strings.Repeat("x", maxShellOutputBytes) + "suffix"))
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("truncateShellOutput() = %q, want truncated marker", got[:64])
	}
	if strings.Contains(got, "prefix") || !strings.Contains(got, "suffix") {
		t.Fatalf("truncateShellOutput() should keep tail, got prefix=%v suffix=%v", strings.Contains(got, "prefix"), strings.Contains(got, "suffix"))
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

func TestDefaultShellSpec(t *testing.T) {
	spec := defaultShellSpec("linux", func(string) (string, error) {
		t.Fatal("lookPath should not be called for linux")
		return "", nil
	})
	if spec.Command != "/bin/sh" || strings.Join(spec.Args, " ") != "-c" || spec.DisplayName != "/bin/sh" {
		t.Fatalf("linux shell spec = %#v", spec)
	}

	spec = defaultShellSpec("windows", func(command string) (string, error) {
		if command == "pwsh" {
			return `C:\Program Files\PowerShell\7\pwsh.exe`, nil
		}
		t.Fatalf("unexpected lookup after pwsh: %s", command)
		return "", os.ErrNotExist
	})
	if spec.Command != "pwsh" || !strings.Contains(spec.DisplayName, "PowerShell") {
		t.Fatalf("windows shell spec = %#v", spec)
	}

	spec = defaultShellSpec("windows", func(command string) (string, error) {
		if command == "powershell.exe" {
			return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
		}
		return "", os.ErrNotExist
	})
	if spec.Command != "powershell.exe" {
		t.Fatalf("windows fallback shell spec = %#v", spec)
	}
}

func shellEchoCommand(text string) string {
	if runtime.GOOS == "windows" {
		return "Write-Output " + text
	}
	return "printf " + text
}

func shellFailCommand(text string, code int) string {
	if runtime.GOOS == "windows" {
		return "Write-Output " + text + "; exit " + fmt.Sprint(code)
	}
	return "printf " + text + "; exit " + fmt.Sprint(code)
}

func shellWorkdirCommand() string {
	if runtime.GOOS == "windows" {
		return "(Get-Location).Path; Get-ChildItem note.txt | Select-Object -ExpandProperty Name"
	}
	return "pwd; ls note.txt"
}

func shellTimeoutCommand() string {
	if runtime.GOOS == "windows" {
		return "Write-Output before; Start-Sleep -Seconds 2"
	}
	return "printf before; sleep 2"
}
