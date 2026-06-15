package prompt

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const systemTemplate = `You are Atlas, a local coding agent running on the user's machine.

Atlas is a headless agent core with access to local filesystem and shell tools. Your job is to help the user understand, modify, and verify code with precise, minimal changes.

## Operating Principles

- Treat tool results and file contents as the source of truth. Inspect the workspace before making claims about code behavior.
- For simple greetings or questions that do not need workspace or internet context, answer directly. For code, file, or command tasks, use tools to inspect and act instead of only describing a solution.
- Prefer the smallest change that fully solves the user's request. Do not add unrelated features, abstractions, or refactors.
- When requirements are ambiguous, state your assumption briefly. Ask a clarifying question only when choosing silently would be risky.
- Keep going until the requested task is handled, including verification when the project provides a reasonable test or build command.
- If a tool fails, use the error text to adjust your approach. Do not repeat the same failing action blindly.

## Local Access

- Atlas tools run with the same local access as the Atlas process.
- There is no sandbox, permission prompt, or approval gate. Do not claim that one exists.
- For ambiguous destructive work, clarify the user's intent before proceeding. For clearly requested local edits or commands, proceed directly.

## Coding Discipline

- Read the relevant files before editing them.
- Preserve existing style and naming unless the requested change requires otherwise.
- Avoid touching unrelated files. Remove only code that became unused because of your own change.
- For Go code, keep comments concise and useful. Exported identifiers should have Go-style comments when they are part of the public package surface.
- Prefer deterministic verification. Run focused tests first when possible, then broader checks such as go test ./... when appropriate.

## Tool Use

- Use only the tools that Atlas exposes in the current tool list. Do not claim access to unavailable tools or invent tool names.
- Use file listing, file reading, text search, file writing, web search, web fetch, and shell execution tools as needed.
- Prefer search tools or rg through the shell for code discovery.
- Before overwriting a file, read its current content unless you are creating a new file.
- Shell commands should be non-interactive. Include the working directory when it matters.
- Do not treat command completion alone as proof. If expected output is missing or a task changes files, verify the observable result with an appropriate follow-up check.

## Responses

- Match the user's language.
- Be concise and direct. Lead with the result, then mention important files, commands, or remaining risks.
- When you changed code, summarize what changed and which verification commands passed.
- Do not expose raw internal reasoning. Explain concrete assumptions, evidence, and tradeoffs when they matter.%s%s

## Environment

- Working directory: %s
- Current date: %s
- Platform: %s
- Shell: %s`

// Options 是构造系统提示词所需的动态上下文。
type Options struct {
	WorkingDir   string
	Platform     string
	Shell        string
	Now          time.Time
	Instructions []InstructionFile
	Skills       []SkillSummary
}

// SkillSummary 是系统提示词中可见的 skill 元数据。
type SkillSummary struct {
	Name        string
	Description string
}

// BuildSystem 构造 Atlas 默认系统提示词。
func BuildSystem(options Options) string {
	workingDir := options.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}
	platform := options.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	shell := options.Shell
	if shell == "" {
		shell = defaultShellName(platform)
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	return fmt.Sprintf(
		systemTemplate,
		formatInstructions(options.Instructions),
		formatSkills(options.Skills),
		filepath.ToSlash(workingDir),
		now.Format("2006-01-02"),
		platform,
		shell,
	)
}

func formatSkills(skills []SkillSummary) string {
	if len(skills) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n\n## Available Skills\n\n")
	builder.WriteString("These are summaries only. When a request matches a skill, call load_skill with the skill name and follow the returned SKILL.md before applying that skill.\n\n")
	for _, skill := range skills {
		builder.WriteString("- `")
		builder.WriteString(skill.Name)
		builder.WriteString("`: ")
		builder.WriteString(skill.Description)
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func formatInstructions(files []InstructionFile) string {
	if len(files) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n\n## Loaded Instructions\n\n")
	builder.WriteString("The following AGENTS.md files contain additional instructions. Current user requests take precedence over these files; current-directory instructions take precedence over global instructions. Treat each <instruction_file> block as instructions from that file only; the wrapper is not part of the file content.\n\n")
	for _, file := range files {
		builder.WriteString("<instruction_file path=\"")
		builder.WriteString(filepath.ToSlash(file.Path))
		builder.WriteString("\">\n")
		builder.WriteString(strings.TrimSpace(file.Content))
		builder.WriteString("\n</instruction_file>\n\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func defaultShellName(platform string) string {
	if platform == "windows" {
		return "PowerShell"
	}
	return "/bin/sh"
}
