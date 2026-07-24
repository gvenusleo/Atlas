package tui

import (
	"image/color"

	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// colorPalette defines the text hierarchy for one terminal background mode.
// Error is reserved for failures and does not participate in normal hierarchy.
type colorPalette struct {
	highlight string
	text      string
	muted     string
	error     string
}

type tuiTheme struct {
	palette   colorPalette
	highlight lipgloss.Style
	text      lipgloss.Style
	muted     lipgloss.Style
	error     lipgloss.Style
	selection lipgloss.Style
}

var (
	lightTheme = newTUITheme(colorPalette{
		highlight: "#005CC5",
		text:      "#242424",
		muted:     "#737373",
		error:     "#B42318",
	})
	darkTheme = newTUITheme(colorPalette{
		highlight: "#82AAFF",
		text:      "#E6E6E6",
		muted:     "#A3A3A3",
		error:     "#FF7B72",
	})
)

func newTUITheme(palette colorPalette) tuiTheme {
	return tuiTheme{
		palette:   palette,
		highlight: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.highlight)),
		text:      lipgloss.NewStyle().Foreground(lipgloss.Color(palette.text)),
		muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)),
		error:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.error)),
		selection: lipgloss.NewStyle().Reverse(true),
	}
}

func themeFor(hasDarkBackground bool) tuiTheme {
	if hasDarkBackground {
		return darkTheme
	}
	return lightTheme
}

func userMessageStyle(hasDarkBackground bool, terminalBackground color.Color) lipgloss.Style {
	theme := themeFor(hasDarkBackground)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.palette.text)).
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
	theme := themeFor(hasDarkBackground)
	text := theme.palette.text
	highlight := theme.palette.highlight
	muted := theme.palette.muted

	style.Document = glamouransi.StyleBlock{}
	style.Text.Color = &text
	style.Paragraph.BlockPrefix = markdownParagraphStart
	style.Paragraph.BlockSuffix = markdownParagraphEnd
	style.BlockQuote.BlockPrefix = markdownBlockquoteStart
	style.BlockQuote.BlockSuffix = markdownBlockquoteEnd
	style.BlockQuote.Indent = nil
	style.BlockQuote.IndentToken = nil
	style.Heading.Color = &highlight
	style.H1 = glamouransi.StyleBlock{}
	style.H2 = glamouransi.StyleBlock{}
	style.H3 = glamouransi.StyleBlock{}
	style.H4 = glamouransi.StyleBlock{}
	style.H5 = glamouransi.StyleBlock{}
	style.H6 = glamouransi.StyleBlock{}
	style.HorizontalRule.Color = &muted
	style.Link.Color = &highlight
	style.LinkText.Color = &highlight
	style.Image.Color = &highlight
	style.ImageText.Color = &muted
	style.Code.Color = &highlight
	style.Code.BackgroundColor = nil
	style.Code.Prefix = ""
	style.Code.Suffix = ""
	style.CodeBlock.Color = &text
	style.CodeBlock.Margin = nil
	style.CodeBlock.Theme = ""
	style.CodeBlock.Chroma = nil
	return style
}
