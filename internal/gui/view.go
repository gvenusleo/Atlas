package gui

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// view 构建主窗口：左侧会话列表，右侧当前会话，以及重命名与删除的对话框。
func (a *App) view(c *ui.Context) {
	t := applyTheme(c)
	a.keyNav = false
	c.OnShortcut(ui.Cmd, ui.KeyB, a.toggleSidebar)
	c.OnShortcut(ui.Cmd, ui.KeyN, a.newChat)
	c.OnShortcut(ui.Ctrl, ui.KeyBackquote, a.toggleTerminal)
	ui.Column(c).Fill().Children(func() {
		a.panes(c, t)
	})
	a.sidebarToggle(c, t)
	a.dialogs(c)
}

// clampSidebar 把侧栏宽度限制在允许的范围内。
func clampSidebar(w float32) float32 {
	return min(max(w, sidebarMin), sidebarMax)
}

// panes 并排放置侧栏与主区。侧栏的展开与收起带动画：
//   - 完全展开时用 mygo 的 Split，分隔条的拖动、光标与键盘操作都由它负责；
//   - 动画途中用固定宽度的列，侧栏内容保持完整宽度、只被裁剪，避免文字被挤压换行；
//   - 完全收起时只有主区。
func (a *App) panes(c *ui.Context, t *ui.Theme) {
	target := float32(1)
	if a.sidebarHidden {
		target = 0
	}
	// 动画值挂在一个不占位的元素上；只动画“展开程度”，拖动宽度时不受动画影响。
	probe := ui.Box(c.Key("sidebar-anim")).Size(0, 0)
	a.shown = probe.AnimateTo("shown", target, ui.Animation{Duration: 200 * time.Millisecond})

	a.sidebarWidth = clampSidebar(a.sidebarWidth)
	switch {
	case a.shown >= 1:
		ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c, t) }, func() { a.main(c, t) }).Fill()
		// Split 只保证两侧各留 40，宽度范围在这里限制，拖动结束的同一帧生效。
		a.sidebarWidth = clampSidebar(a.sidebarWidth)
	case a.shown > 0:
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c.Key("sidebar")).Width(a.sidebarWidth*a.shown).Shrink(0).ClipX().
				BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
				ui.Column(c).Width(a.sidebarWidth).Shrink(0).Grow(1).Children(func() {
					a.sidebar(c, t)
				})
			})
			ui.Column(c.Key("main")).Grow(1).MinWidth(0).Children(func() {
				a.main(c, t)
			})
		})
	default:
		ui.Column(c).Grow(1).Children(func() { a.main(c, t) })
	}
}

// titleBarHeight 是顶部标题栏区域的高度，交通灯在其中垂直居中。
func titleBarHeight(c *ui.Context) float32 {
	return max(c.TitleBar().Height, titleBarMin)
}

// sidebarToggle 在左上角画侧栏开关，紧挨窗口控件，两种状态下位置不变。
func (a *App) sidebarToggle(c *ui.Context, t *ui.Theme) {
	top := (titleBarHeight(c) - barButton) / 2
	btn := ui.ButtonBase(c).Absolute().Top(top).Left(c.TitleBar().Left+6).
		Size(barButton, barButton).Radius(controlRound).Cursor(ui.CursorPointer).
		Label("切换侧栏").Tooltip("切换侧栏")
	if btn.Hovered() {
		btn.Background(t.Text.Alpha(0.08))
	}
	btn.OnClick(a.toggleSidebar)
	btn.Children(func() {
		ui.Icon(c, sidebarIcon).FontSize(16).TextColor(t.TextMuted)
	})
}

// titleBar 画主区顶部的标题栏：可拖动窗口，标题与阅读栏对齐，并避开窗口控件与侧栏开关。
func (a *App) titleBar(c *ui.Context, t *ui.Theme, title string) ui.Element {
	bar := c.TitleBar()
	// 侧栏收起时，标题让开悬浮的侧栏开关；随展开程度平滑过渡。
	left := gutter + (1-a.shown)*max(bar.Left+40-gutter, 0)
	return ui.Row(c).Height(titleBarHeight(c)).Justify(ui.Center).AlignItems(ui.Center).
		Padding(0, gutter+bar.Right, 0, left).DragWindow().Children(func() {
		ui.Text(c, title).Grow(1).MaxWidth(contentWidth).SingleLine()
		a.terminalButton(c, t)
	})
}

// column 把内容放进居中的阅读栏，两侧留白；vertical 是上下留白。
func column(c *ui.Context, vertical float32, fn func()) {
	ui.Row(c).Justify(ui.Center).Padding(vertical, gutter).Children(func() {
		ui.Column(c).Grow(1).MaxWidth(contentWidth).Children(fn)
	})
}

