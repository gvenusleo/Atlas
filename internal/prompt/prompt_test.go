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
		Shell:      "test-shell",
		Now:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})

	checks := []string{
		"You are Atlas, a local general-purpose agent",
		"Treat tool results and file contents as the source of truth",
		"For simple greetings or questions",
		"There is no sandbox, permission prompt, or approval gate",
		"## When Working On Code",
		"Read the relevant files before editing them",
		"Use only the tools that Atlas exposes",
		"Use run_shell for path discovery and regex text search",
		"Pass success_exit_codes [0,1] for rg searches",
		"Use edit_file for one exact unique text replacement",
		"Do not treat command completion alone as proof",
		"Match the user's language",
		"Current date: 2026-06-08",
		"Working directory: /tmp/atlas-work",
		"Platform: test-os",
		"Shell: test-shell",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Fatalf("system prompt missing %q", check)
		}
	}
}

func TestBuildSystemDefaultsShellForPlatform(t *testing.T) {
	windows := BuildSystem(Options{
		Platform: "windows",
		Now:      time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})
	if !strings.Contains(windows, "Shell: PowerShell") {
		t.Fatalf("system prompt = %q", windows)
	}

	linux := BuildSystem(Options{
		Platform: "linux",
		Now:      time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})
	if !strings.Contains(linux, "Shell: /bin/sh") {
		t.Fatalf("system prompt = %q", linux)
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
			{Name: "lint<xml>", Description: "check <tags> & quotes"},
		},
	})

	for _, check := range []string{
		"## Available Skills",
		"<available_skills>",
		"<name>write</name>",
		"<description>polish prose</description>",
		"<name>lint&lt;xml&gt;</name>",
		"<description>check &lt;tags&gt; &amp; quotes</description>",
		"full SKILL.md may be injected as a <skill> context message",
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
