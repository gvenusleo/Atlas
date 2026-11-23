package gui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 这个文件是界面对核心的会话操作：打开、加载历史、发送、换配置、关闭。
// 都在后台调用核心，结果经 Window.Update 回到主线程；后台 goroutine 启动前先取出所需的值。

// open 切换到已有会话，首次打开时加载最近一页历史并让核心打开它。历史直接读库，
// 所以会话打不开（比如工作目录没了）也能看。
func (a *App) open(id string) {
	info := protocol.SessionInfo{SessionID: id}
	if i := slices.IndexFunc(a.infos, func(s protocol.SessionInfo) bool {
		return s.SessionID == id
	}); i >= 0 {
		info = a.infos[i]
	}
	a.openSession(info)
}

// openRun 从定时任务视图打开一个任务会话；任务会话不在侧栏的列表里，info 来自视图的列表。
// 标题栏上有回到定时任务视图的返回。
func (a *App) openRun(info protocol.SessionInfo) {
	if a.openSession(info) {
		a.fromTasks = true
	}
}

// openSession 切换到 info 描述的已有会话，返回是否切换了：有未保存的修改而用户取消时没有。
func (a *App) openSession(info protocol.SessionInfo) bool {
	if !a.leave() {
		return false
	}
	a.worktrees = nil
	a.showTasks, a.fromTasks = false, false
	a.cwdGoneOpen, a.cwdGoneErr, a.reopenOpen = false, "", false
	id := info.SessionID
	if ch := a.chats[id]; ch != nil {
		a.current, ch.unread = ch, false
		if ch.cwdGone && !ch.loading { // 目录可能已经恢复，重新打开一次
			a.openSaved(ch)
		} else if ch.needsReopen {
			a.askReopen(ch)
		}
		return true
	}
	a.keys++
	ch := &chat{key: "chat-" + strconv.Itoa(a.keys), id: id, cwd: info.Cwd, title: info.Title}
	a.chats[id], a.current = ch, ch
	a.openSaved(ch)
	return true
}

// askReopen 保留失效状态；后台会话等用户切回后再提示，避免打断当前会话。
func (a *App) askReopen(ch *chat) {
	ch.ready, ch.needsReopen = false, true
	if a.current == ch && !a.showTasks {
		a.reopenChat, a.reopenOpen = ch, true
	}
}

// reopen 从数据库恢复会话与最近一页历史，保留界面的草稿、附件和本地面板。
func (a *App) reopen(ch *chat) {
	if ch.id == "" || ch.run.Running || ch.loading || ch.paging || a.worktreeBlocked(ch.cwd) {
		return
	}
	a.openSaved(ch)
}

// openSaved 先订阅再按订阅边界读历史。即使会话打不开，历史也可以单独读取。
func (a *App) openSaved(ch *chat) {
	if ch.loading || ch.paging {
		return
	}
	ch.ready, ch.loading, ch.choosing = false, true, false
	ch.starts++
	id, gen, cwd := ch.id, ch.starts, ch.cwd
	record, blocked := a.worktreeRecoveries[id], a.worktreeBlocked(cwd)
	endOpen := a.trackSessionOpen(cwd)
	go func() {
		ctx := context.Background()
		openErr := requireWorktreeRecovery(record, cwd)
		if blocked {
			openErr = errors.New("工作树正在维护，请稍后再试")
		}
		var resp protocol.OpenSessionResponse
		if openErr == nil {
			resp, openErr = a.service.OpenSession(ctx, protocol.OpenSessionRequest{SessionID: id})
		}
		req := protocol.ListHistoryRequest{SessionID: id, Limit: historyPage}
		if openErr == nil {
			req.Before = resp.LastSeq + 1
		}
		history, historyErr := a.service.ListHistory(ctx, req)
		a.win.Update(func() {
			endOpen()
			if ch.starts != gen || ch.id != id || a.chats[id] != ch {
				if openErr == nil && a.chats[id] == nil {
					a.closeSession(id)
				}
				return
			}
			a.finishOpen(ch, resp, history, openErr, historyErr)
		})
	}()
}

