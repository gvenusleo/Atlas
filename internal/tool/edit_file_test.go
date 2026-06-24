package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEditFileRunAppliesSingleEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "beta", "two"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "replaced 1 block") {
		t.Fatalf("Run() = %q, want success message", got)
	}
	assertFileContent(t, path, "alpha\ntwo\ngamma\n")
}

func TestEditFileRunUsesDefaultCWD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (EditFile{CWD: dir}).Run(context.Background(), editFileArgs("note.txt", "old", "new")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "new\n")
}

func TestEditFileRunAllowsEmptyNewText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello old world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (EditFile{}).Run(context.Background(), editFileArgs(path, " old", "")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "hello world")
}

func TestEditFileRunOldTextNotFoundDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "missing", "new"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Run() error = %v, want not found error", err)
	}
	assertFileContent(t, path, "hello")
}

func TestEditFileRunRejectsNonUniqueOldText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old old"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "old", "new"))
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("Run() error = %v, want non-unique error", err)
	}
	assertFileContent(t, path, "old old")
}

func TestEditFileRunRejectsOverlappingOccurrences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "aa", "b"))
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("Run() error = %v, want non-unique error", err)
	}
	assertFileContent(t, path, "aaa")
}

func TestEditFileRunMissingNewTextDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"old_text":"old"}`)
	if err == nil || !strings.Contains(err.Error(), "new_text is required") {
		t.Fatalf("Run() error = %v, want missing new_text error", err)
	}
	assertFileContent(t, path, "old")
}

func TestEditFileRunPreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix execute bits")
	}
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte("echo old\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "old", "new")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("file mode = %v, want %v", got, os.FileMode(0o755))
	}
}

func TestEditFileRunInvalidArguments(t *testing.T) {
	if _, err := (EditFile{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestEditFileRunMissingPath(t *testing.T) {
	if _, err := (EditFile{}).Run(context.Background(), `{"old_text":"old","new_text":"new"}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestEditFileRunMissingOldText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"new_text":"new"}`); err == nil {
		t.Fatal("Run() error = nil, want missing old_text error")
	}
}

func TestEditFileRunRejectsLegacyEditsArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"edits":[{"old_text":"old","new_text":"new"}]}`); err == nil {
		t.Fatal("Run() error = nil, want missing old_text error")
	}
}

func TestEditFileDefinition(t *testing.T) {
	def := (EditFile{}).Definition()
	if def.Name != "edit_file" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "edit_file")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func editFileArgs(path, oldText, newText string) string {
	return `{"path":` + quoteJSON(path) + `,"old_text":` + quoteJSON(oldText) + `,"new_text":` + quoteJSON(newText) + `}`
}
