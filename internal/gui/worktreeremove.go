package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"

	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/protocol"
)

type worktreeRemoval struct {
	entry   worktreeEntry
	refs    []protocol.SessionInfo
	loading bool
}

func worktreeRemoveReason(list worktreeList, entry worktreeEntry) string {
	switch {
	case entry.main:
		return "主工作树不能移除"
	case entry.cwd == list.root:
		return "请从另一棵工作树移除当前工作树"
	case entry.locked:
		return "这棵工作树已锁定"
	case entry.missing:
		return "工作树目录不可用"
	case entry.branch == "":
		return "分离 HEAD 的工作树不能在这里移除"
	case list.main == "":
		return "主工作树不可用，无法保留恢复入口"
	default:
		return ""
	}
}

func (a *App) worktreeBusy() bool {
	return a.creatingWorktree || a.mutatingWorktree != ""
}

func (a *App) worktreeBlocked(cwd string) bool {
	root := a.mutatingWorktree
	return root != "" && (insideWorktree(root, cwd) || slices.Contains(a.mutatingWorktreeCwds, cwd))
}

func (ch *chat) savingTab() bool {
	if ch.files != nil {
		for _, tb := range ch.files.tabs {
			if tb.saving {
				return true
			}
		}
	}
	return false
}

func (a *App) worktreeUseReason(root string) string {
	for cwd := range a.openingSessions {
		if insideWorktree(root, cwd) {
			return "请等待会话打开完成"
		}
	}
	for _, task := range a.tasks.list {
		if task.Enabled && insideWorktree(root, task.Cwd) {
			return "请先停用定时任务「" + task.Name + "」"
		}
	}
	for _, ch := range a.tasks.launched {
		if insideWorktree(root, ch.cwd) && ch.inFlight() {
			return "这棵工作树里的定时任务正在启动或运行"
		}
	}
	for _, cwd := range a.switchingBranches {
		if insideWorktree(root, cwd) {
			return "请等待分支切换完成"
		}
	}
	for path := range a.fileWrites {
		if insideWorktree(root, path) {
			return "请等待文件写入完成"
		}
	}
	if reason := chatWorktreeUseReason(a.current, root); reason != "" {
		return reason
	}
	for _, ch := range a.chats {
		if reason := chatWorktreeUseReason(ch, root); reason != "" {
			return reason
		}
	}
	return ""
}

func chatWorktreeUseReason(ch *chat, root string) string {
	if ch == nil {
		return ""
	}
	if insideWorktree(root, ch.cwd) {
		switch {
		case ch.loading || ch.paging:
			return "请等待会话或历史记录加载完成"
		case ch.run.Running || len(ch.submissions) > 0 || len(ch.queue.Items) > 0 || ch.resuming:
			return "这棵工作树有运行中的会话或待发消息"
		case len(ch.terms) > 0:
			return "请先关闭这棵工作树中的终端"
		}
	}
	if ch.files != nil {
		for _, tb := range ch.files.tabs {
			if (tb.dirty() || tb.saving) && insideWorktree(root, tb.path) {
				return "请先保存或关闭这棵工作树中的未保存文件，并等待写入完成"
			}
		}
	}
	return ""
}

func checkWorktreeRemoval(ctx context.Context, cwd, common string, target worktreeEntry) error {
	list, err := readWorktrees(ctx, cwd)
	if err != nil {
		return err
	}
	if list.common != common {
		return errors.New("当前仓库已变更，请重新打开工作树菜单")
	}
	i := slices.IndexFunc(list.entries, func(entry worktreeEntry) bool {
		return entry.cwd == target.cwd
	})
	if i == -1 || list.entries[i] != target {
		return errors.New("工作树状态已变更，请重新打开工作树菜单")
	}
	if reason := worktreeRemoveReason(list, target); reason != "" {
		return errors.New(reason)
	}
	if err := checkWorktreeCwd(ctx, target.cwd, target.cwd, common); err != nil {
		return errors.New("工作树的检出目录已被替换，请重新打开工作树菜单")
	}
	out, err := gitOut(ctx, target.cwd, "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--ignored=matching", "--ignore-submodules=none")
	if err != nil {
		return errors.New(gitError(err))
	}
	if len(out) > 0 {
		return errors.New("工作树含有未提交、未跟踪或被 Git 忽略的文件，请先处理这些文件")
	}
	// Git 状态会省略标记为 assume-unchanged 或 skip-worktree 的本地改动。
	out, err = gitOut(ctx, target.cwd, "ls-files", "-v", "-z")
	if err != nil {
		return errors.New(gitError(err))
	}
	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry != "" && (entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z') {
			return errors.New("工作树含有 Git 跳过检查的文件，无法确认可以安全移除")
		}
	}
	return nil
}