func (a *App) finishOpen(
	ch *chat, resp protocol.OpenSessionResponse, history protocol.ListHistoryResponse,
	openErr, historyErr error,
) {
	pending := ch.pendingUpdates
	ch.loading, ch.pendingUpdates = false, nil
	ch.err = ""
	if historyErr == nil {
		ch.seekSeq = 0
		ch.load(history.Updates, history.Oldest)
		ch.historyEnd = resp.LastSeq
	}
	if openErr == nil {
		if a.reopenChat == ch {
			a.reopenOpen = false
		}
		ch.attach(resp.ConfigOptions, resp.State)
		if a.current == ch {
			a.cwd = ch.cwd
		}
		a.refreshBranch(ch, true)
	} else if errors.Is(openErr, fs.ErrNotExist) {
		if a.askCwdGone(ch) && a.reopenChat == ch {
			a.reopenOpen = false
		}
	} else if ch.needsReopen {
		a.askReopen(ch)
	}
	if err := cmp.Or(openErr, historyErr); err != nil {
		ch.err = err.Error()
	}
	for _, n := range pending {
		a.applyNotification(ch, n)
	}
}

// start 为新会话请求核心。工作目录变更后，旧请求返回的会话需要关闭。
func (a *App) start(ch *chat) {
	if a.win == nil || a.worktreeBlocked(ch.cwd) {
		return
	}
	ch.starts++
	ch.choosing = false
	if err := requireWorktreeRecovery(a.worktreeRecoveries[ch.id], ch.cwd); err != nil {
		ch.err = err.Error()
		a.askCwdGone(ch)
		return
	}
	// 工作目录可能已变更，旧分支状态与 Git 目录监视需要重新建立。
	ch.branch = branchState{}
	ch.stopWatchHead()
	a.refreshBranch(ch, true)
	gen := ch.starts
	req := protocol.NewSessionRequest{Cwd: ch.cwd}
	launchTask := ch.job != nil
	if launchTask {
		req.Type, req.OwnerID = protocol.SessionScheduled, ch.job.task.ID
	}
	endOpen := a.trackSessionOpen(req.Cwd)
	go func() {
		resp, err := a.service.NewSession(context.Background(), req)
		id := resp.SessionID
		a.win.Update(func() {
			endOpen()
			switch {
			case gen != ch.starts:
				if err == nil && id != ch.id {
					a.closeSession(id)
				}
				return
			case err != nil:
				ch.err = err.Error()
				if launchTask {
					a.failTask(ch, err)
				} else if errors.Is(err, fs.ErrNotExist) {
					a.askCwdGone(ch)
				}
				return
			}
			ch.id, ch.err = id, ""
			a.chats[id] = ch
			ch.attach(resp.ConfigOptions, resp.State)
			if launchTask {
				a.sendTask(ch) // 后台触发的会话不改默认的工作目录
				return
			}
			if a.current == ch {
				a.cwd = resp.State.Cwd
			}
		})
	}()
}

// trackSessionOpen 按请求发起时的目录跟踪打开操作，切换目录或丢弃会话后也要等请求结束。
func (a *App) trackSessionOpen(cwd string) func() {
	if a.openingSessions == nil {
		a.openingSessions = make(map[string]int)
	}
	a.openingSessions[cwd]++
	return func() {
		a.openingSessions[cwd]--
		if a.openingSessions[cwd] == 0 {
			delete(a.openingSessions, cwd)
		}
	}
}

// askCwdGone 确认工作目录是否仍需恢复。只在用户正在查看该会话时显示恢复对话框。
func (a *App) askCwdGone(ch *chat) bool {
	ch.cwdGone = a.worktreeRecoveries[ch.id].Root != "" || cwdMissing(ch.cwd)
	if ch.cwdGone && a.current == ch && !a.showTasks {
		a.cwdGoneChat, a.cwdGoneErr, a.cwdGoneOpen = ch, "", true
	}
	return ch.cwdGone
}

