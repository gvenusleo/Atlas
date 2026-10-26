package gui

import (
	"cmp"

	"github.com/egoist/mygo/ui"
)

// panelButtons 画标题栏右端的开关：文件在最右，其左是更改（工作目录是 git 工作树时才有），
// 再左是终端，最左是会话的大纲（提问多时才有）；没有的不留空位。面板打开时用强调色标出。
func (a *App) panelButtons(c *ui.Context, t *ui.Theme) {
	ch := a.current
	slot := 0
	panelButton(c, t, slot, folderIcon, "文件", ch.filesOpen(), a.toggleFiles)
	slot++
	if ch.inRepo() {
		panelButton(c, t, slot, diffIcon, "更改", ch.panelOpen(changesPanel), a.toggleChanges)
		slot++
	}
	panelButton(c, t, slot, terminalIcon, "终端", ch.panelOpen(termPanel), a.toggleTerminal)
	slot++
	if ch.hasOutline() {
		a.outlineButton(c, t, ch, slot)
	}
}

// panelButton 画标题栏右端从右数第 slot 个开关，点一下调用 click。
func panelButton(
	c *ui.Context, t *ui.Theme, slot int, glyph *ui.SVG, label string, open bool, click func(),
) {
	btn := ui.ButtonBase(c)
	styleBarButton(c, t, btn, slot, glyph, label, open)
	btn.OnClick(click)
}

// styleBarButton 把 btn 画成标题栏右端从右数第 slot 个开关，绝对定位，不占标题栏的位置；
// 标题栏给它们留出了空隙（barSlots 个）。
func styleBarButton(
	c *ui.Context, t *ui.Theme, btn ui.Element, slot int, glyph *ui.SVG, label string, open bool,
) {
	top := (titleBarHeight(c) - barButton) / 2
	right := barInset + c.TitleBar().Right + float32(slot)*(barButton+barGap)
	btn.Absolute().Top(top).Right(right).
		Size(barButton, barButton).Radius(controlRound).Label(label).Tooltip(label)
	color := t.TextMuted
	switch {
	case open:
		btn.Background(accentFg(t).Alpha(0.15))
		color = accentFg(t)
	case btn.Hovered():
		btn.Background(buttonFill(t))
	}
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

// stackHandle 是右侧栏里上下两张卡片之间的拖动条，上下拖动调整上面那张卡片的高度。
func (a *App) stackHandle(c *ui.Context) {
	handle := ui.Box(c).Height(handleWidth).Shrink(0).Cursor(ui.CursorResizeNS)
	if _, dy, ok := handle.Dragged(); ok {
		a.stackHeight += dy
	}
}

// columnHandle 是右侧栏里左右两列之间的拖动条，左右拖动调整左列的宽度。
func (a *App) columnHandle(c *ui.Context) {
	handle := ui.Box(c).Width(handleWidth).Shrink(0).Cursor(ui.CursorResizeEW)
	if dx, _, ok := handle.Dragged(); ok {
		a.leftWidth += dx
	}
}

// sideColumn 是主区右侧的一栏：面板的卡片按打开的先后排布。一个占满整栏，两个上下排；
// 三个时前两个上下排在左列，第三个占满右列。
func (a *App) sideColumn(c *ui.Context, t *ui.Theme, ch *chat) {
	top := cardTop(c)
	if len(ch.order) < 3 {
		ui.Column(c.Key("side")).Width(a.sideWidth).Shrink(0).Margin(top, edge, edge, 0).
			Children(func() { a.cardStack(c, t, ch, ch.order, top) })
		return
	}
	// 左列的宽度限制在范围内，两列都留得下最小宽度。
	room := a.sideWidth - handleWidth
	a.leftWidth = min(max(cmp.Or(a.leftWidth, room/2), sideMin), max(room-sideMin, sideMin))
	ui.Row(c.Key("side")).Width(a.sideWidth).Shrink(0).Margin(top, edge, edge, 0).
		AlignItems(ui.Stretch).Children(func() {
		ui.Column(c.Key("side-left")).Width(a.leftWidth).Shrink(0).Children(func() {
			a.cardStack(c, t, ch, ch.order[:2], top)
		})
		a.columnHandle(c)
		ui.Column(c.Key("side-right")).Grow(1).MinWidth(0).Children(func() {
			a.panelCard(c, t, ch, ch.order[2], 0)
		})
	})
}

// cardStack 把一两个面板的卡片竖着排在一列里；两个时中间的拖动条调整上面那张的高度。
func (a *App) cardStack(c *ui.Context, t *ui.Theme, ch *chat, panels []sidePanel, top float32) {
	if len(panels) == 1 {
		a.panelCard(c, t, ch, panels[0], 0)
		return
	}
	// 上面那张的高度限制在范围内，两张卡片都留得下标签栏与一点内容。
	_, window := c.Size()
	room := window - top - edge - handleWidth
	hi := max(room-stackMin, stackMin)
	a.stackHeight = min(max(cmp.Or(a.stackHeight, room/2), stackMin), hi)
	a.panelCard(c, t, ch, panels[0], a.stackHeight)
	a.stackHandle(c)
	a.panelCard(c, t, ch, panels[1], 0)
}

// panelCard 画面板 p 的卡片；height 为 0 时占满所在列余下的高度。
func (a *App) panelCard(c *ui.Context, t *ui.Theme, ch *chat, p sidePanel, height float32) {
	switch p {
	case termPanel:
		a.terminalCard(c, t, ch, height)
	case changesPanel:
		a.changesCard(c, t, ch, height)
	case filesPanel:
		a.filesCard(c, t, ch, height)
	}
}

// widenSide 把右侧栏拉到 filesMin 宽，放得下文件树与内容；原来更窄时记在 prev 里，供
// restoreSide 还原。
func (a *App) widenSide(prev *float32) {
	*prev = 0
	if a.sideWidth < filesMin {
		*prev, a.sideWidth = a.sideWidth, filesMin
	}
}

// restoreSide 在右侧栏没被再拖过的情况下，还原 widenSide 之前的宽度。
func (a *App) restoreSide(prev *float32) {
	if *prev != 0 && a.sideWidth == filesMin {
		a.sideWidth = *prev
	}
	*prev = 0
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
			a.panelCard(c, t, ch, ch.expanded, 0)
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
