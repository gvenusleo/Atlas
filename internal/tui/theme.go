package tui

import (
	"image/color"

	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	catppuccin "github.com/catppuccin/go"
)

// colorPalette defines stable semantic colors for one terminal background mode.
type colorPalette struct {
	text      string
	muted     string
	divider   string
	surface   string
	selected  string
	link      string
	code      string
	brand     string
	reasoning string
	context   string
	working   string
	success   string
	error     string
}

type tuiTheme struct {
	palette   colorPalette
	text      lipgloss.Style
	muted     lipgloss.Style
	divider   lipgloss.Style
	selected  lipgloss.Style
	brand     lipgloss.Style
	reasoning lipgloss.Style
	context   lipgloss.Style
	working   lipgloss.Style
	success   lipgloss.Style
	error     lipgloss.Style
	selection lipgloss.Style
}

var (
	lightTheme = newTUITheme(catppuccin.Latte, catppuccin.Latte.Mantle())
	darkTheme  = newTUITheme(catppuccin.Mocha, catppuccin.Mocha.Surface0())
)

func newTUITheme(flavor catppuccin.Flavor, surface catppuccin.Color) tuiTheme {
	palette := colorPalette{
		text:      flavor.Text().Hex,
		muted:     flavor.Subtext0().Hex,
		divider:   flavor.Surface2().Hex,
		surface:   surface.Hex,
		selected:  flavor.Blue().Hex,
		link:      flavor.Sky().Hex,
		code:      flavor.Teal().Hex,
		brand:     flavor.Lavender().Hex,
		reasoning: flavor.Mauve().Hex,
		context:   flavor.Teal().Hex,
		working:   flavor.Yellow().Hex,
		success:   flavor.Green().Hex,
		error:     flavor.Red().Hex,
	}
	return tuiTheme{
		palette:   palette,
		text:      lipgloss.NewStyle().Foreground(lipgloss.Color(palette.text)),
		muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.muted)),
		divider:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.divider)),
		selected:  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.selected)),
		brand:     lipgloss.NewStyle().Foreground(lipgloss.Color(palette.brand)),
		reasoning: lipgloss.NewStyle().Foreground(lipgloss.Color(palette.reasoning)),
		context:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.context)),
		working:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.working)),
		success:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.success)),
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

// userMessageBackground returns the Catppuccin surface shared by user messages and the composer.
func userMessageBackground(hasDarkBackground bool, _ color.Color) color.Color {
	return lipgloss.Color(themeFor(hasDarkBackground).palette.surface)
}

// markdownStyle applies Atlas's Catppuccin colors to Glamour's built-in styles.
func markdownStyle(hasDarkBackground bool) glamouransi.StyleConfig {
	style := glamourstyles.LightStyleConfig
	if hasDarkBackground {
		style = glamourstyles.DarkStyleConfig
	}
	theme := themeFor(hasDarkBackground)
	text := theme.palette.text
	selected := theme.palette.selected
	muted := theme.palette.muted
	link := theme.palette.link
	code := theme.palette.code
	syntaxTheme := "catppuccin-latte"
	if hasDarkBackground {
		syntaxTheme = "catppuccin-mocha"
	}

	style.Document.Color = &text
	style.Document.BlockPrefix = ""
	style.Document.Margin = nil
	style.Heading.Color = &selected
	style.H1.Color = &selected
	style.H1.BackgroundColor = nil
	style.H1.Prefix = ""
	style.H6.Color = &selected
	style.HorizontalRule.Color = &muted
	style.Link.Color = &link
	style.LinkText.Color = &link
	style.Image.Color = &link
	style.ImageText.Color = &muted
	style.Code.Color = &code
	style.Code.BackgroundColor = nil
	style.Code.Prefix = ""
	style.Code.Suffix = ""
	style.CodeBlock.Color = &text
	style.CodeBlock.Theme = syntaxTheme
	style.CodeBlock.Chroma = nil
	return style
}
