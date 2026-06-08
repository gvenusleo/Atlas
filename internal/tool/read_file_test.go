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

	if _, err := (ReadFile{}).Run(context.Background(), `{"path":`+quoteJSON(path)+`}`); err == nil {
		t.Fatal("Run() error = nil, want large file error")
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
