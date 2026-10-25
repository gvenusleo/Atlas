package gui

import (
	"cmp"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 启动器列出的条数：输入为空时列出最近的 launcherRecent 个会话，有输入时每组最多列
// launcherGroup 项。
const (
	launcherRecent = 8
	launcherGroup  = 5
)

// launcher 是启动器的状态：⌘K 打开的搜索面板，同时搜索会话、操作与当前工作目录下的文件。
type launcher struct {
	listNav
	open       bool
	query      string
	shownQuery string // 上一帧的 query，变了就让高亮回到第一行
	gen        int    // 打开的次数，翻页取会话的结果据此作废
	// infos 是全部会话，打开后在后台翻页取全；取全之前用侧栏的。
	infos     []protocol.SessionInfo
	fileRanks memo[rankKey, []string]
	results   []result // 上一帧列出的结果，按键作用于它
}

// result 是启动器里的一项结果。
type result struct {
	group      string // 所属分组的标题
	name, hint string
	trail      string  // 行尾：会话的相对时间，或操作的快捷键
	icon       *ui.SVG // 名称前的图标，没有时留空
	run        func()
}

// openLauncher 打开启动器：输入清空，并在后台取全会话。
func (a *App) openLauncher() {
	l := &a.launch
	*l = launcher{open: true, gen: l.gen + 1, pointer: noPointer}
	a.loadAllSessions(l.gen)
}

// chooseResult 关掉启动器，执行选中的一项。
func (a *App) chooseResult(r result) {
	a.launch.open = false
	r.run()
}

// launcherResults 列出当前输入的结果：会话、操作、文件，各为一组。
func (a *App) launcherResults() []result {
	terms := strings.Fields(strings.ToLower(a.launch.query))
	results := a.sessionResults(terms)
	results = append(results, a.actionResults(terms)...)
	return append(results, a.fileResults()...)
}

// matchAll 报告 s（已转小写）是否包含每一个词。
func matchAll(s string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(s, term) {
			return false
		}
	}
	return true
}

// sessionResults 列出标题包含全部词的会话：以第一个词开头的在前，其余保持更新时间的倒序。
// 没有输入时是最近的几个。
func (a *App) sessionResults(terms []string) []result {
	infos := a.launch.infos
	if infos == nil {
		infos = a.infos
	}
	limit := launcherGroup
	if len(terms) == 0 {
		limit = launcherRecent
	}
	var first, rest []result
	for _, info := range infos {
		title := cmp.Or(info.Title, "新会话")
		lower := strings.ToLower(title)
		if !matchAll(lower, terms) {
			continue
		}
		r := result{
			group: "会话", name: title, hint: filepath.Base(info.Cwd), trail: ago(info.UpdatedAt),
			run: func() { a.open(info.SessionID) },
		}
		if len(terms) > 0 && strings.HasPrefix(lower, terms[0]) {
			first = append(first, r)
		} else {
			rest = append(rest, r)
		}
	}
	results := slices.Concat(first, rest)
	return results[:min(len(results), limit)]
}

// actionResults 列出名称包含全部词的操作；没有输入时是全部。
func (a *App) actionResults(terms []string) []result {
	actions := []result{
		{name: "新对话", icon: plusIcon, trail: "⌘N", run: a.newChat},
		{name: "切换侧栏", icon: sidebarIcon, trail: "⌘B", run: a.toggleSidebar},
		{name: "切换文件面板", icon: folderIcon, run: a.toggleFiles},
		{name: "打开设置", icon: settingsIcon, trail: "⌘,", run: a.openSettings},
	}
	if a.current.inRepo() {
		actions = append(actions,
			result{name: "切换更改面板", icon: diffIcon, run: a.toggleChanges})
	}
	actions = append(actions,
		result{name: "切换终端", icon: terminalIcon, trail: "⌃`", run: a.toggleTerminal})
	var results []result
	for _, r := range actions {
		if matchAll(strings.ToLower(r.name), terms) {
			r.group = "操作"
			results = append(results, r)
		}
	}
	if len(terms) > 0 {
		results = results[:min(len(results), launcherGroup)]
	}
	return results
}

// fileResults 列出当前会话的工作目录下路径包含输入的文件，排序与 @ 补全一样；没有输入时
// 没有。路径列表第一次用到时才在后台取。
func (a *App) fileResults() []result {
	ch, query := a.current, strings.TrimSpace(a.launch.query)
	if query == "" || ch.cwd == "" {
		return nil
	}
	if ch.index.stale(ch.cwd) {
		a.loadPaths(ch)
	}
	files := a.launch.fileRanks.get(rankKey{query, ch.index.version}, func() []string {
		var files []string
		for _, p := range rankPaths(ch.index.paths, query, mentionLimit) {
			if !strings.HasSuffix(p, "/") {
				files = append(files, p)
			}
		}
		return files[:min(len(files), launcherGroup)]
	})
	var results []result
	for _, p := range files {
		dir, name := path.Split(p)
		results = append(results, result{
			group: "文件", name: name, hint: strings.TrimSuffix(dir, "/"),
			icon: fileIcon(p), run: func() { a.showFile(ch, p) },
		})
	}
	return results
}
