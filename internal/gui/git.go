package gui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gvenusleo/atlas/internal/shellenv"
)

// 更改面板读 git 的参数：changeLimit 是最多列出的文件数，免得仓库里一次改了成千上万个文件时
// 列太久；diffMaxBytes 是显示对比的单个文件大小上限；gitTimeout 限制一次读取的总耗时。
const (
	changeLimit  = 2000
	diffMaxBytes = 10 << 20
	gitTimeout   = 30 * time.Second
)

// changeKind 是一个文件相对 HEAD 的更改类型。
type changeKind int

const (
	changeModified changeKind = iota
	changeAdded               // 新增，包括还没跟踪的文件
	changeDeleted
	changeRenamed
	changeConflict // 合并冲突
)

// letter 是更改类型在文件树里的标记。
func (k changeKind) letter() string {
	return [...]string{"M", "A", "D", "R", "!"}[k]
}

// fileChange 是工作区里一个文件相对 HEAD 的更改。
type fileChange struct {
	path           string // 相对工作目录，用 / 分隔
	repo           string // 所在仓库相对工作目录的路径；工作目录自己所在的仓库是 "."
	orig           string // 它在 HEAD 里相对仓库根的路径；新增的没有
	kind           changeKind
	added, removed int  // 增删的行数，还没跟踪或不是文本的文件为 0
	hasHead        bool // 所在仓库有提交；没有时所有文件都算新增
}

// changeSet 是一次读到的全部更改。
type changeSet struct {
	files     []fileChange
	truncated bool   // 超过 changeLimit，只列了一部分
	err       string // 读取失败的原因，多个仓库失败时每个一行
}

// findGit 报告 dir 或它的上层目录里有没有 .git（工作树与子模块里它是文件）。
// 没有时不调用 git：macOS 上没装开发工具时，运行 git 会弹出安装提示。
func findGit(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// gitCheckAge 是“工作目录是不是 git 工作树”的结论用多久就要重查，之后运行 git init 的也认得。
const gitCheckAge = 3 * time.Second

// repoCheck 是会话的工作目录是不是 git 工作树（或其下有没有子仓库）的结论，按工作目录缓存。
type repoCheck struct {
	cwd      string    // 结论对应的工作目录
	at       time.Time // 查的时刻
	ok       bool
	scanning bool // 正在后台查，只用于子仓库
}

// inRepo 报告会话的工作目录是不是 git 工作树，分支栏据此决定显不显示。
func (ch *chat) inRepo() bool {
	if ch.repo.cwd != ch.cwd || time.Since(ch.repo.at) > gitCheckAge {
		ch.repo = repoCheck{cwd: ch.cwd, at: time.Now(), ok: findGit(ch.cwd)}
	}
	return ch.repo.ok
}

// hasRepos 报告更改面板有没有可看的仓库：工作目录在 git 工作树里，或其下有子仓库。
// 找子仓库要遍历目录，放到后台；结果回来之前沿用上次的结论。
func (a *App) hasRepos(ch *chat) bool {
	if ch.inRepo() {
		return true
	}
	r := &ch.subs
	ok := r.cwd == ch.cwd && r.ok
	if r.scanning || (r.cwd == ch.cwd && time.Since(r.at) <= gitCheckAge) {
		return ok
	}
	r.scanning = true
	cwd := ch.cwd
	go func() {
		found := len(findRepos(cwd)) > 0
		a.win.Update(func() { ch.subs = repoCheck{cwd: cwd, at: time.Now(), ok: found} })
	}()
	return ok
}

// 找子仓库的范围：最多往下 repoDepth 层，最多读 repoScanMax 个目录，免得工作目录是家目录
// 这类大目录时遍历太久。
const (
	repoDepth   = 3
	repoScanMax = 1000
)

// findRepos 返回更改面板要看的仓库，路径相对 cwd、用 / 分隔：cwd 在 git 工作树里时只有 "."，
// 否则是它的子仓库。找到仓库后不再进入其内部；跳过隐藏目录、node_modules 与符号链接，
// 后者免得绕回上层。
func findRepos(cwd string) []string {
	if findGit(cwd) {
		return []string{"."}
	}
	var repos []string
	level, scanned := []string{"."}, 0
	for range repoDepth {
		var next []string
		for _, dir := range level {
			if scanned >= repoScanMax {
				break
			}
			scanned++
			entries, _ := os.ReadDir(filepath.Join(cwd, dir))
			for _, e := range entries {
				name := e.Name()
				if !e.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" {
					continue
				}
				rel := path.Join(dir, name)
				if _, err := os.Lstat(filepath.Join(cwd, rel, ".git")); err == nil {
					repos = append(repos, rel)
				} else {
					next = append(next, rel)
				}
			}
		}
		level = next
	}
	slices.Sort(repos)
	return repos
}

