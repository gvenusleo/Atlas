package gui

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

// tabBarHeight 是卡片顶部标签栏的高度：卡片上沿离窗口 edge，边框占 1，标签栏高度取标题栏
// 减去上下各 edge 和边框，这样标签与标题栏里的按钮、交通灯在同一条水平线上。
func tabBarHeight(c *ui.Context) float32 {
	return titleBarHeight(c) - 2*edge - 2
}

// tabItem 是标签栏里的一个标签。
type tabItem struct {
	id         int     // 在标签栏里的标识，同一栏里不重复
	icon       *ui.SVG // 标签文字前的图标，没有时为空
	label      string
	note       string // 标签文字后面的次要文字，如终端程序设置的标题
	dirty      bool   // 有没保存的修改：关闭按钮前有一个圆点
	closeLabel string
	pick       func()
	close      func()
}

// tabStrip 画一行标签：放在可横向滚动的行里，太多放不下时滚动。prefix 区分不同标签栏
// 里的标识；只有一个标签时它无从区分，不加底色；reveal 是要滚进可见范围的标签的标识，
// 为 0 时没有，滚过之后清零，之后用户可以自由滚走。
func (a *App) tabStrip(
	c *ui.Context, t *ui.Theme, prefix string, items []tabItem, active int, reveal *int,
) {
	ui.ScrollHorizontal(c).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Center).Children(func() {
		for i, it := range items {
			row := ui.Row(c.Key("tab-"+prefix+strconv.Itoa(it.id))).Height(tabHeight).
				AlignItems(ui.Center).Gap(4).Shrink(0).Radius(controlRound).Padding(0, 4, 0, 8)
			if i == active && len(items) > 1 {
				row.Background(buttonFill(t))
			}
			if *reveal == it.id {
				row.ScrollIntoView()
				a.win.Update(func() { *reveal = 0 })
			}
			row.Children(func() { tabBody(c, t, it) })
		}
	})
}

// tabBody 画一个标签的内容：点文字部分选中它，右侧是关闭按钮。
func tabBody(c *ui.Context, t *ui.Theme, it tabItem) {
	pick := ui.ButtonBase(c).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(it.label)
	pick.OnClick(it.pick)
	pick.Children(func() {
		if it.icon != nil {
			ui.Image(c, it.icon).Size(smallIconSize, smallIconSize).Shrink(0)
		}
		ui.Text(c, it.label).FontSize(textSize).SingleLine().TextColor(t.TextMuted)
		if it.note != "" {
			ui.Text(c, truncate(it.note, 16)).FontSize(textSize).TextColor(t.TextMuted).SingleLine()
		}
	})
	if it.dirty {
		ui.Box(c).Size(7, 7).Shrink(0).Radius(3.5).Background(t.TextMuted)
	}
	iconButton(c, t, closeIcon, it.closeLabel, 18, it.close)
}

// quietButton 是标签栏与卡片顶部的按钮的共同样式：平时没有底色，悬停才有淡底；label 同时
// 是无障碍名称与提示。调用方再设大小与内容。
func quietButton(c *ui.Context, t *ui.Theme, label string, click func()) ui.Element {
	btn := ui.ButtonBase(c).Radius(controlRound).Justify(ui.Center).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	if btn.Hovered() {
		btn.Background(buttonFill(t))
	}
	btn.OnClick(click)
	return btn
}

// iconButton 画一个只有图标的方形小按钮，图标取边长的 0.6。
func iconButton(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, size float32, click func(),
) {
	quietButton(c, t, label, click).Size(size, size).Children(func() {
		ui.Icon(c, glyph).FontSize(size * 0.6).TextColor(t.TextMuted)
	})
}

// textButton 画一个只有小字的按钮；tip 是悬停时的提示。
func textButton(c *ui.Context, t *ui.Theme, label, tip string, click func()) {
	quietButton(c, t, label, click).Height(tabHeight).Padding(0, 8).Tooltip(tip).Children(func() {
		ui.Text(c, label).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
	})
}
