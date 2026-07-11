package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestRunShellRunAcceptsConfiguredExitCode(t *testing.T) {
	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellFailCommand("no-matches", 1))+`,"success_exit_codes":[0,1]}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "no-matches") || !strings.Contains(got, "[command exited with accepted code 1]") {
		t.Fatalf("Run() = %q, want output and accepted exit code", got)
	}
}

func TestRunShellRunCWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (RunShell{}).Run(context.Background(), `{"command":`+quoteJSON(shellWorkdirCommand())+`,"cwd":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, filepath.Base(dir)) || !strings.Contains(got, "note.txt") {
		t.Fatalf("Run() = %q, want workdir output", got)
	}
}

func TestRunShellRunUsesDefaultCWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (RunShell{CWD: dir}).Run(context.Background(), `{"command":`+quoteJSON(shellWorkdirCommand())+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, filepath.Base(dir)) || !strings.Contains(got, "note.txt") {
		t.Fatalf("Run() = %q, want default cwd output", got)
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

func TestParseShellArgsNormalizesSuccessExitCodes(t *testing.T) {
	args, err := ParseShellArgs(`{"command":"check","success_exit_codes":[0,1,1]}`)
	if err != nil {
		t.Fatalf("ParseShellArgs() error = %v", err)
	}
	if got := fmt.Sprint(args.SuccessExitCodes); got != "[0 1]" {
		t.Fatalf("SuccessExitCodes = %s, want [0 1]", got)
	}

	args, err = ParseShellArgs(`{"command":"check","success_exit_codes":[]}`)
	if err != nil {
		t.Fatalf("ParseShellArgs() error = %v", err)
	}
	if got := fmt.Sprint(args.SuccessExitCodes); got != "[0]" {
		t.Fatalf("default SuccessExitCodes = %s, want [0]", got)
	}

	if _, err := ParseShellArgs(`{"command":"check","success_exit_codes":[-1]}`); err == nil {
		t.Fatal("ParseShellArgs() error = nil, want negative exit code error")
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

func TestShellOutputCaptureKeepsEdgesAndFullLog(t *testing.T) {
	capture, err := newShellOutputCapture()
	if err != nil {
		t.Fatal(err)
	}
	full := []byte(strings.Repeat("h", shellOutputEdgeBytes) + "omitted-output" + strings.Repeat("t", shellOutputEdgeBytes))
	if _, err := capture.Write(full); err != nil {
		t.Fatal(err)
	}
	got, err := capture.finish()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(capture.path)
	if !strings.HasPrefix(got, strings.Repeat("h", shellOutputEdgeBytes)) || !strings.HasSuffix(got, strings.Repeat("t", shellOutputEdgeBytes)) {
		t.Fatal("captured output did not preserve its first and last 64 KiB")
	}
	if !strings.Contains(got, "[output truncated: omitted 14 bytes; full output: "+capture.path+"]") {
		t.Fatalf("captured output missing truncation details: %q", got[shellOutputEdgeBytes:shellOutputEdgeBytes+128])
	}
	stored, err := os.ReadFile(capture.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(full) {
		t.Fatal("full output log does not match command output")
	}
}

func TestShellOutputCaptureRemovesUntruncatedLog(t *testing.T) {
	capture, err := newShellOutputCapture()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Write([]byte("complete output")); err != nil {
		t.Fatal(err)
	}
	got, err := capture.finish()
	if err != nil {
		t.Fatal(err)
	}
	if got != "complete output" {
		t.Fatalf("finish() = %q, want exact output", got)
	}
	if _, err := os.Stat(capture.path); !os.IsNotExist(err) {
		t.Fatalf("temporary log still exists: %v", err)
	}
}

func TestValidShellOutputReplacesInvalidUTF8(t *testing.T) {
	got := validShellOutput([]byte{'a', 0xff, 'b'})
	if !strings.Contains(got, "\uFFFD") || !utf8.ValidString(got) {
		t.Fatalf("validShellOutput() = %q, want valid UTF-8 replacement", got)
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
