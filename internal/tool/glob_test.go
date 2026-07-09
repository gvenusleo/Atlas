package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobRun(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "b.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "a.txt"))

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"*"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "b.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGlobRunBraceExpansion(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "config", "a.js"))
	writeTestFile(t, filepath.Join(dir, "config", "b.json"))
	writeTestFile(t, filepath.Join(dir, "config", "c.txt"))

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"**/config/*.{js,json}"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "config/a.js\nconfig/b.json"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGlobRunRecursivePattern(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"))
	writeTestFile(t, filepath.Join(dir, "b.txt"))
	writeTestFile(t, filepath.Join(dir, "nested", "c.go"))

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.go\nnested/c.go"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGlobRunSkipsVCSMetadata(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "keep.txt"))
	writeTestFile(t, filepath.Join(dir, ".git", "config"))
	writeTestFile(t, filepath.Join(dir, ".hg", "store"))
	writeTestFile(t, filepath.Join(dir, ".svn", "entries"))
	writeTestFile(t, filepath.Join(dir, ".github", "workflows", "ci.yml"))

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"**/*"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".github/workflows/ci.yml\nkeep.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGlobRunRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\nbuild/\n!important.txt\n")
	writeTestFile(t, filepath.Join(dir, "ignored.txt"))
	writeTestFile(t, filepath.Join(dir, "important.txt"))
	writeTestFile(t, filepath.Join(dir, "build", "artifact.txt"))
	writeTestFile(t, filepath.Join(dir, "keep.txt"))

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"**/*"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := ".gitignore\nimportant.txt\nkeep.txt"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGlobRunUsesDefaultCWD(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "note.txt"))

	got, err := (Glob{CWD: dir}).Run(context.Background(), `{"pattern":"*.txt"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "note.txt" {
		t.Fatalf("Run() = %q, want cwd-relative match", got)
	}
}

func TestGlobRunInvalidPattern(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.go"))

	_, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"["}`)
	if err == nil || !strings.Contains(err.Error(), "invalid pattern glob") {
		t.Fatalf("Run() error = %v, want invalid pattern error", err)
	}
}

func TestGlobRunTruncates(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < defaultGlobLimit+1; i++ {
		writeTestFile(t, filepath.Join(dir, "file-"+string(rune('a'+i%26))+"-"+string(rune('a'+i/26))+".txt"))
	}

	got, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"*.txt"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("Run() = %q, want truncated marker", got)
	}
}

func TestGlobRunInvalidArguments(t *testing.T) {
	if _, err := (Glob{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestGlobRunMissingPattern(t *testing.T) {
	if _, err := (Glob{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing pattern error")
	}
}

func TestGlobRunMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := (Glob{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`,"pattern":"*"}`); err == nil {
		t.Fatal("Run() error = nil, want missing directory error")
	}
}

func TestGlobDefinition(t *testing.T) {
	def := (Glob{}).Definition()
	if def.Name != "glob" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "glob")
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
