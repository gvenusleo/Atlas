package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type copySelectionMsg struct {
	text string
}

type selectionPoint struct {
	x int
	y int
}

type textSelection struct {
	active       bool
	dragged      bool
	resumeFollow bool
	anchor       selectionPoint
	cursor       selectionPoint
}

type selectionRange struct {
	start selectionPoint
	end   selectionPoint // end.x is exclusive
}

func (s *textSelection) begin(point selectionPoint, resumeFollow bool) {
	*s = textSelection{
		active:       true,
		resumeFollow: resumeFollow,
		anchor:       point,
		cursor:       point,
	}
}

func (s *textSelection) move(point selectionPoint) {
	if !s.active {
		return
	}
	if point != s.anchor {
		s.dragged = true
	}
	s.cursor = point
}

// bounds maps mouse cells to complete grapheme boundaries.
func (s textSelection) bounds(content string) (selectionRange, bool) {
	if !s.active || !s.dragged {
		return selectionRange{}, false
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return selectionRange{}, false
	}

	anchor := clampSelectionPoint(s.anchor, lines)
	cursor := clampSelectionPoint(s.cursor, lines)
	forward := anchor.y < cursor.y || anchor.y == cursor.y && anchor.x <= cursor.x
	if forward {
		start, _ := graphemeBoundsAt(lines[anchor.y], anchor.x)
		_, end := graphemeBoundsAt(lines[cursor.y], cursor.x)
		return selectionRange{
			start: selectionPoint{x: start, y: anchor.y},
			end:   selectionPoint{x: end, y: cursor.y},
		}, true
	}

	start, _ := graphemeBoundsAt(lines[cursor.y], cursor.x)
	_, end := graphemeBoundsAt(lines[anchor.y], anchor.x)
	return selectionRange{
		start: selectionPoint{x: start, y: cursor.y},
		end:   selectionPoint{x: end, y: anchor.y},
	}, true
}

// content returns the selected visible text without terminal styling.
func (s textSelection) content(content string) string {
	selection, ok := s.bounds(content)
	if !ok {
		return ""
	}

	lines := strings.Split(content, "\n")
	selected := make([]string, 0, selection.end.y-selection.start.y+1)
	for y := selection.start.y; y <= selection.end.y; y++ {
		line := strings.TrimRight(ansi.Strip(lines[y]), " ")
		start, end := 0, ansi.StringWidth(line)
		if y == selection.start.y {
			start = selection.start.x
		}
		if y == selection.end.y {
			end = selection.end.x
		}
		selected = append(selected, ansi.Cut(line, start, end))
	}
	return strings.Trim(strings.Join(selected, "\n"), "\n")
}

// render applies the selection style without changing unselected cells.
func (s textSelection) render(content string, style lipgloss.Style) string {
	selection, ok := s.bounds(content)
	if !ok {
		return content
	}

	lines := strings.Split(content, "\n")
	for y := selection.start.y; y <= selection.end.y; y++ {
		lineWidth := ansi.StringWidth(strings.TrimRight(ansi.Strip(lines[y]), " "))
		start, end := 0, lineWidth
		if y == selection.start.y {
			start = selection.start.x
		}
		if y == selection.end.y {
			end = selection.end.x
		}
		if end > start {
			lines[y] = lipgloss.StyleRanges(lines[y], lipgloss.NewRange(start, end, style))
		}
	}
	return strings.Join(lines, "\n")
}

// clampSelectionPoint keeps a mouse position inside rendered content.
func clampSelectionPoint(point selectionPoint, lines []string) selectionPoint {
	point.y = min(max(point.y, 0), len(lines)-1)
	width := ansi.StringWidth(strings.TrimRight(ansi.Strip(lines[point.y]), " "))
	point.x = min(max(point.x, 0), width)
	return point
}

// graphemeBoundsAt returns the cell range containing column.
func graphemeBoundsAt(line string, column int) (int, int) {
	plain := strings.TrimRight(ansi.Strip(line), " ")
	width := ansi.StringWidth(plain)
	column = min(max(column, 0), width)
	if column == width {
		return width, width
	}

	position := 0
	graphemes := uniseg.NewGraphemes(plain)
	for graphemes.Next() {
		graphemeWidth := max(graphemes.Width(), 1)
		if column < position+graphemeWidth {
			return position, position + graphemeWidth
		}
		position += graphemeWidth
	}
	return width, width
}
