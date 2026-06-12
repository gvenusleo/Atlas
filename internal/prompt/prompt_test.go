package prompt

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildSystemIncludesCoreBehavior(t *testing.T) {
	result := BuildSystem(Options{
		WorkingDir: "/tmp/atlas-work",
		Platform:   "test-os",
		Now:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})

	checks := []string{
		"You are Atlas",
		"Treat tool results and file contents as the source of truth",
		"For simple greetings or questions",
		"There is no sandbox, permission prompt, or approval gate",
		"Read the relevant files before editing them",
		"Use only the tools that Atlas exposes",
		"Do not treat command completion alone as proof",
		"Match the user's language",
		"Current date: 2026-06-08",
		"Working directory: /tmp/atlas-work",
		"Platform: test-os",
		"Shell: /bin/sh",
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
	if !strings.Contains(result, "Platform: "+runtime.GOOS) {
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
		"<instruction_file path=\"/home/me/.atlas/AGENTS.md\">",
		"global rules",
		"</instruction_file>",
		"<instruction_file path=\"/tmp/atlas-work/AGENTS.md\">",
		"project rules",
		"current-directory instructions take precedence over global instructions",
		"the wrapper is not part of the file content",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Fatalf("system prompt missing %q", check)
		}
	}
}

func TestBuildSystemIncludesOnlySkillSummaries(t *testing.T) {
	result := BuildSystem(Options{
		WorkingDir: "/tmp/atlas-work",
		Now:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		Skills: []SkillSummary{
			{Name: "write", Description: "polish prose"},
		},
	})

	for _, check := range []string{
		"## Available Skills",
		"`write`: polish prose",
		"call load_skill with the skill name",
	} {
		if !strings.Contains(result, check) {
			t.Fatalf("system prompt missing %q", check)
		}
	}
	if strings.Contains(result, "# Write") {
		t.Fatalf("system prompt includes skill body: %q", result)
	}
}
