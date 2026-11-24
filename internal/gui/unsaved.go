package gui

import (
	"fmt"
	"maps"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// unsavedAsk 保存本次关闭操作的剩余标签，队首等待用户确认或保存完成。
type unsavedAsk struct {
	ch   *chat
	tabs []*fileTab
	open bool
}

func (a *App) closeFileTab(ch *chat, tb *fileTab) {
	a.closeFileTabs(ch, func(tab *fileTab) bool { return tab == tb })
}

func (a *App) closeFileTabs(ch *chat, match func(*fileTab) bool) {
	if a.unsaved.ch != nil {
		return
	}
	a.unsaved.ch = ch
	for _, tb := range ch.files.tabs {
		if match(tb) {
			a.unsaved.tabs = append(a.unsaved.tabs, tb)
		}
	}
	a.continueClosingTabs()
}

func (a *App) continueClosingTabs() {
	p := &a.unsaved
	for len(p.tabs) != 0 {
		tb := p.tabs[0]
		if tb.saving {
			a.finishClosingTab("文件正在保存，请保存完成后重试")
			return
		}
		if tb.dirty() {
			p.open = true
			return
		}
		p.ch.files.removeTab(tb)
		p.tabs = p.tabs[1:]
	}
	*p = unsavedAsk{}
}

func (a *App) finishClosingTab(message string) {
	p := &a.unsaved
	if message != "" {
		a.toast = relOrAbs(p.ch.cwd, p.tabs[0].path) + "：" + message
	}
	if message != "" || a.current != p.ch || a.showTasks {
		*p = unsavedAsk{}
		return
	}
	p.ch.files.removeTab(p.tabs[0])
	p.tabs = p.tabs[1:]
	a.continueClosingTabs()
}

// unsavedDialog 沿用单文件确认，让每个未保存的文件都能单独决定。
func (a *App) unsavedDialog(c *ui.Context) {
	p := &a.unsaved
	if !p.open {
		return
	}
	if a.current != p.ch || a.showTasks {
		*p = unsavedAsk{}
		return
	}
	tb := p.tabs[0]
	compactModal(c, "unsaved-file", &p.open, func() {
		if !p.open {
			return
		}
		name := relOrAbs(p.ch.cwd, tb.path)
		ui.Text(c, "要保存“"+name+"”的修改吗？").FontSize(dialogSize).Bold()
		ui.Text(c, "不保存的话，这些修改会丢掉").TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").OnClick(func() { *p = unsavedAsk{} })
			ui.Button(c, "不保存").OnClick(func() { a.finishClosingTab("") })
			ui.PrimaryButton(c, "保存").OnClick(func() {
				p.open = false
				a.saveTab(p.ch, tb, a.finishClosingTab)
			})
		})
	})
	if !p.open { // 按 Esc 或点击遮罩时，MyGo 关闭对话框。
		*p = unsavedAsk{}
	}
}

// dirtyTabs 返回所有会话里有未保存修改的标签数。
func (a *App) dirtyTabs() int {
	chats := slices.Collect(maps.Values(a.chats))
	if !slices.Contains(chats, a.current) { // 还没创建的新会话不在 a.chats 里
		chats = append(chats, a.current)
	}
	n := 0
	for _, ch := range chats {
		n += ch.dirtyTabs()
	}
	return n
}

// confirmQuit 在有未保存的修改时拦下退出并打开确认框。关窗口只是隐藏，不经过这里。
func (a *App) confirmQuit(e *mygo.QuitEvent) {
	if e.SessionEnding || a.discardOK {
		return
	}
	n := a.dirtyTabs()
	if n == 0 {
		return
	}
	e.PreventDefault()
	a.win.Update(func() {
		a.quitDirty = n
		a.reveal()
	})
}

// quitDialog 在确认丢弃未保存的修改后接着退出。
func (a *App) quitDialog(c *ui.Context) {
	if a.quitDirty == 0 {
		return
	}
	open := true
	title := fmt.Sprintf("有 %d 个文件的修改还没保存", a.quitDirty)
	confirmed := showAlert(c, "quit-unsaved", &open, title, "现在退出会丢掉这些修改", "丢弃修改并退出")
	if !open {
		a.quitDirty = 0
	}
	if confirmed {
		a.discardOK = true
		mygo.App.Quit()
	}
}
