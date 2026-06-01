package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
)

var markdownRenderers sync.Map

// markdownFence records the marker needed to find a matching closing fence.
type markdownFence struct {
	marker rune
	length int
}

// renderMarkdown renders assistant Markdown with compact terminal styling.
func renderMarkdown(source string, width int) ([]string, error) {
	if width < 1 {
		width = 1
	}
	renderer, err := markdownRenderer(width)
	if err != nil {
		return nil, err
	}
	rendered, err := renderer.Render(unwrapMarkdownTableFences(source))
	if err != nil {
		return nil, err
	}
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return nil, nil
	}
	return strings.Split(rendered, "\n"), nil
}

// markdownRenderer returns a width-specific renderer for repeated transcript draws.
func markdownRenderer(width int) (*glamour.TermRenderer, error) {
	if cached, ok := markdownRenderers.Load(width); ok {
		return cached.(*glamour.TermRenderer), nil
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
		glamour.WithInlineTableLinks(true),
	)
	if err != nil {
		return nil, err
	}
	actual, _ := markdownRenderers.LoadOrStore(width, renderer)
	return actual.(*glamour.TermRenderer), nil
}

// markdownStyle keeps Markdown readable without adding large margins or panels.
func markdownStyle() glamouransi.StyleConfig {
	return glamouransi.StyleConfig{
		Document: glamouransi.StyleBlock{},
		BlockQuote: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Color: stringPtr("2"),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("> "),
		},
		Paragraph: glamouransi.StyleBlock{},
		List: glamouransi.StyleList{
			LevelIndent: 2,
		},
		Heading: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				BlockSuffix: "\n",
				Bold:        boolPtr(true),
			},
		},
		H1: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix:    "# ",
				Underline: boolPtr(true),
			},
		},
		H2: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix: "## ",
			},
		},
		H3: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix: "### ",
			},
		},
		H4: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix: "#### ",
			},
		},
		H5: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix: "##### ",
			},
		},
		H6: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Prefix: "###### ",
			},
		},
		Strikethrough: glamouransi.StylePrimitive{
			CrossedOut: boolPtr(true),
		},
		Emph: glamouransi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: glamouransi.StylePrimitive{
			Bold: boolPtr(true),
		},
		HorizontalRule: glamouransi.StylePrimitive{
			Color:  stringPtr("8"),
			Format: "\n--------\n",
		},
		Item: glamouransi.StylePrimitive{
			BlockPrefix: "• ",
			Color:       stringPtr("6"),
		},
		Enumeration: glamouransi.StylePrimitive{
			BlockPrefix: ". ",
			Color:       stringPtr("6"),
		},
		Task: glamouransi.StyleTask{
			Ticked:   "[x] ",
			Unticked: "[ ] ",
		},
		Link: glamouransi.StylePrimitive{
			Color:     stringPtr("6"),
			Underline: boolPtr(true),
		},
		LinkText: glamouransi.StylePrimitive{
			Color: stringPtr("6"),
		},
		Code: glamouransi.StyleBlock{
			StylePrimitive: glamouransi.StylePrimitive{
				Color: stringPtr("6"),
			},
		},
		CodeBlock: glamouransi.StyleCodeBlock{
			StyleBlock: glamouransi.StyleBlock{
				StylePrimitive: glamouransi.StylePrimitive{
					Color: stringPtr("7"),
				},
			},
		},
		Table: glamouransi.StyleTable{
			CenterSeparator: stringPtr("|"),
			ColumnSeparator: stringPtr("|"),
			RowSeparator:    stringPtr("-"),
		},
		DefinitionDescription: glamouransi.StylePrimitive{
			BlockPrefix: "\n• ",
		},
	}
}

// unwrapMarkdownTableFences mirrors Codex's table-friendly markdown fence rule.
func unwrapMarkdownTableFences(source string) string {
	lines := strings.Split(source, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		fence, ok := parseMarkdownFenceOpen(lines[i])
		if !ok {
			out = append(out, lines[i])
			continue
		}

		start := i + 1
		end := start
		for end < len(lines) && !isMarkdownFenceClose(lines[end], fence) {
			end++
		}
		if end == len(lines) {
			out = append(out, lines[i])
			continue
		}

		body := lines[start:end]
		if markdownLinesContainTable(body) {
			out = append(out, body...)
		} else {
			out = append(out, lines[i:end+1]...)
		}
		i = end
	}
	return strings.Join(out, "\n")
}

// parseMarkdownFenceOpen accepts only md/markdown info strings for table unwraps.
func parseMarkdownFenceOpen(line string) (markdownFence, bool) {
	trimmed := trimFenceIndent(line)
	if trimmed == "" {
		return markdownFence{}, false
	}
	marker := rune(trimmed[0])
	if marker != '`' && marker != '~' {
		return markdownFence{}, false
	}
	length := countLeadingRune(trimmed, marker)
	if length < 3 {
		return markdownFence{}, false
	}
	info := strings.ToLower(strings.TrimSpace(trimmed[length:]))
	if fields := strings.Fields(info); len(fields) > 0 {
		info = fields[0]
	}
	if info != "md" && info != "markdown" {
		return markdownFence{}, false
	}
	return markdownFence{marker: marker, length: length}, true
}

// isMarkdownFenceClose reports whether line closes the previously opened fence.
func isMarkdownFenceClose(line string, fence markdownFence) bool {
	trimmed := trimFenceIndent(line)
	if trimmed == "" || rune(trimmed[0]) != fence.marker {
		return false
	}
	length := countLeadingRune(trimmed, fence.marker)
	return length >= fence.length && strings.TrimSpace(trimmed[length:]) == ""
}

// trimFenceIndent allows CommonMark's optional 0-3 leading spaces before fences.
func trimFenceIndent(line string) string {
	count := 0
	for count < len(line) && count < 4 && line[count] == ' ' {
		count++
	}
	if count > 3 {
		return line
	}
	return line[count:]
}

// countLeadingRune counts the repeated fence marker at the start of text.
func countLeadingRune(text string, marker rune) int {
	count := 0
	for _, r := range text {
		if r != marker {
			break
		}
		count++
	}
	return count
}

// markdownLinesContainTable detects a header row followed by a delimiter row.
func markdownLinesContainTable(lines []string) bool {
	var previous string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			previous = ""
			continue
		}
		if markdownTableHeaderLine(previous) && markdownTableDelimiterLine(trimmed) {
			return true
		}
		previous = trimmed
	}
	return false
}

// markdownTableHeaderLine reports whether a line can be the table header row.
func markdownTableHeaderLine(line string) bool {
	return strings.Contains(line, "|") && !markdownTableDelimiterLine(line)
}

// markdownTableDelimiterLine reports whether a line is a Markdown table delimiter.
func markdownTableDelimiterLine(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	columns := strings.Split(strings.TrimSpace(line), "|")
	if len(columns) > 0 && strings.TrimSpace(columns[0]) == "" {
		columns = columns[1:]
	}
	if len(columns) > 0 && strings.TrimSpace(columns[len(columns)-1]) == "" {
		columns = columns[:len(columns)-1]
	}
	if len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		cell := strings.TrimSpace(column)
		cell = strings.TrimPrefix(cell, ":")
		cell = strings.TrimSuffix(cell, ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

// boolPtr returns a pointer for Glamour's style config fields.
func boolPtr(value bool) *bool {
	return &value
}

// stringPtr returns a pointer for Glamour's style config fields.
func stringPtr(value string) *string {
	return &value
}

// uintPtr returns a pointer for Glamour's style config fields.
func uintPtr(value uint) *uint {
	return &value
}
