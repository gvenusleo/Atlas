package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")

	if _, err := (WriteFile{}).Run(context.Background(), writeFileArgs(path, "hello")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "hello")
}

func TestWriteFileRunUsesDefaultCWD(t *testing.T) {
	dir := t.TempDir()

	if _, err := (WriteFile{CWD: dir}).Run(context.Background(), writeFileArgs("note.txt", "hello")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, "note.txt"), "hello")
}

func TestWriteFileRunCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "note.txt")

	if _, err := (WriteFile{}).Run(context.Background(), writeFileArgs(path, "hello")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "hello")
}

func TestWriteFileRunOverwritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (WriteFile{}).Run(context.Background(), writeFileArgs(path, "new")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "new")
}

func TestWriteFileRunAllowsEmptyContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")

	if _, err := (WriteFile{}).Run(context.Background(), writeFileArgs(path, "")); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertFileContent(t, path, "")
}

func TestWriteFileRunInvalidArguments(t *testing.T) {
	if _, err := (WriteFile{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestWriteFileRunMissingPath(t *testing.T) {
	if _, err := (WriteFile{}).Run(context.Background(), `{"content":"hello"}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestWriteFileRunMissingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if _, err := (WriteFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`); err == nil {
		t.Fatal("Run() error = nil, want missing content error")
	}
}

func TestWriteFileDefinition(t *testing.T) {
	def := (WriteFile{}).Definition()
	if def.Name != "write_file" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "write_file")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func writeFileArgs(path, content string) string {
	return `{"path":` + quoteJSON(path) + `,"content":` + quoteJSON(content) + `}`
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}
