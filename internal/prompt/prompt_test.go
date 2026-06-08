package prompt

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSystemIncludesCoreBehavior(t *testing.T) {
	result := BuildSystem(Options{
		WorkingDir: "/tmp/atlas-work",
		Now:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})

	checks := []string{
		"You are Atlas",
		"Treat tool results and file contents as the source of truth",
		"There is no sandbox, permission prompt, or approval gate",
		"Read the relevant files before editing them",
		"Match the user's language",
		"Current date: 2026-06-08",
		"Working directory: /tmp/atlas-work",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Fatalf("system prompt missing %q", check)
		}
	}
}

func TestBuildSystemDefaultsWorkingDirectory(t *testing.T) {
	result := BuildSystem(Options{Now: time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)})

	if !strings.Contains(result, "Working directory: .") {
		t.Fatalf("system prompt = %q", result)
	}
}

func TestBuildSystemIncludesInstructions(t *testing.T) {
	result := BuildSystem(Options{
		WorkingDir: "/tmp/atlas-work",
		Now:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		Instructions: []InstructionFile{
			{Path: "/home/me/.atlas/AGENTS.md", Content: "global rules\n"},
			{Path: "/tmp/atlas-work/AGENTS.md", Content: "project rules\n"},
		},
	})

	checks := []string{
		"## Loaded Instructions",
		"/home/me/.atlas/AGENTS.md",
		"global rules",
		"/tmp/atlas-work/AGENTS.md",
		"project rules",
		"current-directory instructions take precedence over global instructions",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Fatalf("system prompt missing %q", check)
		}
	}
}