// main 构建主区：对话在左，会话的终端面板打开时，浮动的终端卡片在右侧。
func (a *App) main(c *ui.Context, t *ui.Theme) {
	ch := a.current
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c.Key("chat")).Grow(1).MinWidth(0).Children(func() {
			a.conversation(c, t, ch)
		})
		if ch.termOpen {
			a.termHandle(c)
			a.terminalCard(c, t, ch)
		}
	})
	// 卡片宽度限制在范围内，保证对话区不被挤得太窄。
	window, _ := c.Size()
	total := window - a.sidebarWidth*a.shown
	a.termWidth = min(max(a.termWidth, termMin), max(total-chatMin-handleWidth, termMin))
}

// conversation 构建当前会话：新会话居中欢迎，其余为标题、消息列表和底部输入。
func (a *App) conversation(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c.Key(ch.key)).Fill().Background(t.Background).Children(func() {
		if len(ch.items) == 0 && !ch.loading && !ch.running {
			a.titleBar(c, t, "")
			a.welcome(c, t, ch)
			return
		}
		// 消息列表铺满主区，标题栏浮在它上面，滚到标题栏之下的内容被遮住，下沿略有模糊。
		ui.Box(c).Grow(1).Children(func() {
			a.messages(c, t, ch)
			a.topEdge(c, t)
			a.titleBar(c, t, a.heading(ch)).Absolute().Top(0).Left(0).Right(0).
				Background(t.Background)
		})
		if ch.err != "" {
			column(c, 4, func() {
				ui.Text(c, ch.err).TextColor(t.Danger).FontSize(13).Selectable()
			})
		}
		column(c, 0, func() { a.composer(c, t, ch) })
		ui.Column(c).Height(12)
	})
}

// welcome 在空白的新会话中居中显示问候和输入框。
func (a *App) welcome(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c).Grow(1).Justify(ui.Center).Children(func() {
		column(c, 0, func() {
			ui.Text(c, "今天想做点什么？").FontSize(22).Padding(0, 0, 16).TextAlign(ui.Center)
			a.composer(c, t, ch)
		})
	})
}

// messages 构建消息列表，跟随末尾直到用户上滚。
func (a *App) messages(c *ui.Context, t *ui.Theme, ch *chat) {
	blocks := ch.blocks()
	ch.list.Key = func(i int) any { return blocks[i].items[0].id }
	ch.list.FollowEnd = true
	ch.list.FollowAnimation = &ui.Smooth
	top := titleBarHeight(c)
	ui.Column(c).Fill().Children(func() {
		ui.List(c, &ch.list, len(blocks), func(i int) {
			column(c, 5, func() { a.block(c, t, blocks[i]) })
		}).Grow(1).Padding(top, 0, 8).ScrollbarInsets(top, 0, 0).Children(func() {
			if ch.loading {
				ui.Spinner(c).Margin(ui.Auto)
			}
		})
		if len(blocks) > 0 && !ch.list.AtEnd() {
			a.jumpToLatest(c, t, ch)
		}
	})
}

// topEdge 在标题栏下沿铺一条窄带：滚到标题栏之下的内容在这里略微模糊，
// 并从完全被背景色盖住渐渐显现，所以与标题栏相接处没有突兀的截断。
func (a *App) topEdge(c *ui.Context, t *ui.Theme) {
	top := titleBarHeight(c)
	strip := func() ui.Element {
		return ui.Box(c).Absolute().Top(top).Left(0).Right(0).Height(edgeHeight).PassThrough()
	}
	fade := &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180, End: 1}
	strip().Material(glass.Blur{Radius: 4, Mask: fade})
	strip().Gradient(t.Background, t.Background.Alpha(0), 180)
}

// jumpToLatest 在用户上滚离开末尾时，于列表底部居中显示“回到最新”按钮。
func (a *App) jumpToLatest(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(10).Justify(ui.Center).Children(func() {
		btn := ui.ButtonBase(c).Size(32, 32).Radius(16).
			Background(t.Background).Border(1, t.Border).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.12)).
			Cursor(ui.CursorPointer).Label("回到最新").Tooltip("回到最新")
		if btn.Hovered() {
			btn.Background(t.SurfaceHover)
		}
		btn.OnClick(ch.list.ScrollToEnd)
		btn.Children(func() {
			ui.Icon(c, sendIcon).FontSize(16).TextColor(t.TextMuted).Rotate(180)
		})
	})
}

// heading 返回当前会话的标题。
func (a *App) heading(ch *chat) string {
	for _, info := range a.infos {
		if info.ID == ch.id && info.Name != "" {
			return info.Name
		}
	}
	return "新对话"
}