// gitOut 在 dir 运行 git 并返回标准输出。GIT_OPTIONAL_LOCKS=0 让 status 不去抢索引的锁，
// 免得与用户或 Agent 正在运行的 git 命令互相等待。
func gitOut(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(shellenv.Environ(), "GIT_OPTIONAL_LOCKS=0")
	return cmd.Output()
}

// gitError 把 git 的失败说成一句话：优先用它自己的说明。
func gitError(err error) string {
	if errors.Is(err, exec.ErrNotFound) {
		return "没有找到 git"
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		if msg := strings.TrimSpace(string(exit.Stderr)); msg != "" {
			return msg
		}
	}
	return err.Error()
}

// loadChanges 读出工作目录下各仓库相对各自 HEAD 的全部更改：已暂存的、未暂存的与还没跟踪的。
// 一个仓库读取失败时记下原因，其余照常列出。
func loadChanges(cwd string) changeSet {
	repos := findRepos(cwd)
	if len(repos) == 0 {
		return changeSet{err: "工作目录下没有 git 仓库"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	var set changeSet
	var errs []string
	for _, repo := range repos {
		files, err := loadRepoChanges(ctx, filepath.Join(cwd, repo))
		if err != nil {
			if repo != "." {
				err = errors.New(repo + "：" + err.Error())
			}
			errs = append(errs, err.Error())
			continue
		}
		for i := range files {
			files[i].repo = repo
			files[i].path = path.Join(repo, files[i].path)
		}
		set.files = append(set.files, files...)
	}
	set.err = strings.Join(errs, "\n")
	slices.SortFunc(set.files, func(a, b fileChange) int { return strings.Compare(a.path, b.path) })
	if len(set.files) > changeLimit {
		set.files, set.truncated = set.files[:changeLimit], true
	}
	return set
}

// loadRepoChanges 读出 dir 下相对它所在仓库 HEAD 的更改，路径相对 dir。
func loadRepoChanges(ctx context.Context, dir string) ([]fileChange, error) {
	prefix, err := gitOut(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, errors.New(gitError(err))
	}
	status, err := gitOut(ctx, dir, "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--", ".")
	if err != nil {
		return nil, errors.New(gitError(err))
	}
	files := parseStatus(string(status), strings.TrimSpace(string(prefix)))
	if _, err := gitOut(ctx, dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return files, nil
	}
	for i := range files {
		files[i].hasHead = true
	}
	// 重命名在这里拆成删除与新增，路径才与状态里的一一对应；统计只是参考，读不到就没有。
	numstat, err := gitOut(ctx, dir, "diff", "HEAD", "--numstat", "-z", "--no-renames",
		"--relative", "--", ".")
	if err == nil {
		fillCounts(files, string(numstat))
	}
	return files, nil
}

// parseStatus 解析 git status --porcelain=v1 -z 的输出：每项是“XY 路径”，重命名后面还跟着
// 原路径。路径相对仓库根，prefix 是工作目录相对仓库根的路径，要去掉。
func parseStatus(out, prefix string) []fileChange {
	var files []fileChange
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		xy, name := f[:2], f[3:]
		c := fileChange{path: strings.TrimPrefix(name, prefix), orig: name}
		switch {
		case xy == "??":
			c.kind = changeAdded
		case strings.Contains(xy, "U") || xy == "AA" || xy == "DD":
			c.kind = changeConflict
		case xy[0] == 'R' || xy[1] == 'R':
			c.kind = changeRenamed
		case xy[0] == 'A' || xy[0] == 'C':
			c.kind = changeAdded
		case xy[0] == 'D' || xy[1] == 'D':
			c.kind = changeDeleted
		}
		if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
			i++ // 原路径
			if c.kind == changeRenamed && i < len(fields) {
				c.orig = fields[i]
			}
		}
		if c.kind == changeAdded {
			c.orig = ""
		}
		files = append(files, c)
	}
	return files
}

