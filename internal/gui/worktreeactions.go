package gui

import (
	"context"
	"time"
)

const worktreeWriteTimeout = 2 * time.Minute

type worktreePicker struct {
	ch         *chat
	starts     int
	cwd        string
	open       bool
	createOpen bool
	name       string
	branch     string
	createErr  string
	list       *worktreeList
	confirmCwd string
	removal    *worktreeRemoval
}

func (a *App) openWorktrees(ch *chat) {
	if a.current != ch || a.showTasks {
		return
	}
	if d := a.worktrees; d != nil && a.worktreePickerCurrent(d) && (d.open || d.list == nil) {
		a.worktrees = nil
		return
	}
	d := &worktreePicker{ch: ch, starts: ch.starts, cwd: ch.cwd}
	a.worktrees = d
	cwd := d.cwd
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeReadTimeout)
		defer cancel()
		list, err := readWorktrees(ctx, cwd)
		a.win.Update(func() {
			if !a.worktreePickerCurrent(d) {
				return
			}
			if err != nil {
				a.toast = "读取工作树失败：" + err.Error()
			}
			// 先读完再展开，弹出时选项与位置均已确定。
			d.list, d.open = &list, err == nil || ch.fresh()
		})
	}()
}

func (a *App) worktreePickerCurrent(d *worktreePicker) bool {
	return a.worktrees == d && a.current == d.ch && !a.showTasks &&
		d.ch.starts == d.starts && d.ch.cwd == d.cwd
}

func (a *App) useExistingWorktree(d *worktreePicker, entry worktreeEntry) {
	if !a.worktreePickerCurrent(d) || !d.open || a.worktreeBusy() {
		return
	}
	d.open = false
	common, fresh := d.list.common, d.ch.fresh()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeReadTimeout)
		defer cancel()
		err := checkWorktreeCwd(ctx, entry.cwd, entry.cwd, common)
		a.win.Update(func() {
			if !a.worktreePickerCurrent(d) || d.ch.fresh() != fresh {
				return
			}
			if err != nil {
				a.toast = "工作树已不可用：" + shortPath(entry.cwd)
				return
			}
			a.useWorktree(d, entry.cwd)
		})
	}()
}

func (a *App) useWorktree(d *worktreePicker, cwd string) {
	if !a.worktreePickerCurrent(d) || a.worktreeBlocked(cwd) || d.cwd == cwd {
		return
	}
	ch := d.ch
	if !ch.fresh() {
		a.newChatAt(cwd)
		return
	}
	if !a.canChangeDirectory(ch) {
		a.toast = "请先处理未保存的文件或等待当前操作完成"
		return
	}
	if len(ch.terms) > 0 {
		d.confirmCwd = cwd
		return
	}
	a.setCwd(ch, cwd)
}

func (a *App) createWorktree(d *worktreePicker) {
	if !a.worktreePickerCurrent(d) || !d.createOpen || a.worktreeBusy() || d.list.common == "" {
		return
	}
	ch := d.ch
	if ch.fresh() && !a.canChangeDirectory(ch) {
		d.createErr = "请先处理未保存的文件或等待当前操作完成"
		return
	}
	dir, err := a.prefs.worktreeDirectory()
	if err != nil {
		d.createErr = err.Error()
		return
	}
	cwd, common, name, branch := d.cwd, d.list.common, d.name, d.branch
	fresh := ch.fresh()
	d.createErr = ""
	a.creatingWorktree = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeWriteTimeout)
		defer cancel()
		target, err := createAtlasWorktree(ctx, cwd, dir, common, name, branch)
		a.win.Update(func() { a.finishWorktreeCreate(d, target, fresh, err) })
	}()
}

func (a *App) finishWorktreeCreate(d *worktreePicker, target string, fresh bool, err error) {
	a.creatingWorktree = false
	current := a.worktreePickerCurrent(d) && d.createOpen
	if err != nil {
		if current {
			d.createErr = err.Error()
		} else {
			a.toast = "新建工作树失败：" + err.Error()
		}
		return
	}
	d.createOpen = false
	a.toast = "已新建工作树：" + shortPath(target)
	if current && d.ch.fresh() == fresh {
		a.useWorktree(d, target)
	}
}

func (a *App) canChangeDirectory(ch *chat) bool {
	return a.switchingBranches[ch.branch.dir] == "" && ch.dirtyTabs() == 0 && !ch.savingTab() &&
		(a.fileCreate == nil || a.fileCreate.ch != ch || !a.fileCreate.creating)
}
