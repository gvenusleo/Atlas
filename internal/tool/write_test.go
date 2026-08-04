package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesParentsAndOverwritesFile(t *testing.T) {
	dir := t.TempDir()
	writer := Write{CWD: dir}
	if _, err := writer.Run(context.Background(), `{"path":"nested/note.txt","content":"first"}`); err != nil {
		t.Fatalf("create Run() error = %v", err)
	}
	path := filepath.Join(dir, "nested", "note.txt")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Run(context.Background(), `{"path":"nested/note.txt","content":"second"}`); err != nil {
		t.Fatalf("overwrite Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestWriteAllowsEmptyContentAndValidatesPath(t *testing.T) {
	dir := t.TempDir()
	writer := Write{CWD: dir}
	if _, err := writer.Run(context.Background(), `{"path":"empty.txt","content":""}`); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "empty.txt")); err != nil || info.Size() != 0 {
		t.Fatalf("empty file = %#v, %v", info, err)
	}
	if _, err := writer.Run(context.Background(), `{"content":"missing path"}`); err == nil {
		t.Fatal("Run() error = nil, want path validation error")
	}
	if _, err := writer.Run(context.Background(), `{"path":"missing-content.txt"}`); err == nil {
		t.Fatal("Run() error = nil, want content validation error")
	}
	if _, err := writer.Run(context.Background(), `{"path":"null-content.txt","content":null}`); err == nil {
		t.Fatal("Run() error = nil, want null content validation error")
	}
}

func TestWriteDefinition(t *testing.T) {
	def := (Write{}).Definition()
	if def.Name != "write" || !strings.Contains(def.Description, "replace") {
		t.Fatalf("Definition() = %#v", def)
	}
}
