package gui

import "slices"

// sidePanel 是主区右侧的一个面板：终端、更改或文件。
type sidePanel int

const (
	noPanel sidePanel = iota
	termPanel
	changesPanel
	filesPanel
)

// panelOpen 报告面板 p 是否开着。
func (ch *chat) panelOpen(p sidePanel) bool { return slices.Contains(ch.order, p) }

// openPanel 打开面板 p，排在已开着的面板之后；本来就开着时不动。
func (ch *chat) openPanel(p sidePanel) {
	if !ch.panelOpen(p) {
		ch.order = append(ch.order, p)
	}
}

// closePanel 关闭面板 p，其余面板保持原来的先后。
func (ch *chat) closePanel(p sidePanel) {
	ch.order = slices.DeleteFunc(ch.order, func(q sidePanel) bool { return q == p })
}

// togglePanel 显示或收起当前会话的面板 p：别的面板被扩展时，p 已经开着就只还原，没开就还原后
// 打开。open 在打开之后调用，close 在收起之后调用，都可以为 nil；最后让监视与面板一致。
func (a *App) togglePanel(p sidePanel, open, close func()) {
	ch := a.current
	if ch.restoreFor(p) {
		return
	}
	if ch.panelOpen(p) {
		ch.closePanel(p)
		if close != nil {
			close()
		}
	} else {
		ch.openPanel(p)
		if open != nil {
			open()
		}
	}
	a.syncWatch(ch)
}

// restoreFor 在打开或显示面板 p 之前，把被扩展的另一个面板还原；p 本来就开着时返回 true，
// 此时还原就够了，调用方不必再开合它。
func (ch *chat) restoreFor(p sidePanel) bool {
	if ch.expanded == noPanel || ch.expanded == p {
		return false
	}
	ch.expanded = noPanel
	return ch.panelOpen(p)
}

// toggleExpand 把面板 p 扩展到整个主区，已经扩展了就还原。终端换了位置，键盘交还给它。
func (ch *chat) toggleExpand(p sidePanel) {
	if ch.expanded == p {
		ch.expanded = noPanel
	} else {
		ch.expanded = p
	}
	if p == termPanel {
		ch.focusTerm()
	}
}