// cwdMissing 报告会话的工作目录是否已经不可用。空路径不算：拿不到路径时无从新建。
func cwdMissing(cwd string) bool {
	if cwd == "" {
		return false
	}
	info, err := os.Stat(cwd)
	return err != nil || !info.IsDir()
}

// createCwd 在会话原来的路径上建目录，建好后重新打开。只建一级：父目录不在时多半是磁盘
// 没挂载，多建几级会造出一个空目录。失败时把原因留给对话框，用户改完可以再试。
func (a *App) createCwd(ch *chat) {
	if a.worktreeBlocked(ch.cwd) || a.worktreeRecoveries[ch.id].Root != "" {
		return
	}
	done := func() {
		ch.cwdGone, a.cwdGoneOpen, a.cwdGoneErr = false, false, ""
		a.openSaved(ch)
	}
	err := os.Mkdir(ch.cwd, 0o755)
	switch {
	case err == nil:
		done()
	case errors.Is(err, fs.ErrExist):
		if info, statErr := os.Stat(ch.cwd); statErr != nil || !info.IsDir() {
			a.cwdGoneErr = "同名文件已存在，请先处理它"
			return
		}
		done()
	case errors.Is(err, fs.ErrNotExist):
		a.cwdGoneErr = "父目录不存在，请检查磁盘是否已挂载"
	default:
		a.cwdGoneErr = "新建目录失败：" + err.Error()
	}
}

// loadOlder 在列表滚近顶部时再取前面一页记录；没有更早的记录或正在取时什么也不做。
// 大纲要跳到的提问还没加载时（ch.seekSeq），不论滚到哪里都取，最后一页恰好取到它为止。
// 列表的行有 Key，往前加入时当前看的位置不动。
func (a *App) loadOlder(ch *chat) {
	first, last := ch.list.Visible()
	if (last < first || first >= olderNear) && ch.seekSeq == 0 {
		return
	}
	if ch.loading || ch.paging || ch.oldest <= 1 {
		return
	}
	limit := historyPage
	if ch.seekSeq > 0 {
		limit = min(limit, int(ch.oldest-ch.seekSeq))
	}
	ch.paging = true
	id, before, gen := ch.id, ch.oldest, ch.starts
	go func() {
		history, err := a.service.ListHistory(context.Background(),
			protocol.ListHistoryRequest{SessionID: id, Before: before, Limit: limit})
		a.win.Update(func() {
			ch.paging = false
			if ch.starts != gen || a.chats[id] != ch {
				return
			}
			if err != nil {
				ch.err, ch.oldest, ch.seekSeq = err.Error(), 0, 0 // 不再重试
				return
			}
			ch.prepend(history.Updates, history.Oldest)
		})
	}()
}

// refresh 重新读取侧栏的会话列表。
func (a *App) refresh() {
	a.listRefresh++
	gen := a.listRefresh
	go func() {
		list, err := a.service.ListSessions(context.Background(), protocol.ListSessionsRequest{})
		a.win.Update(func() {
			if gen != a.listRefresh {
				return
			}
			if err != nil {
				a.err = err.Error()
				return
			}
			a.err, a.infos, a.listed = "", list.Sessions, true
		})
	}()
}

// loadAllSessions 为启动器读取全部会话。请求失败时沿用侧栏列表，重开启动器后忽略旧结果。
func (a *App) loadAllSessions(gen int) {
	go func() {
		req := protocol.ListSessionsRequest{All: true}
		list, err := a.service.ListSessions(context.Background(), req)
		a.win.Update(func() {
			if !a.launch.open || a.launch.gen != gen {
				return
			}
			if err != nil {
				a.launch.sessionErr = "未能读取全部会话，搜索结果可能不全"
				return
			}
			a.launch.infos = list.Sessions
		})
	}()
}

