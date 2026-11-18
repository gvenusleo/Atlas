package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// overlayPanel 以单层阴影区分面板和底下的内容。阴影和圆角与 MyGo 的字体下拉一致。
func overlayPanel(panel ui.Element, t *ui.Theme) ui.Element {
	return panel.Background(t.Background).Radius(popupRound).Shadow(0, 6, 20, 0,
		ui.RGBA(0, 0, 0, 0.18))
}

// popupPanel 是下拉、日历和补全列表的面板，触发元素与面板的间距由各处自己决定。
func popupPanel(panel ui.Element, t *ui.Theme) ui.Element {
	return overlayPanel(panel, t).Padding(4).Border(1, t.Border)
}

// modalPanel 给模态对话框应用同一遮罩，并防止面板贴到窗口边缘。
func modalPanel(c *ui.Context, backdrop, panel ui.Element) ui.Element {
	t := c.Theme()
	width, height := c.Size()
	backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
	return overlayPanel(panel, t).MaxWidth(width - t.Space(10)).MaxHeight(height - t.Space(10))
}

// showModal 供内容自行排版的对话框使用。
func showModal(c *ui.Context, key string, open *bool, fn func()) ui.Element {
	if !*open {
		return ui.Element{}
	}
	return ui.DialogBase(c.Key(key), open, func(backdrop, panel ui.Element) {
		modalPanel(c, backdrop, panel)
		fn()
	})
}

// compactModal 沿用 MyGo 小型对话框的内容间距。
func compactModal(c *ui.Context, key string, open *bool, fn func()) {
	t := c.Theme()
	showModal(c, key, open, fn).Padding(t.Space(5)).Gap(t.Space(3))
}

// showAlert 使用同一外观。只有按钮或 Esc 可以关闭确认，最后一个按钮是默认操作。
func showAlert(c *ui.Context, key string, open *bool, title, message string, buttons ...string) int {
	if !*open {
		return -1
	}
	t := c.Theme()
	width, _ := c.Size()
	chosen := -1
	ui.Overlay(c, func() {
		// 点击遮罩时保留默认按钮的焦点，避免下一次按回车落到首个按钮。
		backdrop := ui.Box(c.Key(key)).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Center().Modal().KeepFocus()
		if backdrop.OverlayShortcut(0, ui.KeyEscape) {
			chosen = ui.AlertDismissed
		}
		backdrop.Children(func() {
			panel := ui.Column(c).Role(ui.RoleAlertDialog).Label(title).Description(message)
			modalPanel(c, backdrop, panel).MinWidth(min(t.Space(75), width-t.Space(10))).
				Padding(t.Space(5)).Gap(t.Space(3)).Children(func() {
				ui.Text(c, title).FontSize(dialogSize).Bold().Width(0).MinWidthPercent(100)
				if message != "" {
					ui.Text(c, message).TextColor(t.TextMuted).Width(0).MinWidthPercent(100)
				}
				ui.Row(c).Wrap().Gap(t.Space(2)).Justify(ui.End).Margin(t.Space(2), 0, 0, 0).
					Children(func() {
						for i, label := range buttons {
							var button ui.Element
							if i == len(buttons)-1 {
								button = ui.PrimaryButton(c, label).AutoFocus()
							} else {
								button = ui.Button(c, label)
							}
							if button.MaxWidthPercent(100).Clicked() {
								chosen = i
							}
						}
					})
			})
		})
	})
	if chosen != -1 {
		*open = false
	}
	return chosen
}

// rename 把会话改名；名称为空时清除名称。
func (a *App) rename(id, name string) {
	go func() {
		err := a.service.RenameSession(context.Background(),
			protocol.RenameSessionRequest{SessionID: id, Title: strings.TrimSpace(name)})
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
		err := a.service.DeleteSession(context.Background(),
			protocol.DeleteSessionRequest{SessionID: id})
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
				return
			}
			a.dropChat(id)
			a.refresh()
			a.refreshHistory() // 删的可能是任务会话
		})
	}()
}

