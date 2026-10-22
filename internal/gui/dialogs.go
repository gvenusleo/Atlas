package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// rename 把会话改名；名称为空时清除名称。
func (a *App) rename(id, name string) {
	go func() {
		err := a.agent.Rename(context.Background(),
			protocol.RenameRequest{SessionID: id, Title: strings.TrimSpace(name)})
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			a.refresh()
		})
	}()
}

// remove 删除会话；运行中的会话会被核心拒绝，错误显示在侧栏底部。
func (a *App) remove(id string) {
	go func() {
		err := a.agent.DeleteSession(context.Background(),
			protocol.DeleteSessionRequest{SessionID: id})
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			if ch := a.chats[id]; ch != nil {
				ch.closePanels()
				delete(a.chats, id)
				if a.current == ch {
					a.newChat()
				}
			}
			a.refresh()
		})
	}()
}

// dialogs 构建重命名对话框和删除确认。
func (a *App) dialogs(c *ui.Context) {
	ui.Modal(c, &a.renameOpen, func() {
		ui.Text(c, "重命名会话").FontSize(dialogSize).Bold()
		submit := ui.TextInput(c, &a.renameText).AutoFocus().Width(320).Submitted()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel := ui.Button(c, "取消").Clicked()
			save := ui.PrimaryButton(c, "保存").Clicked()
			if save || submit {
				a.rename(a.renameID, a.renameText)
			}
			if cancel || save || submit {
				a.renameOpen = false
			}
		})
	})
	message := "删除后无法恢复。"
	if ch := a.chats[a.deleteID]; ch != nil {
		if n := ch.dirtyTabs(); n > 0 {
			message = fmt.Sprintf("删除后无法恢复，文件面板里 %d 个未保存的修改也会丢掉。", n)
		}
	}
	if ui.AlertDialog(c, &a.deleteOpen, "删除这个会话？", message, "取消", "删除") == 1 {
		a.remove(a.deleteID)
	}
	a.unsavedDialog(c)
}
