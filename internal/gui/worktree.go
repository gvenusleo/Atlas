package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"
)

const worktreeReadTimeout = 5 * time.Second

// worktreeEntry 表示 Git 登记的工作树。分离 HEAD 时 branch 为空。
type worktreeEntry struct {
	cwd, branch, head           string
	main, bare, locked, missing bool
}

type worktreeList struct {
	common, root, main string
	entries            []worktreeEntry
}

func readWorktrees(ctx context.Context, cwd string) (worktreeList, error) {
	root, err := worktreeGitPath(ctx, cwd, "--show-toplevel")
	if err != nil {
		return worktreeList{}, err
	}
	common, err := worktreeGitPath(ctx, cwd, "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return worktreeList{}, err
	}
	out, err := gitOut(ctx, cwd, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return worktreeList{}, errors.New(gitError(err))
	}
	list := worktreeList{root: root, common: common, entries: parseWorktrees(string(out))}
	if len(list.entries) > 0 && !list.entries[0].bare && !list.entries[0].missing {
		list.main = list.entries[0].cwd
	}
	return list, nil
}

func worktreeGitPath(ctx context.Context, cwd string, args ...string) (string, error) {
	out, err := gitOut(ctx, cwd, append([]string{"rev-parse"}, args...)...)
	if err != nil {
		return "", errors.New(gitError(err))
	}
	return filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
}

func parseWorktrees(out string) []worktreeEntry {
	var entries []worktreeEntry
	var entry worktreeEntry
	for line := range strings.SplitSeq(out, "\x00") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			entry.cwd = filepath.Clean(value)
		case "HEAD":
			entry.head = value
		case "branch":
			entry.branch = strings.TrimPrefix(value, "refs/heads/")
		case "locked":
			entry.locked = true
		case "bare":
			entry.bare = true
		case "prunable":
			entry.missing = true
		case "":
			if entry.cwd != "" {
				entry.main = len(entries) == 0
				entries = append(entries, entry)
				entry = worktreeEntry{}
			}
		}
	}
	for i := range entries {
		entry := &entries[i]
		if resolved, err := filepath.EvalSymlinks(entry.cwd); err == nil {
			entry.cwd = resolved
		}
		info, err := os.Stat(entry.cwd)
		entry.missing = entry.missing || err != nil || !info.IsDir()
	}
	return entries
}

func createAtlasWorktree(
	ctx context.Context, cwd, dir, common, name, branch string,
) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("工作树目录必须是绝对路径")
	}
	list, err := readWorktrees(ctx, cwd)
	if err != nil {
		return "", err
	}
	if list.common != common {
		return "", errors.New("工作目录所属的仓库已变更")
	}
	for _, entry := range list.entries {
		if !entry.bare && insideWorktree(entry.cwd, dir) {
			return "", fmt.Errorf("请在已有工作树之外选择工作树目录：%s", entry.cwd)
		}
	}
	_, suffix, _ := strings.CutLast(uuid.NewV7().String(), "-")
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "atlas/" + suffix
	}
	name = strings.TrimSpace(name)
	if name == "" {
		repo := list.main
		if repo == "" {
			repo = list.root
		}
		name = filepath.Base(repo) + "-" + suffix
	}
	if !filepath.IsLocal(name) || name == "." || strings.ContainsAny(name, "/\\\x00") {
		return "", errors.New("工作树名称必须是单个目录名")
	}
	target := filepath.Join(resolveWorktreePath(dir), name)
	if err := worktreePathFree(target); err != nil {
		return "", err
	}
	args := []string{"--no-track", "-b", branch, "--", target, "HEAD"}
	if err := checkoutWorktree(ctx, cwd, args...); err != nil {
		return "", err
	}
	return target, nil
}

func worktreePathFree(cwd string) error {
	_, err := os.Lstat(cwd)
	if err == nil {
		return fmt.Errorf("目标路径已存在：%s", cwd)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func checkoutWorktree(ctx context.Context, cwd string, args ...string) error {
	// 创建工作树仅检出 Git 中的文件，避免额外执行仓库初始化代码。
	args = append([]string{"-c", "core.hooksPath=" + os.DevNull, "worktree", "add"}, args...)
	if _, err := gitOut(ctx, cwd, args...); err != nil {
		return fmt.Errorf("检出工作树失败：%s", gitError(err))
	}
	return nil
}

func insideWorktree(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	if pathWithin(root, path) {
		return true
	}
	resolved := resolveWorktreePath(path)
	if pathWithin(root, resolved) {
		return true
	}
	// 大小写不敏感的卷上，路径拼法不同也可能指向同一工作树。
	info, err := os.Stat(root)
	if err != nil {
		return false
	}
	for dir := resolved; ; dir = filepath.Dir(dir) {
		if strings.EqualFold(dir, root) {
			candidate, err := os.Stat(dir)
			return err == nil && os.SameFile(info, candidate)
		}
		if dir == filepath.Dir(dir) || len(dir) < len(root) {
			return false
		}
	}
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// resolveWorktreePath 解析已存在的父目录，未创建的文件也可能位于符号链接下。
func resolveWorktreePath(path string) string {
	for links := 0; links < 40; links++ {
		parent := path
		for {
			if resolved, err := filepath.EvalSymlinks(parent); err == nil {
				rel, _ := filepath.Rel(parent, path)
				return filepath.Join(resolved, rel)
			}
			if link, err := os.Readlink(parent); err == nil {
				if !filepath.IsAbs(link) {
					link = filepath.Join(filepath.Dir(parent), link)
				}
				rel, _ := filepath.Rel(parent, path)
				path = filepath.Join(link, rel)
				break
			}
			if parent == filepath.Dir(parent) {
				return path
			}
			parent = filepath.Dir(parent)
		}
	}
	return path
}
