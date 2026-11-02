package gui

import "time"

// 本文件管理会话工作目录所在工作树的分支状态；同一工作树的会话共享分支保护与刷新范围。

// branchAge 是分支状态的缓存时长。
const branchAge = 3 * time.Second

// branchState 是会话工作目录所在仓库的当前分支，以及可切换的分支。
type branchState struct {
	dir     string // 解析符号链接后的 Git 目录，独立 worktree 各不相同
	name    string // 分支名；分离头指针时是短哈希
	list    branchList
	at      time.Time // 上次读到分支的时刻
	loading bool      // 正在读
	again   bool      // 读取期间收到刷新请求，完成后再次读取
}

// refreshBranch 在后台重新读取分支；force 为假时使用未过期的缓存。
// 工作目录变更或会话被丢弃后，通过 chat.starts 丢弃在途结果。
func (a *App) refreshBranch(ch *chat, force bool) {
	if !ch.inRepo() {
		ch.branch = branchState{}
		return
	}
	b := &ch.branch
	if b.loading {
		b.again = b.again || force
		return
	}
	if !force && time.Since(b.at) < branchAge {
		return
	}
	b.loading = true
	cwd, starts := ch.cwd, ch.starts
	go func() {
		dir, err := gitDir(cwd)
		var name string
		var list branchList
		if err == nil {
			name, err = readBranch(cwd)
		}
		if err == nil {
			list = readBranches(cwd)
		}
		a.win.Update(func() {
			if ch.starts != starts { // 会话已重建或丢弃
				return
			}
			again := b.again
			if err != nil { // 不是仓库、没有 git：不显示，不报错
				*b = branchState{at: time.Now()}
				return
			}
			*b = branchState{dir: dir, name: name, list: list, at: time.Now()}
			a.watchHead(ch)
			if again {
				a.refreshBranch(ch, true)
			}
		})
	}()
}

// branchBusy 检查同一工作树是否有会话正在运行，避免切换分支替换运行中的文件。
// 运行会话的工作树尚未确定时，暂时禁止切换。
func (a *App) branchBusy(ch *chat) bool {
	if ch.running || ch.branch.dir == "" {
		return true
	}
	for _, other := range a.chats {
		if other.running && other.inRepo() &&
			(other.branch.dir == "" || other.branch.dir == ch.branch.dir) {
			return true
		}
	}
	return false
}

// chooseBranch 在后台切换分支，成功后刷新同一工作树的会话与更改面板。
func (a *App) chooseBranch(ch *chat, name string, remote bool) {
	if a.branchBusy(ch) {
		return
	}
	cwd, starts := ch.cwd, ch.starts
	go func() {
		err := switchBranch(cwd, name, remote)
		a.win.Update(func() {
			if ch.starts != starts {
				return
			}
			if err != nil {
				a.toast = "切换分支失败：" + err.Error()
				return
			}
			a.refreshBranch(ch, true)
			if ch.changes != nil && ch.panelOpen(changesPanel) {
				a.refreshChanges(ch)
			}
			a.branchChanged(ch)
		})
	}()
}

// branchChanged 重新读取同一工作树中其他会话的分支与更改。
func (a *App) branchChanged(ch *chat) {
	for _, other := range a.chats {
		if other == ch || ch.branch.dir == "" || other.branch.dir != ch.branch.dir {
			continue
		}
		a.refreshBranch(other, true)
		if other.changes != nil && other.panelOpen(changesPanel) {
			a.refreshChanges(other)
		}
	}
}
