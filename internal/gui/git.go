package gui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
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
	orig           string // 它在 HEAD 里相对仓库根的路径；新增的没有
	kind           changeKind
	added, removed int // 增删的行数，还没跟踪或不是文本的文件为 0
}

// changeSet 是一次读到的全部更改。
type changeSet struct {
	files     []fileChange
	hasHead   bool   // 仓库有提交；没有时所有文件都算新增
	truncated bool   // 超过 changeLimit，只列了一部分
	err       string // 读取失败的原因
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

// repoCheck 是会话的工作目录是不是 git 工作树的结论，按工作目录缓存。
type repoCheck struct {
	cwd string    // 结论对应的工作目录
	at  time.Time // 查的时刻
	ok  bool
}

// inRepo 报告会话的工作目录是不是 git 工作树，标题栏据此决定有没有更改按钮。
func (ch *chat) inRepo() bool {
	if ch.repo.cwd != ch.cwd || time.Since(ch.repo.at) > gitCheckAge {
		ch.repo = repoCheck{ch.cwd, time.Now(), findGit(ch.cwd)}
	}
	return ch.repo.ok
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

// loadChanges 读出工作目录下相对 HEAD 的全部更改：已暂存的、未暂存的与还没跟踪的。
func loadChanges(cwd string) changeSet {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	prefix, err := gitOut(ctx, cwd, "rev-parse", "--show-prefix")
	if err != nil {
		return changeSet{err: gitError(err)}
	}
	status, err := gitOut(ctx, cwd, "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--", ".")
	if err != nil {
		return changeSet{err: gitError(err)}
	}
	set := changeSet{files: parseStatus(string(status), strings.TrimSpace(string(prefix)))}
	if len(set.files) > changeLimit {
		set.files, set.truncated = set.files[:changeLimit], true
	}
	if _, err := gitOut(ctx, cwd, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return set
	}
	set.hasHead = true
	// 重命名在这里拆成删除与新增，路径才与状态里的一一对应；统计只是参考，读不到就没有。
	numstat, err := gitOut(ctx, cwd, "diff", "HEAD", "--numstat", "-z", "--no-renames",
		"--relative", "--", ".")
	if err == nil {
		fillCounts(set.files, string(numstat))
	}
	return set
}

// parseStatus 解析 git status --porcelain=v1 -z 的输出：每项是“XY 路径”，重命名后面还跟着
// 原路径。路径相对仓库根，prefix 是工作目录相对仓库根的路径，要去掉。结果按路径排序。
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
	slices.SortFunc(files, func(a, b fileChange) int { return strings.Compare(a.path, b.path) })
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
// 不能显示（太大、不是 UTF-8 文本）时 note 是原因。
func readChange(cwd string, c fileChange, hasHead bool) (before, after, note string) {
	if c.kind != changeAdded && hasHead {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		out, err := gitOut(ctx, cwd, "show", "HEAD:"+c.orig)
		switch {
		case err != nil:
			return "", "", "无法读取 HEAD 里的文件：" + gitError(err)
		case len(out) > diffMaxBytes:
			return "", "", "文件太大，不显示更改。"
		case !utf8.Valid(out) || bytes.IndexByte(out, 0) >= 0:
			return "", "", "不是 UTF-8 文本，不显示更改。"
		}
		before = string(out)
	}
	if c.kind != changeDeleted {
		content := readTab(filepath.Join(cwd, c.path))
		switch {
		case content.note != "":
			return "", "", content.note
		case content.img != nil:
			return "", "", "图片，不显示更改。"
		}
		after = content.text
	}
	return before, after, ""
}
