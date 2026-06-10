package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditFileRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello old world"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "old", "new"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "replaced 1 block") {
		t.Fatalf("Run() = %q, want success message", got)
	}
	assertFileContent(t, path, "hello new world")
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

func TestEditFileRunOldTextNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (EditFile{}).Run(context.Background(), editFileArgs(path, "missing", "new"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Run() error = %v, want not found error", err)
	}
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

func TestEditFileRunMissingOldTextArgument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"new_text":"new"}`); err == nil {
		t.Fatal("Run() error = nil, want missing old_text error")
	}
}

func TestEditFileRunMissingNewText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (EditFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"old_text":"old"}`); err == nil {
		t.Fatal("Run() error = nil, want missing new_text error")
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
