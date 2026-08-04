package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditAppliesMultipleEditsAgainstOriginalContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := (Edit{CWD: dir}).Run(context.Background(), `{"path":"note.txt","edits":[{"old_text":"gamma","new_text":"delta"},{"old_text":"alpha","new_text":"first"}]}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "Applied 2 edit(s) to note.txt" {
		t.Fatalf("Run() = %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\nbeta\ndelta\n" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestEditPreservesBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	original := append(append([]byte(nil), utf8BOM...), []byte("one\r\ntwo\r\n")...)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (Edit{CWD: dir}).Run(context.Background(), `{"path":"note.txt","edits":[{"old_text":"one\ntwo","new_text":"first\nsecond"}]}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), utf8BOM...), []byte("first\r\nsecond\r\n")...)
	if string(data) != string(want) {
		t.Fatalf("content = %q, want %q", data, want)
	}
}

func TestEditFailuresDoNotModifyFile(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		args  string
		match string
	}{
		{name: "missing", body: "alpha", args: `{"path":"note.txt","edits":[{"old_text":"beta","new_text":"x"}]}`, match: "0 matches"},
		{name: "duplicate", body: "alpha alpha", args: `{"path":"note.txt","edits":[{"old_text":"alpha","new_text":"x"}]}`, match: "2 matches"},
		{name: "empty old text", body: "alpha", args: `{"path":"note.txt","edits":[{"old_text":"","new_text":"x"}]}`, match: "must not be empty"},
		{name: "missing new text", body: "alpha", args: `{"path":"note.txt","edits":[{"old_text":"alpha"}]}`, match: "new_text is required"},
		{name: "null new text", body: "alpha", args: `{"path":"note.txt","edits":[{"old_text":"alpha","new_text":null}]}`, match: "new_text is required"},
		{name: "no-op", body: "alpha", args: `{"path":"note.txt","edits":[{"old_text":"alpha","new_text":"alpha"}]}`, match: "does not change"},
		{name: "overlap", body: "abcdef", args: `{"path":"note.txt","edits":[{"old_text":"abcd","new_text":"x"},{"old_text":"cdef","new_text":"y"}]}`, match: "overlap"},
		{name: "overlapping duplicate", body: "aaa", args: `{"path":"note.txt","edits":[{"old_text":"aa","new_text":"x"}]}`, match: "2 matches"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "note.txt")
			if err := os.WriteFile(path, []byte(test.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := (Edit{CWD: dir}).Run(context.Background(), test.args)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("Run() error = %v, want %q", err, test.match)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != test.body {
				t.Fatalf("content after failure = %q, %v", data, readErr)
			}
		})
	}
}

func TestEditRejectsNonUTF8AndEmptyEdits(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "binary"), []byte{0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{
		`{"path":"binary","edits":[{"old_text":"x","new_text":"y"}]}`,
		`{"path":"binary","edits":[]}`,
	} {
		if _, err := (Edit{CWD: dir}).Run(context.Background(), args); err == nil {
			t.Fatalf("Run(%s) error = nil", args)
		}
	}
}

func TestEditDefinition(t *testing.T) {
	def := (Edit{}).Definition()
	if def.Name != "edit" || !strings.Contains(def.Description, "all-or-nothing") {
		t.Fatalf("Definition() = %#v", def)
	}
}