// dropChat 在会话被删除后关闭本地面板。若删的是当前会话，清空它再新建，避免已确认
// 丢弃的未保存修改再次拦截切换。定时任务视图仍保持打开。
func (a *App) dropChat(id string) {
	ch := a.chats[id]
	if ch == nil {
		return
	}
	ch.closePanels()
	delete(a.chats, id)
	if a.current == ch {
		showTasks, cwd := a.showTasks, a.cwd
		if a.worktreeBlocked(cwd) {
			cwd = ch.cwd
			if cwdMissing(cwd) || cwd == "" || a.worktreeBlocked(cwd) {
				cwd = filepath.Dir(a.mutatingWorktree)
			}
		}
		a.current = nil
		a.newChatAt(cwd)
		a.showTasks = showTasks
	}
}

// groupDeletion 记录一次批量删除的确认范围与执行状态。cwd 非空表示操作尚未结束。
type groupDeletion struct {
	cwd      string
	ids      []string
	open     bool
	deleting bool
}

// askDeleteGroup 先读齐会话再打开确认框，避免侧栏的显示上限缩小删除范围。
func (a *App) askDeleteGroup(cwd string) {
	if cwd == "" || a.groupDelete.cwd != "" {
		return
	}
	a.groupDelete.cwd = cwd
	go func() {
		req := protocol.ListSessionsRequest{Cwd: cwd, Type: protocol.SessionInteractive, All: true}
		list, err := a.service.ListSessions(context.Background(), req)
		ids := make([]string, 0, len(list.Sessions))
		for _, info := range list.Sessions {
			ids = append(ids, info.SessionID)
		}
		a.win.Update(func() {
			switch {
			case err != nil:
				a.groupDelete = groupDeletion{}
				a.toast = "读取目录会话失败：" + err.Error()
			case len(ids) == 0:
				a.groupDelete = groupDeletion{}
				a.toast = "此目录没有可删除的会话"
				a.refresh()
			default:
				a.groupDelete.ids, a.groupDelete.open = ids, true
			}
		})
	}()
}

// deleteGroup 逐个删除已经确认的会话，让忙碌会话或单条失败不影响其余会话。
func (a *App) deleteGroup() {
	a.groupDelete.deleting = true
	ids := a.groupDelete.ids
	go func() {
		ctx := context.Background()
		var deleted, reasons []string
		for _, id := range ids {
			err := a.service.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: id})
			if err == nil {
				deleted = append(deleted, id)
			} else if reason := err.Error(); !slices.Contains(reasons, reason) {
				reasons = append(reasons, reason)
			}
		}
		a.win.Update(func() {
			for _, id := range deleted {
				a.dropChat(id)
			}
			a.groupDelete = groupDeletion{}
			a.refresh()
			if len(reasons) > 0 {
				a.toast = fmt.Sprintf("已删除 %d 个会话，保留 %d 个：%s",
					len(deleted), len(ids)-len(deleted), strings.Join(reasons, "、"))
			}
		})
	}()
}

// choose 让核心保存选择，由配置项更新重绘选择器。等待响应期间串行处理两种选择。
func (a *App) choose(ch *chat, id, value string) {
	if !ch.ready || ch.choosing {
		return
	}
	ch.choosing = true
	starts := ch.starts
	req := protocol.SetConfigOptionRequest{SessionID: ch.id, ConfigID: id, Value: value}
	go func() {
		_, err := a.service.SetConfigOption(context.Background(), req)
		a.win.Update(func() {
			if ch.starts != starts {
				return
			}
			ch.choosing = false
			if err != nil && ch.ready {
				ch.err = err.Error()
			}
		})
	}()
}

// setCwd 给还没发过消息的新会话换工作目录：工作目录在建会话时就定了，所以丢掉原来的，
// 按新目录再建一个。
func (a *App) setCwd(ch *chat, cwd string) {
	if !ch.fresh() || a.worktreeBlocked(cwd) || !a.canChangeDirectory(ch) || ch.cwd == cwd {
		return
	}
	ch.closePanels()
	ch.files, ch.changes, ch.order, ch.expanded = nil, nil, nil, noPanel
	if a.fileCreate != nil && a.fileCreate.ch == ch {
		a.fileCreate = nil
	}
	ch.cwd, a.cwd = cwd, cwd
	a.restart(ch)
}

