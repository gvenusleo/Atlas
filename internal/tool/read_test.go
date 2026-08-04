package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadReturnsRequestedLinesAndContinuation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (Read{CWD: dir}).Run(context.Background(), `{"path":"note.txt","offset":2,"limit":2}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "two\nthree\n\n[Showing lines 2-3. Use offset=4 to continue.]"
	if got != want {
		t.Fatalf("Run() = %q, want %q", got, want)
	}
}

func TestReadSupportsEmptyFileAndRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary"), []byte{0xff}, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (Read{CWD: dir}).Run(context.Background(), `{"path":"empty.txt"}`)
	if err != nil || got != "" {
		t.Fatalf("empty Run() = %q, %v", got, err)
	}
	for _, args := range []string{
		`{"path":"missing"}`,
		`{"path":"folder"}`,
		`{"path":"binary"}`,
		`{"path":"empty.txt","offset":2}`,
		`{"path":"empty.txt","offset":0}`,
		`{"path":"empty.txt","limit":0}`,
		`{"path":"empty.txt","limit":2001}`,
	} {
		if _, err := (Read{CWD: dir}).Run(context.Background(), args); err == nil {
			t.Fatalf("Run(%s) error = nil", args)
		}
	}
}

func TestReadDoesNotReturnPartialOversizedLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Repeat("x", readByteLimit+1)), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (Read{CWD: dir}).Run(context.Background(), `{"path":"long.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "run_shell") {
		t.Fatalf("Run() error = %v, want byte-oriented inspection guidance", err)
	}
}

func TestReadDefinition(t *testing.T) {
	def := (Read{}).Definition()
	if def.Name != "read" || !strings.Contains(def.Description, "UTF-8") {
		t.Fatalf("Definition() = %#v", def)
	}
	properties := def.Parameters["properties"].(map[string]any)
	if got := fmt.Sprint(properties["limit"].(map[string]any)["maximum"]); got != "2000" {
		t.Fatalf("limit maximum = %s", got)
	}
}
