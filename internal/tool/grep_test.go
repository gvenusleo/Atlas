package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepRun(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "b.txt"), "skip\nneedle here\n")
	writeTextFile(t, filepath.Join(dir, "nested", "a.txt"), "needle again\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "b.txt:2:needle here\nnested/a.txt:1:needle again"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGrepRunRegexByDefault(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "replaceTextAndCompute\nreplaceOther\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, `replaceTextAndCompute|replaceTextAndTryCompute`, ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.txt:1:replaceTextAndCompute"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGrepRunFilePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	writeTextFile(t, file, "needle\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(file, "needle", ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "note.txt:1:needle"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGrepRunIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.go"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "b.txt"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "nested", "c.go"), "needle\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", "*.go"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.go:1:needle\nnested/c.go:1:needle"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGrepRunIncludeBraceGlob(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.js"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "b.json"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "c.txt"), "needle\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", "*.{js,json}"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.js:1:needle\nb.json:1:needle"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestGrepRunRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\n")
	writeTextFile(t, filepath.Join(dir, "ignored.txt"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "keep.txt"), "needle\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "keep.txt:1:needle" {
		t.Fatalf("Run() = %q, want only non-ignored match", got)
	}
}

func TestGrepRunUsesDefaultCWD(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "note.txt"), "needle\n")

	got, err := (Grep{CWD: dir}).Run(context.Background(), `{"pattern":"needle"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "note.txt:1:needle" {
		t.Fatalf("Run() = %q, want cwd-relative result", got)
	}
}

func TestGrepRunNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "skip\n")

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "No matches found" {
		t.Fatalf("Run() = %q, want no matches message", got)
	}
}

func TestGrepRunTruncates(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < defaultGrepLimit+1; i++ {
		writeTextFile(t, filepath.Join(dir, "file-"+string(rune('a'+i%26))+"-"+string(rune('a'+i/26))+".txt"), "needle\n")
	}

	got, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("Run() = %q, want truncated marker", got)
	}
}

func TestGrepRunInvalidArguments(t *testing.T) {
	if _, err := (Grep{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestGrepRunMissingPattern(t *testing.T) {
	if _, err := (Grep{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing pattern error")
	}
}

func TestGrepRunMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", "")); err == nil {
		t.Fatal("Run() error = nil, want missing directory error")
	}
}

func TestGrepRunInvalidPattern(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "needle\n")

	_, err := (Grep{}).Run(context.Background(), grepArgs(dir, "[", ""))
	if err == nil || !strings.Contains(err.Error(), "invalid grep pattern") {
		t.Fatalf("Run() error = %v, want invalid pattern error", err)
	}
}

func TestGrepRunInvalidIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "needle\n")

	_, err := (Grep{}).Run(context.Background(), grepArgs(dir, "needle", "["))
	if err == nil || !strings.Contains(err.Error(), "invalid grep include glob") {
		t.Fatalf("Run() error = %v, want invalid include error", err)
	}
}

func TestGrepDefinition(t *testing.T) {
	def := (Grep{}).Definition()
	if def.Name != "grep" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "grep")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func grepArgs(path, pattern, include string) string {
	args := `{"path":` + quoteJSON(path) + `,"pattern":` + quoteJSON(pattern)
	if include != "" {
		args += `,"include":` + quoteJSON(include)
	}
	return args + `}`
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
