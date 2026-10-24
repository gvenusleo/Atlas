package gui

import "github.com/egoist/mygo/ui"

// 启动器面板的尺寸：宽 launcherWidth，距窗口上沿 launcherTop；结果列表最高
// launcherHeight，超出滚动。
const (
	launcherWidth  = 560
	launcherHeight = 360
	launcherTop    = 96
)

// launcherDialog 在启动器打开时画它：顶部是输入框，其下是结果，底部是按键提示。
func (a *App) launcherDialog(c *ui.Context, t *ui.Theme) {
	l := &a.launch
	width, height := c.Size()
	ui.DialogBase(c, &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25)).Justify(ui.Start).Padding(launcherTop, 0, 0)
		popupPanel(panel, t).Width(min(launcherWidth, width-2*gutter)).Radius(cardRound).
			Padding(0).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.25))
		if l.query != l.shownQuery {
			l.index, l.shownQuery = 0, l.query
		}
		l.results = a.launcherResults()
		a.launcherInput(c, t)
		a.launcherList(c, t, min(launcherHeight, height-launcherTop-90))
		launcherHelp(c, t)
	})
}

// launcherInput 画输入框，↑↓ 与 Enter 由 launcherKey 处理。
func (a *App) launcherInput(c *ui.Context, t *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Gap(10).Padding(12, 16).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Icon(c, searchIcon).FontSize(iconSize).TextColor(t.TextMuted).Shrink(0)
		ui.TextInputBase(c.Key("launcher-input"), &a.launch.query).AutoFocus().
			Placeholder("搜索会话、操作和文件").Grow(1).MinWidth(0).OnTextKey(a.launcherKey)
	})
}

// launcherKey 处理输入框里的按键：↑↓ 移动高亮，Enter 执行高亮的一项；返回是否已处理。
func (a *App) launcherKey(k ui.TextKey) bool {
	l := &a.launch
	n := len(l.results)
	if n == 0 || k.Mods != 0 {
		return false
	}
	i := l.row(n)
	switch k.Key {
	case ui.KeyUp:
		l.index, l.moved = (i+n-1)%n, true
	case ui.KeyDown:
		l.index, l.moved = (i+1)%n, true
	case ui.KeyEnter:
		a.chooseResult(l.results[i])
	default:
		return false
	}
	return true
}

// launcherList 画结果，每组前有标题；高亮的一项是 Enter 作用的项，键盘与指针选的是同一行。
func (a *App) launcherList(c *ui.Context, t *ui.Theme, maxHeight float32) {
	l := &a.launch
	if len(l.results) == 0 {
		ui.Text(c, "没有匹配").TextColor(t.TextMuted).FontSize(textSize).Padding(24).
			TextAlign(ui.Center)
		return
	}
	selected := l.row(len(l.results))
	ui.Scroll(c).MaxHeight(maxHeight).Children(func() {
		ui.Column(c).Padding(6).Children(func() {
			group := ""
			for i, r := range l.results {
				if r.group != group {
					group = r.group
					ui.Text(c, group).FontSize(smallSize).TextColor(t.TextMuted).
						Padding(8, 10, 4)
				}
				row := launcherRow(c, t, r, func() { a.chooseResult(r) })
				if x, y, over := row.PointerPosition(); over {
					b := row.Bounds()
					l.pointerTo([2]float32{b.X + x, b.Y + y}, i)
				}
				if i == selected {
					row.Background(t.SurfaceHover)
					if l.moved {
						row.ScrollIntoView()
					}
				}
			}
		})
	})
	l.moved = false
}

// launcherRow 画一行结果：图标（会话没有）、名称、说明和行尾；点一下就调用 choose。
func launcherRow(c *ui.Context, t *ui.Theme, r result, choose func()) ui.Element {
	row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(10).
		Padding(6, 10).Radius(controlRound).Label(r.name)
	row.OnClick(choose)
	row.Children(func() {
		if r.icon != nil {
			ui.Image(c, r.icon).Size(smallIconSize, smallIconSize).Shrink(0)
		}
		ui.Text(c, r.name).FontSize(textSize).SingleLine().MinWidth(0)
		ui.Text(c, r.hint).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().
			Grow(1).MinWidth(0)
		ui.Text(c, r.trail).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().Shrink(0)
	})
	return row
}

// launcherHelp 画底部的按键提示。
func launcherHelp(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Gap(16).Padding(8, 16).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
		Children(func() {
			for _, tip := range []string{"↑↓ 选择", "↵ 打开", "esc 关闭"} {
				ui.Text(c, tip).FontSize(smallSize).TextColor(t.TextMuted)
			}
		})
}
