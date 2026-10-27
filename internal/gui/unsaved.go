package gui

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// unsavedAsk 是关闭有未保存修改的文件标签时的确认：哪个会话的哪个标签。
type unsavedAsk struct {
	ch   *chat
	tb   *fileTab
	open bool
}

// closeFileTab 关闭标签；有未保存的修改时先问保存、不保存还是取消。
func (a *App) closeFileTab(ch *chat, tb *fileTab) {
	if !tb.dirty() {
		ch.files.remove(tb)
		return
	}
	a.unsaved = unsavedAsk{ch, tb, true}
}

// unsavedDialog 在关闭有未保存修改的文件标签时问：保存、不保存，还是取消。
func (a *App) unsavedDialog(c *ui.Context) {
	ui.Modal(c, &a.unsaved.open, func() {
		ch, tb := a.unsaved.ch, a.unsaved.tb
		if tb == nil {
			return
		}
		ui.Text(c, "要保存“"+filepath.Base(tb.path)+"”的修改吗？").FontSize(dialogSize).Bold()
		ui.Text(c, "不保存的话，这些修改会丢掉。").TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel := ui.Button(c, "取消").Clicked()
			discard := ui.Button(c, "不保存").Clicked()
			save := ui.PrimaryButton(c, "保存").Clicked()
			switch {
			case save:
				a.saveTab(tb, func() { ch.files.remove(tb) })
			case discard:
				ch.files.remove(tb)
			}
			if cancel || discard || save {
				a.unsaved.open = false
			}
		})
	})
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

// confirmQuit 在有未保存的修改时拦下退出，问用户是否丢弃；确认后再退出一次。关窗口只是隐藏
// 窗口，不经过这里。
func (a *App) confirmQuit(e *mygo.Preventable) {
	n := a.dirtyTabs()
	if n == 0 || a.discardOK {
		return
	}
	e.PreventDefault()
	a.reveal() // 窗口可能藏在后台，确认框要有地方出现
	go func() {
		r, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: a.win, Type: mygo.MessageWarning, Buttons: []string{"丢弃修改并退出", "取消"},
			DefaultButton: 1, CancelButton: 1,
			Message: fmt.Sprintf("有 %d 个文件的修改还没保存", n),
			Detail:  "现在退出会丢掉这些修改。",
		})
		if err == nil && r.Button == 0 {
			a.win.Update(func() {
				a.discardOK = true
				mygo.App.Quit()
			})
		}
	}()
}
