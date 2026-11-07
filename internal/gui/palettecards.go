package gui

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// 配色卡片的版式，单位为 DIP。
const (
	paletteColumns = 4  // 每行几张卡片
	sampleHeight   = 44 // 色样的高度
	accentDot      = 9  // 色样里强调色圆点的直径，比其余的大，一眼看出主色
	colorDot       = 5  // 色样里其余圆点的直径
)

// paletteCards 画配色的色板卡片：每张是该配色亮暗两套颜色的色样，点一下选用。
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

// paletteCard 画一张配色卡片：上面是色样，下面是名称。选中的色样外有一圈强调色的描边，
// 名称用正文色；不加粗，免得名称变宽。悬停的卡片是淡底。
func (a *App) paletteCard(c *ui.Context, t *ui.Theme, it schemeChoice, on bool) {
	card := ui.ButtonBase(c.Key("palette-" + it.id)).Grow(1).MinWidth(0).Padding(2).
		Radius(boxRound).Label(it.name)
	if !on && card.Hovered() {
		card.Background(rowFill(t))
	}
	card.OnClick(func() { a.choosePalette(it.id) })
	card.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
			// 描边与色样之间留一道缝；没选中时描边透明，各卡片的色样一样大。
			ring := ui.Column(c).AlignSelf(ui.Stretch).Padding(2).Radius(controlRound+4).
				Border(2, ui.Color{})
			if on {
				ring.BorderColor(accentFg(t))
			}
			ring.Children(func() { paletteSample(c, t, it.scheme) })
			name := ui.Text(c, it.name).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
			if on {
				name.TextColor(t.Text)
			}
		})
	})
}

// paletteSample 画配色的色样：左半是亮色、右半是暗色，配色之间亮色常常相近，
// 差别多在暗色里，并排放才分得开。外框用当前主题的边框色，与周围一致。
func paletteSample(c *ui.Context, t *ui.Theme, sc scheme) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Stretch).Height(sampleHeight).
		Radius(controlRound).Clip().Border(1, t.Border).Children(func() {
		sampleHalf(c, sc.light, sc.lightTokens)
		sampleHalf(c, sc.dark, sc.darkTokens)
	})
}

// sampleHalf 画色样的一半：底色上一行文字色的“Aa”，其下是强调色、成功、运行、危险，
// 以及关键字与函数的代码色。
func sampleHalf(c *ui.Context, p palette, tok [6]ui.Color) {
	ui.Column(c).Grow(1).MinWidth(0).Justify(ui.Center).AlignItems(ui.Center).Gap(4).
		Background(p.canvas).Children(func() {
		ui.Text(c, "Aa").FontSize(smallSize).TextColor(p.text).FontWeight(500).SingleLine()
		ui.Row(c).AlignItems(ui.Center).Gap(2).Children(func() {
			sampleDot(c, accentDot, p.accent).Margin(0, 1, 0, 0)
			for _, color := range []ui.Color{p.success, p.running, p.danger, tok[0], tok[3]} {
				sampleDot(c, colorDot, color)
			}
		})
	})
}

// sampleDot 画色样里的一个圆点。
func sampleDot(c *ui.Context, size float32, color ui.Color) ui.Element {
	return ui.Row(c).Size(size, size).Shrink(0).Radius(size / 2).Background(color)
}