// dialogs 构建重命名对话框和删除确认。
func (a *App) dialogs(c *ui.Context) {
	compactModal(c, "rename-session", &a.renameOpen, func() {
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
	message := "删除后无法恢复"
	if ch := a.chats[a.deleteID]; ch != nil {
		if n := ch.dirtyTabs(); n > 0 {
			message = fmt.Sprintf("删除后无法恢复，文件面板里 %d 个未保存的修改也会丢掉", n)
		}
	}
	if showAlert(c, "delete-session", &a.deleteOpen,
		"删除这个会话？", message, "取消", "删除") == 1 {
		a.remove(a.deleteID)
	}
	a.cwdGoneDialog(c)
	a.reopenDialog(c)
	a.unsavedDialog(c)
	a.fileCreateDialog(c)
	a.groupDeleteDialog(c)
}

// groupDeleteDialog 提示将被永久删除的会话与未保存的文件修改。
func (a *App) groupDeleteDialog(c *ui.Context) {
	p := &a.groupDelete
	if p.deleting {
		if showAlert(c, "group-delete-progress", &p.deleting,
			"正在删除会话…", "") == ui.AlertDismissed {
			p.deleting = true
		}
		return
	}
	if !p.open {
		return
	}
	dirty := 0
	for _, id := range p.ids {
		if ch := a.chats[id]; ch != nil {
			dirty += ch.dirtyTabs()
		}
	}
	message := p.cwd + "\n删除后无法恢复"
	if dirty > 0 {
		message += fmt.Sprintf("，文件面板里 %d 个未保存的修改也会丢失", dirty)
	}
	title := fmt.Sprintf("删除此目录的 %d 个会话？", len(p.ids))
	result := showAlert(c, "group-delete", &p.open, title, message, "删除", "取消")
	if result == 0 {
		a.deleteGroup()
	} else if !p.open {
		*p = groupDeletion{}
	}
}

// reopenDialog 让用户在会话失效后显式恢复，失败时保留对话框供再次尝试。
func (a *App) reopenDialog(c *ui.Context) {
	ch := a.reopenChat
	if ch == nil || a.current != ch || a.showTasks || a.chats[ch.id] != ch {
		a.reopenOpen = false
		return
	}
	compactModal(c, "reopen-dialog", &a.reopenOpen, func() {
		ui.Text(c, "需要重新打开会话").FontSize(dialogSize).Bold()
		ui.Text(c, ch.err).TextColor(c.Theme().TextMuted).Width(360)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.reopenOpen = false })
			ui.PrimaryButton(c, "重新打开").
				Disabled(ch.running || ch.loading || ch.paging).
				OnClick(func() { a.reopen(ch) })
		})
	})
}

// cwdGoneDialog 在会话的工作目录已不存在时让用户新建目录或删除会话。取消后会话仍然打不开，
// 原因留在会话页上。
func (a *App) cwdGoneDialog(c *ui.Context) {
	ch := a.cwdGoneChat
	if ch == nil || (ch.id != "" && a.chats[ch.id] != ch) {
		a.cwdGoneOpen = false
		return
	}
	t := c.Theme()
	compactModal(c, "missing-cwd", &a.cwdGoneOpen, func() {
		ui.Text(c, "工作目录不存在").FontSize(dialogSize).Bold()
		ui.Text(c, "工作目录 "+shortPath(ch.cwd)+" 已不存在。可以在原路径新建一个空目录，"+
			"或删除这个会话；删除后无法恢复").TextColor(t.TextMuted).Width(360)
		if a.cwdGoneErr != "" {
			ui.Text(c, a.cwdGoneErr).TextColor(t.Danger).Width(360)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel := ui.Button(c, "取消").Clicked()
			del := false
			if ch.id != "" {
				del = ui.Button(c, "删除会话").Clicked()
			}
			rebuild := ui.PrimaryButton(c, "新建目录").Clicked()
			switch {
			case rebuild:
				a.createCwd(ch)
			case del:
				a.cwdGoneOpen, a.cwdGoneErr = false, ""
				a.remove(ch.id)
			case cancel:
				a.cwdGoneOpen, a.cwdGoneErr = false, ""
			}
		})
	})
}
