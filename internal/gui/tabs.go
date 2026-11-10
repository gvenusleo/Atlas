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

// tabFade 是标签栏一端渐隐的宽度。
const tabFade = 24

// tabBar 是一个标签栏要跨帧记住的状态。
type tabBar struct {
	reveal int            // 要滚进可见范围的标签的标识，为 0 时没有；滚过之后清零，之后可以自由滚走
	scroll ui.ScrollState // 横向滚到的位置
}

// tabStrip 画一行标签：放在可横向滚动的行里，太多放不下时滚动。prefix 区分不同标签栏
// 里的标识；只有一个标签时它无从区分，不加底色。滚动条浮在标签上会挡住文字和点击，所以
// 不显示，改由两端的渐隐提示还有标签没露出来。
func (a *App) tabStrip(
	c *ui.Context, t *ui.Theme, prefix string, items []tabItem, active int, bar *tabBar,
) {
	strip := ui.ScrollHorizontal(c).TrackScroll(&bar.scroll).Scrollbars(false).
		Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Center)
	strip.HandleInput(func(ev ui.InputEvent) bool { return scrollSideways(&bar.scroll, ev) })
	if fade, ok := tabFadeMask(c, &bar.scroll); ok {
		strip.MaskLinearGradient(fade)
	}
	strip.Children(func() {
		for i, it := range items {
			row := ui.Row(c.Key("tab-"+prefix+strconv.Itoa(it.id))).Height(tabHeight).
				AlignItems(ui.Center).Gap(4).Shrink(0).Radius(controlRound).Padding(0, 4, 0, 8)
			if i == active && len(items) > 1 {
				row.Background(buttonFill(t))
			}
			if bar.reveal == it.id {
				row.ScrollIntoView()
				a.win.Update(func() { bar.reveal = 0 })
			}
			row.Children(func() { tabBody(c, t, it) })
		}
	})
}

// scrollSideways 把标签栏上以上下为主的滚动转成左右：横向滚动的行只认左右，只有上下滚轮的
// 鼠标在没有滚动条时就滚不动它。
func scrollSideways(s *ui.ScrollState, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll || ev.DY*ev.DY <= ev.DX*ev.DX {
		return false
	}
	s.X = min(max(s.X+ev.DY, 0), s.MaxX)
	return true
}

// tabFadeMask 返回标签栏的遮罩：哪一端还有没露出来的标签，哪一端就渐隐；两端都没有时不加。
// 宽度取上一帧的，第一帧还不知道，就先不加。
func tabFadeMask(c *ui.Context, s *ui.ScrollState) (ui.LinearGradient, bool) {
	w := s.Bounds(c).W
	start, end := s.X > 0.5, s.X < s.MaxX-0.5
	if w <= 0 || !start && !end {
		return ui.LinearGradient{}, false
	}
	f := min(tabFade/w, 0.5)
	stop := func(at float32, fades bool) ui.GradientStop {
		if fades {
			return ui.GradientStop{Color: ui.Transparent, At: at}
		}
		return ui.GradientStop{Color: ui.RGB(0, 0, 0), At: at}
	}
	stops := []ui.GradientStop{stop(0, start), stop(f, false), stop(1-f, false), stop(1, end)}
	return ui.LinearGradient{Angle: 90, Stops: stops}, true
}

// tabBody 画一个标签的内容：点文字部分选中它，右侧是关闭按钮。
func tabBody(c *ui.Context, t *ui.Theme, it tabItem) {
	pick := ui.ButtonBase(c).Gap(6).AlignItems(ui.Center).Label(it.label)
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

// quietButton 是标签栏与卡片顶部的按钮的共同样式：平时没有底色，悬停与按下才有淡底；label
// 是无障碍名称。调用方再设大小与内容；只有图标的按钮由调用方加上同名的提示。
func quietButton(c *ui.Context, t *ui.Theme, label string, click func()) ui.Element {
	btn := ui.ButtonBase(c).Radius(controlRound).Justify(ui.Center).AlignItems(ui.Center).
		Label(label)
	switch {
	case btn.Pressed():
		btn.Background(pressFill(t))
	case btn.Hovered():
		btn.Background(buttonFill(t))
	}
	btn.OnClick(click)
	return btn
}

// iconButton 画一个只有图标的方形小按钮，图标取边长的 0.6；看不到文字，悬停时给出名称。
func iconButton(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, size float32, click func(),
) {
	quietButton(c, t, label, click).Size(size, size).Tooltip(label).Children(func() {
		ui.Icon(c, glyph).FontSize(size * 0.6).TextColor(t.TextMuted)
	})
}

// textButton 画一个只有小字的按钮；tip 是悬停时的提示。
func textButton(c *ui.Context, t *ui.Theme, label, tip string, click func()) ui.Element {
	btn := quietButton(c, t, label, click).Height(tabHeight).Padding(0, 8).Tooltip(tip)
	btn.Children(func() {
		ui.Text(c, label).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
	})
	return btn
}