// restart 丢掉还没发过消息的新会话在核心里的那一个，按它现在的工作目录再建一个。
func (a *App) restart(ch *chat) {
	if ch.id != "" {
		a.discard(ch)
	}
	ch.id, ch.ready, ch.err, ch.options, ch.commands = "", false, "", nil, nil
	ch.usage = protocol.Usage{}
	a.start(ch)
}

// refreshOptions 在设置变更后让已打开的会话重新读取模型清单。
func (a *App) refreshOptions() {
	a.optionsRefresh++
	gen := a.optionsRefresh
	a.settings.models.refreshErr = ""
	for _, ch := range slices.Collect(maps.Values(a.chats)) {
		if ch.job != nil || !ch.ready || a.worktreeBlocked(ch.cwd) {
			continue
		}
		if ch.fresh() {
			a.refreshDraftOptions(ch, gen)
			continue
		}
		id, epoch, starts := ch.id, ch.epoch, ch.starts
		endOpen := a.trackSessionOpen(ch.cwd)
		go func() {
			resp, err := a.service.OpenSession(context.Background(),
				protocol.OpenSessionRequest{SessionID: id})
			a.win.Update(func() {
				endOpen()
				if gen != a.optionsRefresh || a.chats[id] != ch || ch.starts != starts {
					return
				}
				if err == nil && resp.State.Epoch != epoch {
					a.askReopen(ch)
					return
				}
				if err != nil {
					a.modelRefreshFailed(err)
					return
				}
				ch.options = resp.ConfigOptions
			})
		}()
	}
}

// refreshDraftOptions 用最新设置重建还未发送的会话，建成之前保留原会话。
func (a *App) refreshDraftOptions(ch *chat, gen int) {
	id, starts, cwd := ch.id, ch.starts, ch.cwd
	endOpen := a.trackSessionOpen(cwd)
	go func() {
		resp, err := a.service.NewSession(context.Background(), protocol.NewSessionRequest{Cwd: cwd})
		a.win.Update(func() {
			endOpen()
			if gen != a.optionsRefresh || a.chats[id] != ch || ch.starts != starts || !ch.fresh() {
				if err == nil {
					a.closeSession(resp.SessionID)
				}
				return
			}
			if err != nil {
				a.modelRefreshFailed(err)
				return
			}
			delete(a.chats, id)
			a.closeSession(id)
			ch.id = resp.SessionID
			a.chats[ch.id] = ch
			ch.attach(resp.ConfigOptions, resp.State)
		})
	}()
}

func (a *App) modelRefreshFailed(err error) {
	if a.settings.models.refreshErr == "" {
		a.settings.models.refreshErr = "部分会话的模型列表未刷新：" + err.Error()
	}
}

// discard 让核心不再保留这个会话，并从已打开的会话里去掉它。
func (a *App) discard(ch *chat) {
	ch.starts++ // 使尚未返回的会话与分支结果失效
	ch.stopWatchHead()
	ch.branch = branchState{}
	if ch.id == "" {
		return
	}
	delete(a.chats, ch.id)
	a.closeSession(ch.id)
}

// closeSession 在后台通知核心，这个界面不再订阅会话 id。
func (a *App) closeSession(id string) {
	go func() {
		_ = a.service.CloseSession(context.Background(),
			protocol.CloseSessionRequest{SessionID: id})
	}()
}

