package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestApplyPatchRunAppliesMultipleOperations(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "modify.txt"), "line1\nline2\n")
	writeTextFile(t, filepath.Join(dir, "delete.txt"), "obsolete\n")
	patch := `*** Begin Patch
*** Add File: nested/new.txt
+created
*** Update File: modify.txt
@@
-line2
+changed
*** Delete File: delete.txt
*** End Patch`

	result, err := (ApplyPatch{CWD: dir}).RunResult(context.Background(), applyPatchArgs(patch))
	if err != nil {
		t.Fatalf("RunResult() error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, "nested/new.txt"), "created\n")
	assertFileContent(t, filepath.Join(dir, "modify.txt"), "line1\nchanged\n")
	if _, err := os.Stat(filepath.Join(dir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
	if len(result.Metadata.Diffs) != 3 || len(result.Metadata.Locations) != 3 {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	if !strings.Contains(result.Content, "A nested/new.txt") || !strings.Contains(result.Content, "M modify.txt") || !strings.Contains(result.Content, "D delete.txt") {
		t.Fatalf("RunResult() content = %q", result.Content)
	}
}

func TestApplyPatchRunMovesAndUpdatesFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "old", "name.txt")
	destination := filepath.Join(dir, "new dir", "name.txt")
	writeTextFile(t, source, "before\n")
	patch := `*** Begin Patch
*** Update File: old/name.txt
*** Move to: new dir/name.txt
@@
-before
+after
*** End Patch`

	result, err := (ApplyPatch{CWD: dir}).RunResult(context.Background(), applyPatchArgs(patch))
	if err != nil {
		t.Fatalf("RunResult() error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	assertFileContent(t, destination, "after\n")
	if len(result.Metadata.Diffs) != 2 {
		t.Fatalf("diffs = %#v", result.Metadata.Diffs)
	}
}

func TestApplyPatchRunMoveOnlyPreservesContent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	destination := filepath.Join(dir, "destination.txt")
	writeTextFile(t, source, "no trailing newline")
	patch := `*** Begin Patch
*** Update File: source.txt
*** Move to: destination.txt
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, destination, "no trailing newline")
}

func TestApplyPatchRunCreatesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	patch := `*** Begin Patch
*** Add File: empty.txt
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, "empty.txt"), "")
}

func TestApplyPatchValidationFailureDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	patch := `*** Begin Patch
*** Add File: created.txt
+created
*** Update File: missing.txt
@@
-old
+new
*** End Patch`

	_, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch))
	if err == nil || !strings.Contains(err.Error(), "cannot update missing file") {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("validation failure wrote a file: %v", err)
	}
}

func TestApplyPatchRunPreservesBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "windows.txt")
	writeTextFile(t, path, "\uFEFFone\r\ntwo\r\n")
	patch := `*** Begin Patch
*** Update File: windows.txt
@@
-two
+changed
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "\uFEFFone\r\nchanged\r\n")
}

func TestApplyPatchRunUsesContextAndEndOfFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sections.txt")
	writeTextFile(t, path, "top\nsection\nold\nlast\n")
	patch := `*** Begin Patch
*** Update File: sections.txt
@@ section
-old
+changed
@@
-last
+final
*** End of File
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "top\nsection\nchanged\nfinal\n")
}

func TestApplyPatchRunRejectsContextMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	writeTextFile(t, path, "current\n")
	patch := `*** Begin Patch
*** Update File: file.txt
@@
-missing
+new
*** End Patch`

	_, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch))
	if err == nil || !strings.Contains(err.Error(), "failed to find expected lines") {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "current\n")
}

func TestApplyPatchRunRejectsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.dat")
	if err := os.WriteFile(path, []byte{0xff, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `*** Begin Patch
*** Add File: binary.dat
+text
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err == nil || !strings.Contains(err.Error(), "not a UTF-8 text file") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestApplyPatchRunRejectsSymbolicLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links require additional privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	writeTextFile(t, target, "target\n")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	patch := `*** Begin Patch
*** Delete File: link.txt
*** End Patch`

	if _, err := (ApplyPatch{CWD: dir}).Run(context.Background(), applyPatchArgs(patch)); err == nil || !strings.Contains(err.Error(), "does not support symbolic links") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestApplyPatchPathsSupportsSpaces(t *testing.T) {
	cwd := filepath.Clean("/tmp/project")
	patch := `*** Begin Patch
*** Add File: dir/new file.txt
+hello
*** Update File: old file.txt
*** Move to: moved file.txt
@@
-old
+new
*** End Patch`

	got := ApplyPatchPaths(patch, cwd)
	want := []string{
		filepath.Join(cwd, "dir", "new file.txt"),
		filepath.Join(cwd, "moved file.txt"),
		filepath.Join(cwd, "old file.txt"),
	}
	if len(got) != len(want) {
		t.Fatalf("ApplyPatchPaths() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("ApplyPatchPaths()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestApplyPatchRunInvalidPatch(t *testing.T) {
	tests := []string{
		"not a patch",
		"*** Begin Patch\n*** End Patch",
		"*** Begin Patch\n*** Frobnicate File: file.txt\n*** End Patch",
		"*** Begin Patch\n*** Update File: file.txt\n*** End Patch",
		"*** Begin Patch\n*** Update File: file.txt\n@@\n-old\n+new\n*** End of File\n+extra\n*** End Patch",
	}
	for _, patch := range tests {
		if _, err := (ApplyPatch{}).Run(context.Background(), applyPatchArgs(patch)); err == nil {
			t.Fatalf("Run(%q) error = nil", patch)
		}
	}
}

func TestApplyPatchRunInvalidArguments(t *testing.T) {
	if _, err := (ApplyPatch{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
	if _, err := (ApplyPatch{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing patch error")
	}
}

func TestApplyPatchDefinition(t *testing.T) {
	def := (ApplyPatch{}).Definition()
	if def.Name != "apply_patch" || def.Parameters == nil || !strings.Contains(def.Description, "Codex-style") {
		t.Fatalf("Definition() = %#v", def)
	}
}

func TestPathLockSetRemovesUnusedLocks(t *testing.T) {
	locks := newPathLockSet()
	unlock := locks.lock([]string{"/tmp/a", "/tmp/b"})
	if len(locks.locks) != 2 {
		t.Fatalf("locks = %d, want 2", len(locks.locks))
	}
	unlock()
	if len(locks.locks) != 0 {
		t.Fatalf("locks = %d, want 0", len(locks.locks))
	}
}

func TestPatchDiffsEnforcesTotalLimit(t *testing.T) {
	content := strings.Repeat("x", maxPatchDiffBytes+1)
	diffs := patchDiffs([]patchMutation{{
		path: "/tmp/large.txt",
		new:  patchFileState{exists: true, content: content, mode: 0o644},
	}})
	if len(diffs) != 0 {
		t.Fatalf("diffs = %#v, want none", diffs)
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
