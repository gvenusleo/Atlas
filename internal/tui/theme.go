package tui

import "charm.land/lipgloss/v2"

// Color palette for the borderless TUI.
var (
	userColor      = lipgloss.Color("4") // terminal-defined blue
	assistantColor = lipgloss.Color("2") // terminal-defined green
	toolColor      = lipgloss.Color("3") // terminal-defined yellow
	mutedColor     = lipgloss.Color("8") // terminal-defined grey
	errorColor     = lipgloss.Color("1") // terminal-defined red

	userStyle      = lipgloss.NewStyle().Foreground(userColor)
	messageStyle   = lipgloss.NewStyle()
	assistantStyle = lipgloss.NewStyle().Foreground(assistantColor)
	toolStyle      = lipgloss.NewStyle().Foreground(toolColor)
	mutedStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	errorStyle     = lipgloss.NewStyle().Foreground(errorColor)
)

func userMessageStyle(hasDarkBackground bool) lipgloss.Style {
	background := lipgloss.LightDark(hasDarkBackground)(lipgloss.Color("255"), lipgloss.Color("234"))
	return lipgloss.NewStyle().Background(background).Padding(1, 1)
}

func composerStyle(hasDarkBackground bool) lipgloss.Style {
	return userMessageStyle(hasDarkBackground).PaddingLeft(0)
}
