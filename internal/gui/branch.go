package gui

import "time"

// 这个文件是会话当前分支的状态：从工作目录所在的仓库读出来，供状态行的分支下拉使用，并在
// 切换之后让界面跟上。分支属于目录而不属于会话，所以同一个目录的其它会话要一起重读。

// branchAge 是分支结论用多久就要重读，与 gitCheckAge 同量级。
const branchAge = 3 * time.Second

// branchState 是会话工作目录所在仓库的当前分支，以及可切换的分支。
type branchState struct {
	name    string // 分支名；分离头指针时是短哈希
	list    branchList
	at      time.Time // 上次读到分支的时刻
	loading bool      // 正在读
	again   bool      // 读的时候又被要求重读，读完后再读一次，不并发地跑两轮 git
}

// refreshBranch 在后台重读会话的分支；force 为假时刚读过的跳过。工作目录只在重新打开会话时
// 才会换（见 App.start），在途的结果据 chat.starts 认出过时的。
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
		name, err := readBranch(cwd)
		var list branchList
		if err == nil {
			list = readBranches(cwd)
		}
		a.win.Update(func() {
			if ch.starts != starts { // 读的时候重新打开了会话，结果作废
				return
			}
			again := b.again
			if err != nil { // 不是仓库、没有 git：不显示，不报错
				*b = branchState{at: time.Now()}
				return
			}
			*b = branchState{name: name, list: list, at: time.Now()}
			a.watchHead(ch)
			if again {
				a.refreshBranch(ch, true)
			}
		})
	}()
}

// branchBusy 报告 ch 的工作目录里有没有会话正在运行：分支属于目录，运行中的 Agent 脚下的
// 文件不能换掉。
func (a *App) branchBusy(ch *chat) bool {
	if ch.running {
		return true
	}
	for _, other := range a.chats {
		if other.cwd == ch.cwd && other.running {
			return true
		}
	}
	return false
}

// chooseBranch 在后台切到用户选中的分支；成功后重读分支、刷新更改面板，并让同目录的其它会话跟上。
func (a *App) chooseBranch(ch *chat, name string, remote bool) {
	cwd := ch.cwd // 切换期间换了工作目录的，不能切到新目录的仓库里
	go func() {
		err := switchBranch(cwd, name, remote)
		a.win.Update(func() {
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

// branchChanged 让同工作目录的其它会话重读分支：分支属于目录，不属于会话。
func (a *App) branchChanged(ch *chat) {
	for _, other := range a.chats {
		if other == ch || other.cwd != ch.cwd {
			continue
		}
		a.refreshBranch(other, true)
		if other.changes != nil && other.panelOpen(changesPanel) {
			a.refreshChanges(other)
		}
	}
}