// send 记下按键时看到的轮次。运行中仍可继续输入，由 sendNext 按提交顺序交给核心。
func (a *App) send(ch *chat, text string, nextTurn bool) bool {
	if a.worktreeBlocked(ch.cwd) {
		a.toast = "工作树正在维护，请稍后再试"
		return false
	}
	if ch.cwdGone && a.askCwdGone(ch) {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" && len(ch.attachments) == 0 || !ch.canSend() {
		return false
	}
	if ch.needsReopen {
		a.askReopen(ch)
		return false
	}
	if len(ch.attachments) > 0 && !ch.acceptsImages() {
		a.toast = noImagesHint
		return false
	}
	req := protocol.SendPromptRequest{
		SessionID: ch.id,
		Prompt:    draftPrompt(trimmed, ch.attachments),
	}
	if ch.run.Running {
		if trimmed == "/compact" && len(ch.attachments) == 0 {
			a.toast = "请在会话空闲时发送 /compact"
			return false
		}
		req.RunID = ch.run.RunID
		req.Delivery = protocol.PromptInTurn
		if nextTurn {
			req.Delivery = protocol.PromptNextTurn
		} else if ch.run.Kind == protocol.RunCompaction {
			a.toast = "手动压缩期间请按 ⌘Enter 排到下一轮"
			return false
		}
	}
	ch.submissions = append(ch.submissions, promptSubmission{request: req,
		draft: promptDraft{input: text, attachments: ch.attachments}})
	ch.attachments, ch.err = nil, ""
	if len(ch.submissions) == 1 {
		a.sendNext(ch)
	}
	return true
}

func (a *App) sendNext(ch *chat) {
	submission := ch.submissions[0]
	id, ready, gen, cwd := ch.id, ch.ready, ch.starts, ch.cwd
	record := a.worktreeRecoveries[id]
	go func() {
		ctx := context.Background()
		err := requireWorktreeRecovery(record, cwd)
		if err == nil && !ready {
			var resp protocol.OpenSessionResponse
			resp, err = a.service.OpenSession(ctx, protocol.OpenSessionRequest{SessionID: id})
			if err == nil {
				// 打开时的快照须先于此次发送产生的更新进入主线程队列。
				a.win.Update(func() {
					if ch.starts == gen && a.chats[id] == ch {
						ch.attach(resp.ConfigOptions, resp.State)
					}
				})
			}
		}
		if err == nil {
			_, err = a.service.SendPrompt(ctx, submission.request)
		}
		a.win.Update(func() { a.finishSubmission(ch, submission, gen, err) })
	}()
}

func (a *App) finishSubmission(ch *chat, finished promptSubmission, gen int, err error) {
	id := finished.request.SessionID
	if ch.id != id || a.chats[id] != ch || len(ch.submissions) == 0 {
		return
	}
	ch.submissions = slices.Delete(ch.submissions, 0, 1)
	if err != nil {
		if ch.queueEditing == nil && !ch.hasDraft() && ch.preparing == 0 &&
			len(ch.submissions) == 0 && len(ch.failedDrafts) == 0 {
			ch.promptDraft = finished.draft
		} else {
			ch.keepFailedDraft(finished.draft, err.Error())
		}
		if ch.starts == gen {
			a.finish(ch, err)
		}
	}
	if len(ch.submissions) != 0 {
		a.sendNext(ch)
	}
}

// finish 显示核心没有受理这条消息的原因；受理之后的错误经 run_state 更新送达。
func (a *App) finish(ch *chat, err error) {
	switch {
	case errors.Is(err, protocol.ErrNothingToCompact):
		a.toast = err.Error()
	case err != nil:
		ch.err = err.Error()
		if errors.Is(err, protocol.ErrConflict) || errors.Is(err, protocol.ErrNotFound) {
			a.askReopen(ch)
		}
		// 发送时才发现工作目录没了：与打开时一样给出恢复的对话框。
		if errors.Is(err, fs.ErrNotExist) {
			a.askCwdGone(ch)
		}
	}
}

// pickDir 让用户为尚未保存的新会话选择工作目录。
func (a *App) pickDir(ch *chat) {
	a.pickDirectory(ch, func(cwd string) {
		d := &worktreePicker{ch: ch, starts: ch.starts, cwd: ch.cwd}
		a.worktrees = d
		a.useWorktree(d, cwd)
	})
}

func (a *App) pickDirectory(ch *chat, apply func(string)) {
	starts := ch.starts
	options := mygo.OpenDialogOptions{
		Parent: a.win, Title: "选择工作目录", DefaultPath: ch.cwd, Directory: true,
	}
	go func() {
		paths, err := mygo.Dialog.Open(options)
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() {
			if a.current == ch && !a.showTasks && ch.starts == starts && ch.fresh() {
				apply(paths[0])
			}
		})
	}()
}