// fillCounts 把 git diff --numstat -z 的增删行数填进 files。每项是“增<tab>删<tab>路径”，
// 不是文本的文件两个数都是 -。
func fillCounts(files []fileChange, numstat string) {
	byPath := make(map[string][2]int)
	for rec := range strings.SplitSeq(numstat, "\x00") {
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, err1 := strconv.Atoi(parts[0])
		removed, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			byPath[parts[2]] = [2]int{added, removed}
		}
	}
	for i := range files {
		if files[i].kind == changeRenamed {
			continue // 拆成删除与新增后的数不是这次重命名的改动
		}
		n := byPath[files[i].path]
		files[i].added, files[i].removed = n[0], n[1]
	}
}

// readChange 读出一个更改前后的文本：前是 HEAD 里的，后是磁盘上的；新增的前为空，删除的后为空。
// 不能显示（图片、太大、不是 UTF-8 文本）时 note 是原因。
func readChange(cwd string, c fileChange) (before, after, note string) {
	if isRaster(c.path) {
		return "", "", "图片，不显示更改"
	}
	if c.kind != changeAdded && c.hasHead {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		out, err := gitOut(ctx, filepath.Join(cwd, c.repo), "show", "HEAD:"+c.orig)
		switch {
		case err != nil:
			return "", "", "无法读取 HEAD 里的文件：" + gitError(err)
		case len(out) > diffMaxBytes:
			return "", "", "文件太大，不显示更改"
		case !utf8.Valid(out) || bytes.IndexByte(out, 0) >= 0:
			return "", "", "不是 UTF-8 文本，不显示更改"
		}
		before = string(out)
	}
	if c.kind != changeDeleted {
		content := readTab(filepath.Join(cwd, c.path))
		if content.note != "" {
			return "", "", content.note
		}
		after = content.text
	}
	return before, after, ""
}

// readBranch 读取当前分支；未提交的仓库也有符号引用，因此先读取 symbolic-ref，
// 分离 HEAD 时再读取短哈希。
func readBranch(cwd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	if out, err := gitOut(ctx, cwd, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	out, err := gitOut(ctx, cwd, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", errors.New(gitError(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// branchMax 是本地与远程各最多列出的分支数。
const branchMax = 200

// branchList 是可切换的分支：本地与远程，各自最近提交的在前。
type branchList struct {
	local  []string // 如 main
	remote []string // 带远程前缀，如 origin/main
}

// readBranches 读出 cwd 所在仓库的本地与远程分支。
func readBranches(cwd string) branchList {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	return branchList{
		local:  refNames(ctx, cwd, "refs/heads"),
		remote: refNames(ctx, cwd, "refs/remotes"),
	}
}

// refNames 返回 ns 下按最近提交排序的引用短名。限制数量以控制大型仓库的读取成本，
// 并排除不代表分支的远程 HEAD 引用。
func refNames(ctx context.Context, cwd, ns string) []string {
	out, err := gitOut(ctx, cwd, "for-each-ref", "--sort=-committerdate",
		"--count="+strconv.Itoa(branchMax), "--format=%(refname)", ns)
	if err != nil {
		return nil
	}
	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		name, ok := strings.CutPrefix(line, ns+"/")
		if !ok || strings.HasSuffix(name, "/HEAD") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// switchBranch 切换到分支 name。remote 表示远程分支（如 origin/main）：本地存在同名分支时
// 直接切换，否则建立跟踪分支。
func switchBranch(cwd, name string, remote bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	args := []string{"switch", name}
	if remote {
		branch := name
		if _, after, ok := strings.Cut(name, "/"); ok { // 去掉远程名
			branch = after
		}
		args = []string{"switch", "-c", branch, "--track", name}
		if hasBranch(ctx, cwd, branch) {
			args = []string{"switch", branch}
		}
	}
	if _, err := gitOut(ctx, cwd, args...); err != nil {
		return errors.New(gitError(err))
	}
	return nil
}

// hasBranch 检查本地分支 name 是否存在。
func hasBranch(ctx context.Context, cwd, name string) bool {
	_, err := gitOut(ctx, cwd, "rev-parse", "--verify", "-q", "refs/heads/"+name)
	return err == nil
}

// gitDir 返回解析符号链接后的 Git 目录；独立 worktree 使用各自的 Git 目录。
func gitDir(cwd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := gitOut(ctx, cwd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", errors.New(gitError(err))
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}
