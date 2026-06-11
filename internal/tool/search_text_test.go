package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSearchTextRun(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "b.txt"), "skip\nneedle here\n")
	writeTextFile(t, filepath.Join(dir, "nested", "a.txt"), "needle again\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgs(dir, "needle", 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "b.txt:2:needle here\nnested/a.txt:1:needle again"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestSearchTextRunRegex(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "user-123\nuser-abc\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgsWithOptions(dir, `user-\d+`, 0, true, ""))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.txt:1:user-123"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestSearchTextRunLiteralByDefault(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "a.b\nacb\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgs(dir, "a.b", 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.txt:1:a.b"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestSearchTextRunFilePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	writeTextFile(t, file, "needle\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgs(file, "needle", 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "note.txt:1:needle"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestSearchTextRunIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.go"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "b.txt"), "needle\n")
	writeTextFile(t, filepath.Join(dir, "nested", "c.go"), "needle\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgsWithOptions(dir, "needle", 0, false, "*.go"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "a.go:1:needle\nnested/c.go:1:needle"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestSearchTextRunNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "skip\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgs(dir, "needle", 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "No matches found" {
		t.Fatalf("Run() = %q, want no matches message", got)
	}
}

func TestSearchTextRunMaxLines(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "needle 1\nneedle 2\n")

	got, err := (SearchText{}).Run(context.Background(), searchTextArgs(dir, "needle", 1))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "[output truncated]") {
		t.Fatalf("Run() = %q, want truncated marker", got)
	}
	if strings.Count(got, "needle") != 1 {
		t.Fatalf("Run() = %q, want one match", got)
	}
}

func TestSearchTextRunInvalidArguments(t *testing.T) {
	if _, err := (SearchText{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestSearchTextRunMissingPath(t *testing.T) {
	if _, err := (SearchText{}).Run(context.Background(), `{"query":"needle"}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestSearchTextRunMissingQuery(t *testing.T) {
	dir := t.TempDir()
	if _, err := (SearchText{}).Run(context.Background(), `{"path":`+quoteJSON(dir)+`}`); err == nil {
		t.Fatal("Run() error = nil, want missing query error")
	}
}

func TestSearchTextRunMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := (SearchText{}).Run(context.Background(), searchTextArgs(dir, "needle", 0)); err == nil {
		t.Fatal("Run() error = nil, want missing directory error")
	}
}

func TestSearchTextRunInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "needle\n")

	_, err := (SearchText{}).Run(context.Background(), searchTextArgsWithOptions(dir, "[", 0, true, ""))
	if err == nil || !strings.Contains(err.Error(), "invalid search_text regex") {
		t.Fatalf("Run() error = %v, want invalid regex error", err)
	}
}

func TestSearchTextRunInvalidIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeTextFile(t, filepath.Join(dir, "a.txt"), "needle\n")

	_, err := (SearchText{}).Run(context.Background(), searchTextArgsWithOptions(dir, "needle", 0, false, "["))
	if err == nil || !strings.Contains(err.Error(), "invalid search_text include glob") {
		t.Fatalf("Run() error = %v, want invalid include error", err)
	}
}

func TestSearchTextDefinition(t *testing.T) {
	def := (SearchText{}).Definition()
	if def.Name != "search_text" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "search_text")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func searchTextArgs(path, query string, maxLines int) string {
	return searchTextArgsWithOptions(path, query, maxLines, false, "")
}

func searchTextArgsWithOptions(path, query string, maxLines int, regex bool, include string) string {
	args := `{"path":` + quoteJSON(path) + `,"query":` + quoteJSON(query)
	if maxLines > 0 {
		args += `,"max_lines":` + strconv.Itoa(maxLines)
	}
	if regex {
		args += `,"regex":true`
	}
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
