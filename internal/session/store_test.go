package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, err := Open(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".atlas", "atlas.db")); err != nil {
		t.Fatalf("default database: %v", err)
	}
}
