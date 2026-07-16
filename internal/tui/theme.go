package tui

import (
	"image/color"

	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

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
	selectionStyle = lipgloss.NewStyle().Reverse(true)
)

func userMessageStyle(hasDarkBackground bool, terminalBackground color.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(userMessageBackground(hasDarkBackground, terminalBackground)).
		Padding(1, 1)
}

func composerStyle(hasDarkBackground bool, terminalBackground color.Color) lipgloss.Style {
	return userMessageStyle(hasDarkBackground, terminalBackground).PaddingLeft(0)
}

// userMessageBackground matches Codex CLI's terminal-aware composer tint.
func userMessageBackground(hasDarkBackground bool, terminalBackground color.Color) color.Color {
	if terminalBackground == nil {
		return lipgloss.LightDark(hasDarkBackground)(lipgloss.Color("255"), lipgloss.Color("239"))
	}

	r, g, b, _ := terminalBackground.RGBA()
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
	isLight := 0.299*float32(r8)+0.587*float32(g8)+0.114*float32(b8) > 128
	top, alpha := float32(255), float32(0.12)
	if isLight {
		top, alpha = 0, 0.04
	}
	blend := func(channel uint8) uint8 {
		return uint8(top*alpha + float32(channel)*(1-alpha))
	}
	return color.RGBA{R: blend(r8), G: blend(g8), B: blend(b8), A: 255}
}

// markdownStyle adapts Glamour's built-in palette to Atlas's borderless layout.
func markdownStyle(hasDarkBackground bool) glamouransi.StyleConfig {
	style := glamourstyles.LightStyleConfig
	if hasDarkBackground {
		style = glamourstyles.DarkStyleConfig
	}

	style.Document = glamouransi.StyleBlock{}
	style.Paragraph.BlockPrefix = markdownParagraphStart
	style.Paragraph.BlockSuffix = markdownParagraphEnd
	style.BlockQuote.BlockPrefix = markdownBlockquoteStart
	style.BlockQuote.BlockSuffix = markdownBlockquoteEnd
	style.BlockQuote.Indent = nil
	style.BlockQuote.IndentToken = nil
	style.H1 = glamouransi.StyleBlock{}
	style.H2 = glamouransi.StyleBlock{}
	style.H3 = glamouransi.StyleBlock{}
	style.H4 = glamouransi.StyleBlock{}
	style.H5 = glamouransi.StyleBlock{}
	style.H6 = glamouransi.StyleBlock{}
	style.Code.BackgroundColor = nil
	style.Code.Prefix = ""
	style.Code.Suffix = ""
	style.CodeBlock.Margin = nil
	style.CodeBlock.Theme = ""
	style.CodeBlock.Chroma = nil
	return style
}
