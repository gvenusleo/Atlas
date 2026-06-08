package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInstructionsLoadsGlobalAndCurrentOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	project := t.TempDir()
	parent := filepath.Dir(project)
	writeFile(t, filepath.Join(home, ".atlas", "AGENTS.md"), "global")
	writeFile(t, filepath.Join(parent, "AGENTS.md"), "parent")
	writeFile(t, filepath.Join(project, "AGENTS.md"), "current")

	files, err := LoadInstructions(project)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
	if files[0].Content != "global" {
		t.Fatalf("global content = %q", files[0].Content)
	}
	if files[1].Content != "current" {
		t.Fatalf("current content = %q", files[1].Content)
	}
	parentInstruction := filepath.Join(parent, "AGENTS.md")
	for _, file := range files {
		if file.Path == parentInstruction {
			t.Fatalf("loaded parent instruction: %s", file.Path)
		}
	}
}

func TestLoadInstructionsMissingFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	files, err := LoadInstructions(t.TempDir())
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %#v", files)
	}
}

func TestLoadInstructionsDeduplicatesSamePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	atlasDir := filepath.Join(home, ".atlas")
	writeFile(t, filepath.Join(atlasDir, "AGENTS.md"), "global")

	files, err := LoadInstructions(atlasDir)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %#v", files)
	}
}

func TestLoadInstructionsRejectsLargeFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".atlas", "AGENTS.md"), strings.Repeat("a", maxInstructionBytes+1))

	_, err := LoadInstructions(t.TempDir())
	if err == nil {
		t.Fatal("LoadInstructions() error = nil")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
