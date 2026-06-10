package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReadFileRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("Run() = %q, want %q", got, "hello")
	}
}

func TestReadFileRunInvalidArguments(t *testing.T) {
	if _, err := (ReadFile{}).Run(context.Background(), `{`); err == nil {
		t.Fatal("Run() error = nil, want invalid arguments error")
	}
}

func TestReadFileRunMissingPath(t *testing.T) {
	if _, err := (ReadFile{}).Run(context.Background(), `{}`); err == nil {
		t.Fatal("Run() error = nil, want missing path error")
	}
}

func TestReadFileRunMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")
	if _, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`); err == nil {
		t.Fatal("Run() error = nil, want missing file error")
	}
}

func TestReadFileRunLargeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	content := strings.Repeat("x", maxReadFileBytes+1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "byte output limit") {
		t.Fatalf("Run() = %q, want byte limit notice", got)
	}
}

func TestReadFileRunOffsetAndLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"offset":2,"limit":1}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "two\n") || !strings.Contains(got, "Use offset=3 to continue") {
		t.Fatalf("Run() = %q, want selected line and continuation", got)
	}
}

func TestReadFileRunOffsetBeyondEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`,"offset":3}`)
	if err == nil || !strings.Contains(err.Error(), "beyond end of file") {
		t.Fatalf("Run() error = %v, want offset error", err)
	}
}

func TestReadFileDefinition(t *testing.T) {
	def := (ReadFile{}).Definition()
	if def.Name != "read_file" {
		t.Fatalf("Definition().Name = %q, want %q", def.Name, "read_file")
	}
	if def.Parameters == nil {
		t.Fatal("Definition().Parameters = nil")
	}
}

func quoteJSON(value string) string {
	return strconv.Quote(value)
}
