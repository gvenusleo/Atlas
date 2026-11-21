package gui

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"github.com/gvenusleo/atlas/internal/config"
)

const worktreeRecoveryFile = "worktree-recovery.json"

// worktreeRecovery 记录会话在工作树被移除前使用的检出。
type worktreeRecovery struct {
	Root   string `json:"root"`
	Main   string `json:"main"`
	Common string `json:"common"`
	Branch string `json:"branch"`
	Head   string `json:"head"`
	Staged string `json:"staged,omitempty"`
}

func loadWorktreeRecoveries(dir string) (map[string]worktreeRecovery, error) {
	data, err := os.ReadFile(filepath.Join(dir, worktreeRecoveryFile))
	if errors.Is(err, fs.ErrNotExist) {
		return make(map[string]worktreeRecovery), nil
	}
	if err != nil {
		return nil, err
	}
	var records map[string]worktreeRecovery
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if records == nil {
		return nil, errors.New("工作树恢复记录为空")
	}
	return records, nil
}

func updateWorktreeRecoveries(dir string, update func(map[string]worktreeRecovery)) error {
	records, err := loadWorktreeRecoveries(dir)
	if err != nil {
		return err
	}
	update(records)
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".worktree-recovery-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, worktreeRecoveryFile))
}

func (a *App) loadWorktreeRecovery() {
	dir, err := config.Dir()
	if err == nil {
		a.worktreeRecoveries, err = loadWorktreeRecoveries(dir)
	}
	if err != nil {
		a.toast = "读取工作树恢复记录失败：" + err.Error()
	}
}

func requireWorktreeRecovery(record worktreeRecovery, cwd string) error {
	if record.Root == "" {
		return nil
	}
	return fmt.Errorf("请先恢复原工作树 %s：%w", cwd, fs.ErrNotExist)
}

func checkWorktreeCwd(ctx context.Context, cwd, root, common string) error {
	actualRoot, err := worktreeGitPath(ctx, cwd, "--show-toplevel")
	if err == nil && actualRoot == root {
		actualCommon, err := worktreeGitPath(ctx, cwd, "--path-format=absolute", "--git-common-dir")
		if err == nil && actualCommon == common {
			return nil
		}
	}
	return fmt.Errorf("原工作目录 %s 已不可用：%w", cwd, fs.ErrNotExist)
}

func restoreWorktree(ctx context.Context, record worktreeRecovery, cwd string) error {
	if checkWorktreeCwd(ctx, record.Root, record.Root, record.Common) == nil {
		return checkRestoredWorktree(ctx, record, cwd)
	}
	if err := worktreePathFree(record.Root); err != nil {
		return err
	}
	if resolveWorktreePath(record.Root) != record.Root {
		return errors.New("原工作树的父目录已变更")
	}
	list, err := readWorktrees(ctx, record.Main)
	if err != nil || list.common != record.Common {
		return fmt.Errorf("原仓库已不可用：%s", record.Main)
	}
	ref := "refs/heads/" + record.Branch
	out, err := gitOut(ctx, record.Main, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil || string(out) != record.Head+"\n" {
		return fmt.Errorf("原分支 %q 已不存在或已移动至其他提交", record.Branch)
	}
	for _, entry := range list.entries {
		if entry.branch != record.Branch {
			continue
		}
		if entry.cwd != record.Staged {
			return fmt.Errorf("原分支已在另一棵工作树登记：%s", entry.cwd)
		}
		if !entry.missing {
			if err := moveWorktree(ctx, record.Main, entry.cwd, record.Root); err != nil {
				return err
			}
			return checkRestoredWorktree(ctx, record, cwd)
		}
		if err := removeWorktreeRegistration(ctx, record.Main, entry.cwd); err != nil {
			return err
		}
	}
	if err := checkoutWorktree(ctx, record.Main, "--", record.Root, record.Branch); err != nil {
		return err
	}
	return checkRestoredWorktree(ctx, record, cwd)
}

func checkRestoredWorktree(ctx context.Context, record worktreeRecovery, cwd string) error {
	if err := checkWorktreeCwd(ctx, cwd, record.Root, record.Common); err != nil {
		return err
	}
	ref, err := gitOut(ctx, cwd, "symbolic-ref", "-q", "HEAD")
	if err != nil || string(ref) != "refs/heads/"+record.Branch+"\n" {
		return errors.New("原目录检出的分支与移除时不同")
	}
	head, err := gitOut(ctx, cwd, "rev-parse", "HEAD")
	if err != nil || string(head) != record.Head+"\n" {
		return errors.New("原目录检出的提交与移除时不同")
	}
	return nil
}

func forgetWorktreeRecovery(dir string, record worktreeRecovery) error {
	return updateWorktreeRecoveries(dir, func(records map[string]worktreeRecovery) {
		maps.DeleteFunc(records, func(_ string, saved worktreeRecovery) bool {
			return saved == record
		})
	})
}

func (a *App) restoreCwdWorktree(ch *chat, record worktreeRecovery) {
	if a.worktreeBusy() {
		return
	}
	cwd, starts := ch.cwd, ch.starts
	a.mutatingWorktree, a.cwdGoneErr = record.Root, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), worktreeWriteTimeout)
		defer cancel()
		err := restoreWorktree(ctx, record, cwd)
		if err == nil {
			var dir string
			dir, err = config.Dir()
			if err == nil {
				err = forgetWorktreeRecovery(dir, record)
			}
		}
		a.win.Update(func() {
			a.mutatingWorktree = ""
			if err == nil {
				maps.DeleteFunc(a.worktreeRecoveries,
					func(_ string, saved worktreeRecovery) bool { return saved == record })
			}
			if a.current != ch || ch.starts != starts || !a.cwdGoneOpen {
				if err == nil {
					a.toast = "已恢复工作树：" + shortPath(record.Root)
				}
				return
			}
			if err != nil {
				a.cwdGoneErr = "恢复工作树失败：" + err.Error()
				return
			}
			ch.cwdGone, a.cwdGoneOpen = false, false
			a.start(ch)
		})
	}()
}