func (a *App) worktreeSessionRefs(
	ctx context.Context, root string, recovering map[string]worktreeRecovery,
) ([]protocol.SessionInfo, error) {
	var refs []protocol.SessionInfo
	kinds := []protocol.SessionType{protocol.SessionInteractive, protocol.SessionScheduled}
	for _, kind := range kinds {
		req := protocol.ListSessionsRequest{Type: kind, All: true}
		list, err := a.service.ListSessions(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, info := range list.Sessions {
			if recovering[info.SessionID].Root == "" && insideWorktree(root, info.Cwd) {
				refs = append(refs, info)
			}
		}
	}
	slices.SortFunc(refs, func(x, y protocol.SessionInfo) int {
		return strings.Compare(x.SessionID, y.SessionID)
	})
	return refs, nil
}

func (a *App) inspectWorktreeRemoval(d *worktreePicker, entry worktreeEntry) {
	if !a.worktreePickerCurrent(d) || !d.open || a.worktreeBusy() ||
		worktreeRemoveReason(*d.list, entry) != "" {
		return
	}
	d.open = false
	if reason := a.worktreeUseReason(entry.cwd); reason != "" {
		a.toast = reason
		return
	}
	p := &worktreeRemoval{entry: entry, loading: true}
	d.removal = p
	cwd, common := d.cwd, d.list.common
	recovering := maps.Clone(a.worktreeRecoveries)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeReadTimeout)
		defer cancel()
		err := checkWorktreeRemoval(ctx, cwd, common, entry)
		var refs []protocol.SessionInfo
		if err == nil {
			refs, err = a.worktreeSessionRefs(ctx, entry.cwd, recovering)
		}
		a.win.Update(func() {
			if !a.worktreePickerCurrent(d) || d.removal != p {
				return
			}
			p.loading, p.refs = false, refs
			if err != nil {
				d.removal = nil
				a.toast = "无法移除工作树：" + err.Error()
			}
		})
	}()
}

func (a *App) removeWorktree(d *worktreePicker, p *worktreeRemoval) {
	if !a.worktreePickerCurrent(d) || d.removal != p || p.loading || a.worktreeBusy() {
		return
	}
	entry := p.entry
	if reason := a.worktreeUseReason(entry.cwd); reason != "" {
		a.toast = reason
		return
	}
	cwd, refs := d.cwd, p.refs
	record := worktreeRecovery{Root: entry.cwd, Branch: entry.branch, Head: entry.head,
		Main: d.list.main, Common: d.list.common}
	crossCwds := make([]string, 0, len(refs))
	for _, ref := range refs {
		crossCwds = append(crossCwds, ref.Cwd)
	}
	for _, ch := range a.chats {
		if insideWorktree(entry.cwd, ch.cwd) {
			crossCwds = append(crossCwds, ch.cwd)
		}
	}
	a.mutatingWorktree, a.mutatingWorktreeCwds = entry.cwd, crossCwds
	recovering := maps.Clone(a.worktreeRecoveries)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeWriteTimeout)
		defer cancel()
		latest, err := a.worktreeSessionRefs(ctx, entry.cwd, recovering)
		if err == nil && !slices.EqualFunc(latest, refs, sameWorktreeSessionRef) {
			err = errors.New("关联会话已变更，请重新确认")
		}
		var saved worktreeRecovery
		if err == nil {
			var dir string
			dir, err = config.Dir()
			if err == nil {
				saved, err = persistAndTrashWorktree(ctx, cwd, dir, record, entry, refs,
					mygo.Shell.TrashItem)
			}
		}
		a.win.Update(func() { a.finishWorktreeRemoval(record, saved, refs, err) })
	}()
}

func sameWorktreeSessionRef(x, y protocol.SessionInfo) bool {
	return x.SessionID == y.SessionID && x.Cwd == y.Cwd
}

