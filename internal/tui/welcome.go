package tui

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/version"
)

const welcomeWideMinWidth = 52

var welcomeLogo = []string{
	"     ✦",
	"    ▟█▙",
	"   ▟███▙",
	"  ▟█████▙",
	" ▟██▙ ▟██▙",
	"▟█▛     ▜█▙",
}

// welcomeView renders the transient new-session identity block.
func (m Model) welcomeView() string {
	width := max(m.width, 1)
	logo := userStyle.Bold(true).Render(strings.Join(welcomeLogo, "\n"))

	if width < 28 {
		return m.welcomeMetadata(width, false)
	}
	if width < welcomeWideMinWidth {
		return logo + "\n\n" + m.welcomeMetadata(width, true)
	}

	const gap = 4
	metadataWidth := max(width-lipgloss.Width(welcomeLogo[len(welcomeLogo)-1])-gap, 1)
	metadata := "\n" + m.welcomeMetadata(metadataWidth, true)
	return lipgloss.JoinHorizontal(lipgloss.Top, logo, strings.Repeat(" ", gap), metadata)
}

func (m Model) welcomeMetadata(width int, labels bool) string {
	name := userStyle.Bold(true).Render("Atlas")
	app := name + "  " + messageStyle.Render("v"+version.Current)
	cwd := compactWorkingDirectory(m.cwd)
	model := m.welcomeModelName()

	if !labels {
		return strings.Join([]string{
			ansi.Truncate(app, width, ""),
			messageStyle.Render(fitFromLeft(cwd, width)),
			messageStyle.Render(fitFromLeft(model, width)),
		}, "\n")
	}

	const labelWidth = 7
	valueWidth := max(width-labelWidth, 1)
	labelStyle := mutedStyle
	if m.hasDarkBackground {
		labelStyle = subtleStyle
	}
	return strings.Join([]string{
		ansi.Truncate(app, width, ""),
		labelStyle.Render("cwd    ") + messageStyle.Render(fitFromLeft(cwd, valueWidth)),
		labelStyle.Render("model  ") + messageStyle.Render(fitFromLeft(model, valueWidth)),
	}, "\n")
}

func (m Model) welcomeModelName() string {
	if m.modelStatusErr != nil {
		return "unavailable"
	}
	if m.modelName == "" {
		return "loading..."
	}
	if m.reasoningEffort == "" {
		return m.modelName
	}
	return m.modelName + " " + m.reasoningEffort
}

func compactWorkingDirectory(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return "."
	}
	clean := filepath.Clean(cwd)
	home, err := os.UserHomeDir()
	if err != nil {
		return clean
	}
	rel, err := filepath.Rel(home, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return clean
	}
	if rel == "." {
		return "~"
	}
	return "~" + string(os.PathSeparator) + rel
}

func fitFromLeft(value string, width int) string {
	if width <= 0 {
		return ""
	}
	valueWidth := lipgloss.Width(value)
	if valueWidth <= width {
		return value
	}
	return ansi.TruncateLeft(value, valueWidth-width+1, "…")
}
