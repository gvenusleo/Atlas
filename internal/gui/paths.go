package gui

import (
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// 路径列表的参数：walkLimit 是遍历时最多列出的文件数，免得在巨大的目录里
// 列太久、占太多内存；pathsMaxAge 是路径列表用多久就要重新列，运行中新建的文件也才找得到。
const (
	walkLimit   = 20000
	pathsMaxAge = 3 * time.Second
)

// pathIndex 是会话工作目录下的路径列表，@ 补全与文件面板的筛选共用。
type pathIndex struct {
	cwd     string    // paths 对应的工作目录
	paths   []string  // 工作目录下的文件与目录
	at      time.Time // 开始列 paths 的时刻
	version int       // paths 每变一次加一，按它排好的缓存据此作废
}

// rankKey 是按查询排好的路径的缓存键：同一个查询、同一版路径列表，排出来的结果就一样。
type rankKey struct {
	query   string
	version int
}

// stale 报告路径列表是否该（重新）列出：换了工作目录，或用了 pathsMaxAge。
func (ix *pathIndex) stale(cwd string) bool {
	return ix.cwd != cwd || time.Since(ix.at) > pathsMaxAge
}

// loadPaths 在后台列出 ch 的工作目录下的路径。重新列完之前先用旧的，换了工作目录则先清掉旧的。
func (a *App) loadPaths(ch *chat) {
	ix, cwd := &ch.index, ch.cwd
	if ix.cwd != cwd {
		ix.paths = nil
		ix.version++
	}
	ix.cwd, ix.at = cwd, time.Now()
	go func() {
		list := listPaths(cwd)
		a.win.Update(func() {
			if ix.cwd == cwd { // 列的时候换了工作目录的，结果作废
				ix.paths = list
				ix.version++
			}
		})
	}()
}

// relOrAbs 返回 p 相对工作目录 cwd 的路径；不在 cwd 下的返回绝对路径。相对的 p 按 cwd 解析。
func relOrAbs(cwd, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	if rel, err := filepath.Rel(cwd, p); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return p
}

// skipped 报告文件树不显示的名字：.git 目录与 macOS 自动生成的 .DS_Store。
// 文件树与路径列表共用，两处的口径一致。
func skipped(name string) bool { return name == ".git" || name == ".DS_Store" }

// extOf 是路径的小写扩展名，带点。
func extOf(p string) string { return strings.ToLower(filepath.Ext(p)) }

// listPaths 列出工作目录下的文件与目录，供补全消息里的 @ 引用与文件面板的筛选：路径相对
// 工作目录、用 / 分隔，目录以 / 结尾。自浅入深遍历，超过 walkLimit 时只列到浅处；
// 出错时当作没有。
func listPaths(cwd string) []string {
	return withDirs(walkPaths(cwd))
}

// walkPaths 自浅入深遍历 cwd，列出其中的文件，不含 skipped 的名字。
func walkPaths(cwd string) []string {
	var paths []string
	for dirs := []string{""}; len(dirs) > 0 && len(paths) < walkLimit; dirs = dirs[1:] {
		entries, _ := os.ReadDir(filepath.Join(cwd, dirs[0]))
		for _, e := range entries {
			rel := path.Join(dirs[0], e.Name())
			switch {
			case skipped(e.Name()):
			case !e.IsDir():
				paths = append(paths, rel)
			default:
				dirs = append(dirs, rel)
			}
		}
	}
	return paths
}

// withDirs 在文件之外补上它们所在的各级目录。
func withDirs(files []string) []string {
	dirs := make(map[string]bool)
	for _, f := range files {
		for dir := path.Dir(f); dir != "." && !dirs[dir+"/"]; dir = path.Dir(dir) {
			dirs[dir+"/"] = true
		}
	}
	return slices.AppendSeq(files, maps.Keys(dirs))
}