func persistAndTrashWorktree(
	ctx context.Context, cwd, dir string, record worktreeRecovery, entry worktreeEntry,
	refs []protocol.SessionInfo, trash func(string) error,
) (worktreeRecovery, error) {
	if err := checkWorktreeRemoval(ctx, cwd, record.Common, entry); err != nil {
		return worktreeRecovery{}, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(entry.cwd), ".atlas-worktree-*")
	if err != nil {
		return worktreeRecovery{}, err
	}
	defer os.Remove(staging)
	record.Staged = filepath.Join(staging, filepath.Base(entry.cwd))
	if err := updateWorktreeRecoveries(dir, func(records map[string]worktreeRecovery) {
		for _, ref := range refs {
			records[ref.SessionID] = record
		}
	}); err != nil {
		return worktreeRecovery{}, fmt.Errorf("保存工作树恢复记录失败：%w", err)
	}
	err = checkWorktreeRemoval(ctx, cwd, record.Common, entry)
	if err == nil {
		err = moveWorktree(ctx, cwd, entry.cwd, record.Staged)
	}
	if err != nil {
		return finishFailedWorktreeRemoval(dir, record, err)
	}
	if err := trash(record.Staged); err != nil {
		// 操作超时后仍尝试归还目录。在归还失败时保留临时检出与恢复记录。
		ctx, cancel := context.WithTimeout(context.Background(), worktreeReadTimeout)
		defer cancel()
		if moveErr := moveWorktree(ctx, cwd, record.Staged, entry.cwd); moveErr != nil {
			return record, fmt.Errorf("移入废纸篓失败，临时路径为 %s：%w",
				record.Staged, errors.Join(err, moveErr))
		}
		return finishFailedWorktreeRemoval(dir, record, err)
	}
	return record, removeWorktreeRegistration(ctx, cwd, record.Staged)
}

func finishFailedWorktreeRemoval(
	dir string, record worktreeRecovery, err error,
) (worktreeRecovery, error) {
	if _, statErr := os.Lstat(record.Staged); !errors.Is(statErr, fs.ErrNotExist) {
		return record, fmt.Errorf("工作树移除中断，临时路径为 %s：%w", record.Staged, err)
	}
	if saveErr := forgetWorktreeRecovery(dir, record); saveErr != nil {
		return record, errors.Join(err, saveErr)
	}
	return worktreeRecovery{}, err
}

func moveWorktree(ctx context.Context, cwd, from, to string) error {
	if _, err := gitOut(ctx, cwd, "worktree", "move", "--", from, to); err != nil {
		return errors.New(gitError(err))
	}
	return nil
}

func removeWorktreeRegistration(ctx context.Context, cwd, staged string) error {
	// Git 只清除已腾空的临时登记，实际文件始终由系统废纸篓接管。
	if err := worktreePathFree(staged); err != nil {
		return err
	}
	if _, err := gitOut(ctx, cwd, "worktree", "remove", "--", staged); err != nil {
		return errors.New(gitError(err))
	}
	return nil
}

func (a *App) finishWorktreeRemoval(
	record, saved worktreeRecovery, refs []protocol.SessionInfo, err error,
) {
	cwds := a.mutatingWorktreeCwds
	a.mutatingWorktree, a.mutatingWorktreeCwds = "", nil
	if saved.Root != "" {
		if a.worktreeRecoveries == nil {
			a.worktreeRecoveries = make(map[string]worktreeRecovery)
		}
		for _, ref := range refs {
			a.worktreeRecoveries[ref.SessionID] = saved
		}
	}
	if err == nil || saved.Root != "" {
		if pathWithin(record.Root, a.cwd) || slices.Contains(cwds, a.cwd) {
			a.cwd = record.Main
		}
		a.markWorktreeMissing(record, cwds)
	}
	if err != nil {
		a.toast = "移除工作树失败：" + err.Error()
	} else {
		a.toast = "已将工作树移入废纸篓：" + shortPath(record.Root)
	}
}

func (a *App) markWorktreeMissing(record worktreeRecovery, cwds []string) {
	for _, ch := range a.chats {
		if !pathWithin(record.Root, ch.cwd) && !slices.Contains(cwds, ch.cwd) {
			continue
		}
		ch.closePanels()
		ch.loading = false
		ch.branch = branchState{}
		ch.ready, ch.cwdGone = false, true
		ch.err = "工作树已移除：" + record.Root
		a.askCwdGone(ch)
	}
}
