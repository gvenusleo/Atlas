package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansiSequencePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func TestRenderMarkdownStylesBold(t *testing.T) {
	lines, err := renderMarkdown("This is **bold**.", 80)
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(lines, "\n")
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "This is bold.") || strings.Contains(plain, "**") {
		t.Fatalf("unexpected bold render: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("bold text should include ANSI styling: %q", rendered)
	}
}

func TestRenderMarkdownStylesInlineCode(t *testing.T) {
	lines, err := renderMarkdown("Use `go test` now.", 80)
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(lines, "\n")
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "go test") || strings.Contains(plain, "`go test`") {
		t.Fatalf("unexpected inline code render: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("inline code should include ANSI styling: %q", rendered)
	}
}

func TestRenderMarkdownPreservesCodeBlockContent(t *testing.T) {
	lines, err := renderMarkdown("```go\nfmt.Println(\"hi\")\n```", 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "fmt.Println(\"hi\")") || strings.Contains(plain, "```") {
		t.Fatalf("unexpected code block render: %q", plain)
	}
}

func TestRenderMarkdownRendersTable(t *testing.T) {
	lines, err := renderMarkdown("| A | B |\n|---|---|\n| 1 | 2 |", 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "A") || !strings.Contains(plain, "B") ||
		!strings.Contains(plain, "1") || !strings.Contains(plain, "2") {
		t.Fatalf("table cells missing: %q", plain)
	}
	if strings.Contains(plain, "|---|---|") {
		t.Fatalf("table delimiter should be rendered, not shown raw: %q", plain)
	}
}

func TestRenderMarkdownUnwrapsMarkdownFencedTable(t *testing.T) {
	lines, err := renderMarkdown("```markdown\n| A | B |\n|---|---|\n| 1 | 2 |\n```", 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	if strings.Contains(plain, "```") || strings.Contains(plain, "|---|---|") {
		t.Fatalf("markdown fenced table should render as a table: %q", plain)
	}
	if !strings.Contains(plain, "A") || !strings.Contains(plain, "2") {
		t.Fatalf("table cells missing after fence unwrap: %q", plain)
	}
}

func TestRenderMarkdownKeepsNonMarkdownFenceAsCode(t *testing.T) {
	lines, err := renderMarkdown("```rust\n| A | B |\n|---|---|\n```", 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "|---|---|") || strings.Contains(plain, "```") {
		t.Fatalf("non-markdown fence should remain a code block: %q", plain)
	}
}

func TestRenderMarkdownRendersListsAndHeading(t *testing.T) {
	lines, err := renderMarkdown("# Title\n\n1. Tight item\n- Loose item", 80)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "# Title") {
		t.Fatalf("heading missing: %q", plain)
	}
	if !strings.Contains(plain, "1. Tight item") {
		t.Fatalf("ordered list should stay on one line: %q", plain)
	}
	if !strings.Contains(plain, "• Loose item") {
		t.Fatalf("unordered list should include a bullet: %q", plain)
	}
}

func stripANSI(text string) string {
	return ansiSequencePattern.ReplaceAllString(text, "")
}
