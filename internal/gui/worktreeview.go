package gui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) worktreeMenu(c *ui.Context, t *ui.Theme, ch *chat, anchor ui.Element) {
	d := a.worktrees
	if d == nil || d.ch != ch || !d.open || !a.worktreePickerCurrent(d) {
		return
	}
	fresh, busy := ch.fresh(), a.worktreeBusy()
	ui.PopoverBase(c.Key("worktree-menu"), anchor, &d.open, func(panel ui.Element) {
		popupPanel(panel, t).MinWidth(anchor.Bounds().W).
			Margin(6, 0, 6, 0).FocusGroup(ui.Vertical)
		ui.Scroll(c.Key("worktrees")).MaxHeight(dropdownMaxHeight).Children(func() {
			focused := false
			focusFirst := func(row ui.Element, enabled bool) {
				if !focused && enabled {
					row.AutoFocus()
					focused = true
				}
			}
			for _, entry := range d.list.entries {
				if entry.bare || entry.missing || entry.cwd == d.list.root {
					continue
				}
				focusFirst(a.worktreeMenuEntry(c.Key(entry.cwd), t, d, entry), !busy)
			}
			if fresh {
				focusFirst(directoryMenuItem(c.Key("choose-folder"), t, "选择文件夹…", func() {
					d.open = false
					a.pickDirectory(ch, func(cwd string) { a.useWorktree(d, cwd) })
				}), true)
			}
			if d.list.common != "" {
				focusFirst(directoryMenuItem(c.Key("create-worktree"), t, "新建工作树…", func() {
					d.open, d.createOpen = false, true
					d.name, d.branch, d.createErr = "", "", ""
				}).Disabled(busy), !busy)
			}
		})
	})
}

func (a *App) worktreeMenuEntry(
	c *ui.Context, t *ui.Theme, d *worktreePicker, entry worktreeEntry,
) ui.Element {
	branch := entry.branch
	if branch == "" {
		branch = entry.head[:min(8, len(entry.head))]
	}
	name := filepath.Base(entry.cwd)
	if entry.main && branch != "" {
		name = branch
	} else if branch != "" && branch != name {
		name += " · " + branch
	}
	return directoryMenuItem(c, t, name, func() { a.useExistingWorktree(d, entry) }).
		Tooltip(shortPath(entry.cwd)).Disabled(a.worktreeBusy()).ContextMenu(func(m *ui.Menu) {
		dirItems(m, entry.cwd)
		if worktreeRemoveReason(*d.list, entry) == "" {
			m.Separator()
			if m.Item("移除工作树…").Disabled(a.worktreeBusy()).Chosen() {
				a.inspectWorktreeRemoval(d, entry)
			}
		}
	})
}

func directoryMenuItem(c *ui.Context, t *ui.Theme, label string, run func()) ui.Element {
	row := popupItem(ui.ButtonBase(c).Label(label).Justify(ui.Start).
		AlignItems(ui.Center).FocusRing(false), t)
	if row.Hovered() || row.FocusVisible() {
		row.Background(rowFill(t))
	}
	return row.OnClick(run).Children(func() {
		ui.Text(c, label).FontSize(smallSize).SingleLine().MinWidth(0)
	})
}

func (a *App) worktreeDialogs(c *ui.Context) {
	d := a.worktrees
	if d == nil || !a.worktreePickerCurrent(d) {
		return
	}
	if d.createOpen {
		a.worktreeCreateDialog(c, d)
	}
	if cwd := d.confirmCwd; cwd != "" {
		open := true
		if showAlert(c, "worktree-directory", &open, "关闭终端并更换工作目录？",
			"更换到 "+shortPath(cwd)+" 将关闭当前会话的终端", "关闭终端并更换") &&
			d.ch.fresh() && a.canChangeDirectory(d.ch) {
			a.setCwd(d.ch, cwd)
		}
		if !open {
			d.confirmCwd = ""
		}
	}
	if p := d.removal; p != nil {
		a.worktreeRemovalDialog(c, d, p)
	}
}

func (a *App) worktreeCreateDialog(c *ui.Context, d *worktreePicker) {
	busy := a.worktreeBusy()
	compactModal(c, "new-worktree", &d.createOpen, func() {
		ui.Text(c, "新建工作树").FontSize(dialogSize).Bold()
		ui.Column(c.Key("worktree-fields")).Width(360).Gap(6).Children(func() {
			fieldRow(c, "工作树名称", func() {
				textField(c, c.Theme(), "name", &d.name, "留空自动生成").Grow(1).MinWidth(0).
					Label("工作树名称").AutoFocus().Disabled(busy).
					OnSubmit(func() { a.createWorktree(d) })
			})
			fieldRow(c, "分支名称", func() {
				textField(c, c.Theme(), "branch", &d.branch, "留空自动生成").Grow(1).MinWidth(0).
					Label("分支名称").Disabled(busy).OnSubmit(func() { a.createWorktree(d) })
			})
		})
		if d.createErr != "" {
			ui.Text(c, d.createErr).Width(360).TextColor(c.Theme().Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			cancel, label := "取消", "创建"
			if a.creatingWorktree {
				cancel, label = "关闭", "正在创建…"
			}
			ui.Button(c.Key("cancel"), cancel).OnClick(func() { d.createOpen = false })
			ui.PrimaryButton(c.Key("create"), label).Disabled(busy).
				OnClick(func() { a.createWorktree(d) })
		})
	})
}

func (a *App) worktreeRemovalDialog(c *ui.Context, d *worktreePicker, p *worktreeRemoval) {
	if p.loading || a.worktreeBlocked(p.entry.cwd) {
		return
	}
	var message strings.Builder
	fmt.Fprintf(&message, "%s\n%s\n检出目录将移入废纸篓，分支和会话历史会保留",
		p.entry.branch, shortPath(p.entry.cwd))
	if len(p.refs) != 0 {
		message.WriteString("\n关联会话")
		for _, ref := range p.refs {
			name := ref.Title
			if name == "" {
				name = "新会话"
			}
			message.WriteString("\n" + name)
		}
	}
	open := true
	if showAlert(c, "remove-worktree", &open, "移除工作树？", message.String(), "移除工作树") {
		a.removeWorktree(d, p)
	}
	if !open {
		d.removal = nil
	}
}
