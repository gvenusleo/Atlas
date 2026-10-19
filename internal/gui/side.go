package gui

import (
	"cmp"

	"github.com/egoist/mygo/ui"
)

// panelButtons 画标题栏右端的两个开关：文件在最右，终端在它左边；面板打开时用强调色标出。
func (a *App) panelButtons(c *ui.Context, t *ui.Theme) {
	ch := a.current
	panelButton(c, t, 0, folderIcon, "文件", ch.filesOpen(), a.toggleFiles)
	panelButton(c, t, 1, terminalIcon, "终端", ch.termOpen, a.toggleTerminal)
}

// panelButton 画标题栏右端从右数第 slot 个开关，绝对定位，不占标题栏的位置；
// 标题栏给它们留出了空隙。
func panelButton(
	c *ui.Context, t *ui.Theme, slot int, glyph *ui.SVG, label string, open bool, click func(),
) {
	top := (titleBarHeight(c) - barButton) / 2
	right := barInset + c.TitleBar().Right + float32(slot)*(barButton+barGap)
	btn := ui.ButtonBase(c).Absolute().Top(top).Right(right).
		Size(barButton, barButton).Radius(controlRound).Cursor(ui.CursorPointer).
		Label(label).Tooltip(label)
	color := t.TextMuted
	switch {
	case open:
		btn.Background(t.Accent.Alpha(0.15))
		color = t.Accent
	case btn.Hovered():
		btn.Background(buttonFill(t))
	}
	btn.OnClick(click)
	btn.Children(func() { ui.Icon(c, glyph).FontSize(iconSize).TextColor(color) })
}

// sideHandle 是对话区与右侧栏之间的拖动条：不可见，悬停时显示左右拖动的光标，
// 左右拖动调整右侧栏的宽度。
func (a *App) sideHandle(c *ui.Context) {
	handle := ui.Box(c).Width(handleWidth).Shrink(0).Cursor(ui.CursorResizeEW)
	if dx, _, ok := handle.Dragged(); ok {
		a.sideWidth -= dx
	}
}

// stackHandle 是右侧栏里上下两张卡片之间的拖动条，上下拖动调整终端卡片的高度。
func (a *App) stackHandle(c *ui.Context) {
	handle := ui.Box(c).Height(handleWidth).Shrink(0).Cursor(ui.CursorResizeNS)
	if _, dy, ok := handle.Dragged(); ok {
		a.termHeight += dy
	}
}

// sideColumn 是主区右侧的一栏：终端与文件面板的卡片竖着叠放，都开着时终端在上，中间的
// 拖动条调整两者的高度；只开一个时它占满整栏。
func (a *App) sideColumn(c *ui.Context, t *ui.Theme, ch *chat) {
	top := cardTop(c)
	both := ch.termOpen && ch.filesOpen()
	var termHeight float32
	if both {
		// 叠放时终端的高度限制在范围内，两张卡片都留得下标签栏与一点内容。
		_, window := c.Size()
		room := window - top - edge - handleWidth
		hi := max(room-stackMin, stackMin)
		a.termHeight = min(max(cmp.Or(a.termHeight, room/2), stackMin), hi)
		termHeight = a.termHeight
	}
	ui.Column(c.Key("side")).Width(a.sideWidth).Shrink(0).Margin(top, edge, edge, 0).
		Children(func() {
			if ch.termOpen {
				a.terminalCard(c, t, ch, termHeight)
			}
			if both {
				a.stackHandle(c)
			}
			if ch.filesOpen() {
				a.filesCard(c, t, ch)
			}
		})
}

// cardTop 是浮动卡片离窗口顶部的距离；窗口控件在右上角的平台上，卡片从标题栏下方开始。
func cardTop(c *ui.Context) float32 {
	if c.TitleBar().Right > 0 {
		return titleBarHeight(c)
	}
	return edge
}

// expandedPanel 画扩展到整个主区的面板：从窗口顶部铺到底，对话区与另一个面板暂时不画。
// 侧栏收起时，窗口控件与侧栏开关浮在主区左上角，卡片随侧栏的收起让出一条标题栏高度的空带，
// 带上仍能拖动窗口。
func (a *App) expandedPanel(c *ui.Context, t *ui.Theme, ch *chat) {
	top := cardTop(c)
	top += (1 - a.shown) * max(titleBarHeight(c)-top, 0)
	ui.Column(c.Key("expanded")).Fill().Children(func() {
		ui.Row(c).Height(top).Shrink(0).DragWindow()
		ui.Column(c).Grow(1).MinHeight(0).Margin(0, edge, edge, edge).Children(func() {
			if ch.expanded == termPanel {
				a.terminalCard(c, t, ch, 0)
			} else {
				a.filesCard(c, t, ch)
			}
		})
	})
}

// cardActions 画卡片标签栏右端的两个按钮：把面板 p 扩展到整个主区（扩展着时是还原），
// 以及关闭。
func (a *App) cardActions(
	c *ui.Context, t *ui.Theme, ch *chat, p sidePanel, closeLabel string, onClose func(),
) {
	glyph, label := expandIcon, "扩展面板"
	if ch.expanded == p {
		glyph, label = restoreIcon, "还原面板"
	}
	iconButton(c, t, glyph, label, barButton, func() { ch.toggleExpand(p) })
	iconButton(c, t, closeIcon, closeLabel, barButton, onClose)
}

// sideCard 是右侧栏里的一张浮动卡片：圆角边框；height 为 0 时占满栏里余下的高度，
// 否则是这个高度。
func sideCard(c *ui.Context, t *ui.Theme, key string, height float32) ui.Element {
	card := ui.Column(c.Key(key)).Radius(cardRound).Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.06)).Clip() // 内容是方角的，裁到卡片的圆角里
	if height > 0 {
		return card.Height(height).Shrink(0)
	}
	return card.Grow(1).MinHeight(0)
}
