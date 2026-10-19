package gui

// sidePanel 是主区右侧的一个面板：终端或文件。
type sidePanel int

const (
	noPanel sidePanel = iota
	termPanel
	filesPanel
)

// panelOpen 报告面板 p 是否开着。
func (ch *chat) panelOpen(p sidePanel) bool {
	switch p {
	case termPanel:
		return ch.termOpen
	case filesPanel:
		return ch.filesOpen()
	}
	return false
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
