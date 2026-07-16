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
	assistantStyle = lipgloss.NewStyle().Foreground(assistantColor)
	toolStyle      = lipgloss.NewStyle().Foreground(toolColor)
	mutedStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	errorStyle     = lipgloss.NewStyle().Foreground(errorColor)
	headerStyle    = lipgloss.NewStyle().Bold(true)
	dividerStyle   = lipgloss.NewStyle().Foreground(mutedColor)
)
