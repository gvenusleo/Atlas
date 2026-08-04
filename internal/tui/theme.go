package tui

import (
	"fmt"
	"image/color"
	"math"

	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
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
	dark      bool
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
	lightTheme = newTUITheme(false, colorPalette{
		text: "#202433", muted: "#666B78", divider: "#C6CAD2", surface: "#F0F2F6",
		selected: "#1E66F5", link: "#087EA4", code: "#0F766E", brand: "#2F76F6",
		reasoning: "#7C3AED", context: "#0F766E", working: "#B26A00", success: "#238636", error: "#C93C49",
	})
	darkTheme = newTUITheme(true, colorPalette{
		text: "#E6E9EF", muted: "#9AA2B1", divider: "#4A515D", surface: "#2C3038",
		selected: "#89B4FA", link: "#65C4E8", code: "#69D6C4", brand: "#A8C4FF",
		reasoning: "#C5A3FF", context: "#69D6C4", working: "#F2C66D", success: "#7AD98B", error: "#FF8792",
	})
)

func newTUITheme(dark bool, palette colorPalette) tuiTheme {
	return tuiTheme{
		dark:      dark,
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

// resolveTheme combines Atlas semantic colors with the terminal's native neutrals.
func resolveTheme(hasDarkBackground bool, terminalForeground, terminalBackground color.Color) tuiTheme {
	theme := themeFor(hasDarkBackground)
	palette := theme.palette
	foreground := colorRGB(terminalForeground, palette.text)
	fallbackBackground := "#FFFFFF"
	if hasDarkBackground {
		fallbackBackground = "#111318"
	}
	background := colorRGB(terminalBackground, fallbackBackground)

	palette.text = colorHex(foreground)
	palette.muted = colorHex(blendRGB(foreground, background, 0.62))
	palette.divider = colorHex(blendRGB(foreground, background, 0.22))
	if terminalBackground != nil {
		tint := color.RGBA{A: 0xff}
		alpha := 0.04
		if hasDarkBackground {
			tint = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
			alpha = 0.12
		}
		palette.surface = colorHex(blendRGB(tint, background, alpha))
	}
	return newTUITheme(hasDarkBackground, palette)
}

func colorRGB(value color.Color, fallback string) color.RGBA {
	if value == nil {
		value = lipgloss.Color(fallback)
	}
	r, g, b, _ := value.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}
}

func blendRGB(foreground, background color.RGBA, alpha float64) color.RGBA {
	blend := func(foreground, background uint8) uint8 {
		return uint8(math.Round(float64(foreground)*alpha + float64(background)*(1-alpha)))
	}
	return color.RGBA{
		R: blend(foreground.R, background.R),
		G: blend(foreground.G, background.G),
		B: blend(foreground.B, background.B),
		A: 0xff,
	}
}

func colorHex(value color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", value.R, value.G, value.B)
}

func themeFor(hasDarkBackground bool) tuiTheme {
	if hasDarkBackground {
		return darkTheme
	}
	return lightTheme
}

func userMessageStyle(theme tuiTheme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.palette.text)).
		Background(userMessageBackground(theme)).
		Padding(1, 1)
}

func composerStyle(theme tuiTheme) lipgloss.Style {
	return userMessageStyle(theme).PaddingLeft(0)
}

// userMessageBackground returns the adaptive surface shared by user messages and the composer.
func userMessageBackground(theme tuiTheme) color.Color {
	return lipgloss.Color(theme.palette.surface)
}

// markdownStyle applies Atlas colors to Glamour's built-in styles.
func markdownStyle(theme tuiTheme) glamouransi.StyleConfig {
	style := glamourstyles.LightStyleConfig
	if theme.dark {
		style = glamourstyles.DarkStyleConfig
	}
	text := theme.palette.text
	selected := theme.palette.selected
	muted := theme.palette.muted
	link := theme.palette.link
	code := theme.palette.code
	syntaxTheme := "github"
	if theme.dark {
		syntaxTheme = "github-dark"
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
