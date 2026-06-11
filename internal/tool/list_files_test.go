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
	want := "b.txt\nnested/"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunDepth(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "b.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "a.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "deep", "c.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"depth":1}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "b.txt\nnested/\nnested/a.txt\nnested/deep/"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunIncludesHiddenAndDependencyDirectories(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "keep.txt"))
	writeTestFile(t, filepath.Join(dir, ".git", "config"))
	writeTestFile(t, filepath.Join(dir, "node_modules", "pkg", "index.js"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"depth":2}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".git/\n.git/config\nkeep.txt\nnode_modules/\nnode_modules/pkg/\nnode_modules/pkg/index.js"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunRespectsGitignoreWhenRequested(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\nbuild/\n!important.txt\n")
	writeTestFile(t, filepath.Join(dir, "ignored.txt"))
	writeTestFile(t, filepath.Join(dir, "important.txt"))
	writeTestFile(t, filepath.Join(dir, "build", "artifact.txt"))
	writeTestFile(t, filepath.Join(dir, "keep.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"depth":1,"respect_gitignore":true}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".gitignore\nimportant.txt\nkeep.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunGitignoreTrailingDoubleStarKeepsDirectory(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "foo/**\n")
	writeTestFile(t, filepath.Join(dir, "foo", "a.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"depth":1,"respect_gitignore":true}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".gitignore\nfoo/"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunDoesNotRespectGitignoreByDefault(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\n")
	writeTestFile(t, filepath.Join(dir, "ignored.txt"))
	writeTestFile(t, filepath.Join(dir, "keep.txt"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".gitignore\nignored.txt\nkeep.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"))
	writeTestFile(t, filepath.Join(dir, "b.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "c.go"))

	got, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"depth":1,"include":"*.go"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.go\nnested/c.go"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestListFilesRunInvalidIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"))

	_, err := (ListFiles{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"include":"["}`)
	if err == nil || !strings.Contains(err.Error(), "invalid list_files include glob") {
		t.Fatalf("Run() error = %v, want invalid include error", err)
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
