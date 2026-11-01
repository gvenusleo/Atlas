package gui

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// 配色卡片的版式，单位为 DIP。
const (
	paletteColumns = 4  // 每行几张卡片
	sampleHeight   = 44 // 缩影的高度
)

// paletteCards 画配色的色板卡片：每张是该配色在当前亮暗下的缩影，点一下选用。
// 选中的是偏好里的配色，不另存状态。
func (a *App) paletteCards(c *ui.Context, t *ui.Theme) {
	selected := schemeChoices[schemeIndex(a.prefs.Palette)].id
	ui.Column(c).Gap(4).Children(func() {
		for row := range slices.Chunk(schemeChoices, paletteColumns) {
			ui.Row(c).Gap(4).Children(func() {
				for _, it := range row {
					a.paletteCard(c, t, it, it.id == selected)
				}
				// 最后一行不满时用空位补齐，卡片才与上面各行一样宽。
				for range paletteColumns - len(row) {
					ui.Row(c).Grow(1).MinWidth(0)
				}
			})
		}
	})
}

// paletteCard 画一张配色卡片：上面是缩影，下面是名称。选中的与本应用其他列表一样
// 是淡底，名称加粗并用正文色；悬停的是更淡的底。
func (a *App) paletteCard(c *ui.Context, t *ui.Theme, it schemeChoice, on bool) {
	card := ui.ButtonBase(c.Key("palette-" + it.id)).Grow(1).MinWidth(0).Padding(4).
		Radius(boxRound).Label(it.name)
	switch {
	case on:
		card.Background(t.SurfaceHover)
	case card.Hovered():
		card.Background(rowFill(t))
	}
	card.OnClick(func() { a.choosePalette(it.id) })
	card.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).AlignItems(ui.Center).Children(func() {
			paletteSample(c, t, it.scheme)
			name := ui.Text(c, it.name).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
			if on {
				name.TextColor(t.Text).Bold()
			}
		})
	})
}

// paletteSample 画配色的缩影：主区底色上一条侧栏、一枚强调色的按钮、两行字，
// 以及关键字、字符串、函数的代码色。
func paletteSample(c *ui.Context, t *ui.Theme, sc scheme) {
	p, tok := sc.light, sc.lightTokens
	if t.Dark {
		p, tok = sc.dark, sc.darkTokens
	}
	ui.Row(c).AlignSelf(ui.Stretch).Height(sampleHeight).Radius(controlRound).Clip().
		Background(p.canvas).Border(1, p.border).Children(func() {
		ui.Column(c).Width(20).Gap(4).Padding(8, 5).Background(p.subtle).Children(func() {
			sampleBar(c, 10, p.muted)
			sampleBar(c, 10, p.muted)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Padding(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
				ui.Row(c).Size(16, 8).Shrink(0).Radius(4).Background(p.accent)
				sampleBar(c, 28, p.text)
			})
			ui.Row(c).Gap(4).Children(func() {
				for _, i := range []int{0, 1, 3} {
					ui.Row(c).Size(6, 6).Shrink(0).Radius(3).Background(tok[i])
				}
			})
		})
	})
}

// sampleBar 画缩影里代表一段文字的细条。
func sampleBar(c *ui.Context, width float32, color ui.Color) {
	ui.Row(c).Size(width, 4).Shrink(0).Radius(2).Background(color)
}
