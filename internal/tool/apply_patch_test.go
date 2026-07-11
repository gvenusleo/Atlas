package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchRunUpdatesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "old a\n")
	writeTextFile(t, filepath.Join(dir, "b.txt"), "old b\n")
	patch := strings.Join([]string{
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1 +1 @@",
		"-old a",
		"+new a",
		"--- a/b.txt",
		"+++ b/b.txt",
		"@@ -1 +1 @@",
		"-old b",
		"+new b",
		"",
	}, "\n")

	got, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "b.txt") {
		t.Fatalf("Run() = %q, want changed paths", got)
	}
	assertFileContent(t, filepath.Join(dir, "a.txt"), "new a\n")
	assertFileContent(t, filepath.Join(dir, "b.txt"), "new b\n")
}

func TestApplyPatchRunCreatesFile(t *testing.T) {
	dir := t.TempDir()
	patch := strings.Join([]string{
		"--- /dev/null",
		"+++ b/new.txt",
		"@@ -0,0 +1 @@",
		"+hello",
		"",
	}, "\n")

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, "new.txt"), "hello\n")
}

func TestApplyPatchRunFailureDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	writeTextFile(t, path, "current\n")
	patch := strings.Join([]string{
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1 +1 @@",
		"-missing",
		"+new",
		"",
	}, "\n")

	_, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch))
	if err == nil || !strings.Contains(err.Error(), "apply_patch failed") {
		t.Fatalf("Run() error = %v, want apply failure", err)
	}
	assertFileContent(t, path, "current\n")
}

func TestApplyPatchPaths(t *testing.T) {
	cwd := filepath.Clean("/tmp/project")
	patch := strings.Join([]string{
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"--- a/old.txt",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-gone",
		"",
	}, "\n")

	got := ApplyPatchPaths(patch, cwd)
	want := []string{filepath.Join(cwd, "a.txt"), filepath.Join(cwd, "old.txt")}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ApplyPatchPaths() = %#v, want %#v", got, want)
	}
}

func TestApplyPatchRunInvalidArguments(t *testing.T) {
	if _, err := (ApplyPatch{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestApplyPatchRunMissingPatch(t *testing.T) {
	if _, err := (ApplyPatch{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing patch error")
	}
}

func TestApplyPatchDefinition(t *testing.T) {
	def := (ApplyPatch{}).Definition()
	if def.Name != "apply_patch" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "apply_patch")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func applyPatchArgs(patch string) string {
	return `{"patch":` + quoteJSON(patch) + `}`
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
