package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileRunAppliesMultipleEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "alpha", NewText: stringPtr("one")},
		editFileReplacement{OldText: "gamma", NewText: stringPtr("three")},
	))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "replaced 2 blocks") {
		t.Fatalf("Run() = %q, want success message", got)
	}
	assertFileContent(t, path, "one\nbeta\nthree\n")
}

func TestEditFileRunAllowsEmptyNewText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello old world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: " old", NewText: stringPtr("")},
	)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "hello world")
}

func TestEditFileRunOldTextNotFoundDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "missing", NewText: stringPtr("new")},
	))
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

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "old", NewText: stringPtr("new")},
	))
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("Run() error = %v, want non-unique error", err)
	}
}

func TestEditFileRunRejectsOverlappingOccurrences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "aa", NewText: stringPtr("b")},
	))
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("Run() error = %v, want non-unique error", err)
	}
	assertFileContent(t, path, "aaa")
}

func TestEditFileRunRejectsOverlappingEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "abc", NewText: stringPtr("x")},
		editFileReplacement{OldText: "bcd", NewText: stringPtr("y")},
	))
	if err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("Run() error = %v, want overlap error", err)
	}
	assertFileContent(t, path, "abcdef")
}

func TestEditFileRunMissingNewTextDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "old"},
	))
	if err == nil || !strings.Contains(err.Error(), "new_text is required") {
		t.Fatalf("Run() error = %v, want missing new_text error", err)
	}
	assertFileContent(t, path, "old")
}

func TestEditFileRunPreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte("echo old\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := (EditFile{}).Run(context.Background(), editFileArgs(path,
		editFileReplacement{OldText: "old", NewText: stringPtr("new")},
	)); err != nil {
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
	if _, err := (EditFile{}).Run(context.Background(), `{"edits":[]}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestEditFileRunMissingEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`); err == nil {
		t.Fatal("Run() error = nil, want missing edits error")
	}
}

func TestEditFileRunRejectsLegacyTopLevelEditFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"old_text":"old","new_text":"new"}`); err == nil {
		t.Fatal("Run() error = nil, want missing edits error")
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

func editFileArgs(path string, edits ...editFileReplacement) string {
	result := `{"path":` + quoteJSON(path) + `,"edits":[`
	for i, edit := range edits {
		if i > 0 {
			result += ","
		}
		result += `{"old_text":` + quoteJSON(edit.OldText)
		if edit.NewText != nil {
			result += `,"new_text":` + quoteJSON(*edit.NewText)
		}
		result += `}`
	}
	return result + `]}`
}

func stringPtr(value string) *string {
	return &value
}
