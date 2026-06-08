package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesRun(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "b.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "a.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "b.txt\nnested/a.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunIncludesHiddenAndDependencyDirectories(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "keep.txt"))
	writeTestFile(t, filepath.Join(dir, ".git", "config"))
	writeTestFile(t, filepath.Join(dir, "node_modules", "pkg", "index.js"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".git/config\nkeep.txt\nnode_modules/pkg/index.js"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunMaxFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.txt"))
	writeTestFile(t, filepath.Join(dir, "b.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"max_files":1}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("Run() = %q, want truncated marker", got)
	}
	if strings.Count(got, ".txt") != 1 {
		t.Fatalf("Run() = %q, want one file", got)
	}
}

func TestListFilesRunInvalidArguments(t *testing.T) {
	if _, err := (ListFiles{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestListFilesRunMissingPath(t *testing.T) {
	if _, err := (ListFiles{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestListFilesRunMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`}`); err == nil {
		t.Fatal("Run() error = nil, want missing directory error")
	}
}

func TestListFilesDefinition(t *testing.T) {
	def := (ListFiles{}).Definition()
	if def.Name != "list_files" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "list_files")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
