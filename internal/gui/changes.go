package gui

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// changePanel 是会话的更改面板：工作目录下相对 HEAD 的更改文件树，以及选中文件的对比。
type changePanel struct {
	cwd       string // set 对应的工作目录
	gen       int    // 换一次工作目录加一，读到的结果据此判断是否过时
	set       changeSet
	loaded    bool    // 至少读完过一次
	loading   bool    // 正在读
	again     bool    // 读的时候又有变化：读完后再读一次
	prevWidth float32 // 打开时右侧栏被拉宽过：拉宽之前的宽度，关闭时还原；0 为没拉宽

	// 更改树，见 tree.go。条目是相对工作目录的路径。
	roots  []string                // 顶层的条目
	kids   map[string][]string     // 目录里的条目，文件没有
	files  map[string]fileChange   // 文件的更改
	seen   map[string]bool         // 展开过的目录：新出现的默认展开，用户收起的不再自动展开
	tree   ui.OutlineState[string] // 展开着的目录与滚动位置
	chosen ui.Selection[string]    // 选中的条目

	selected  string // 对比里显示的文件；没有时为空
	view      *changeView
	treeWidth float32 // 文件树与对比之间分隔条的位置
	split     bool    // 对比并排显示，否则上下合并
}

// changeView 是选中文件的对比。
type changeView struct {
	path string
	seq  int               // 读过几次，后读的结果才有效
	note string            // 不能显示的原因
	diff *editor.DiffState // 不能显示或还没读到时为空
}

// toggleChanges 显示或收起当前会话的更改面板；第一次打开时建面板，右侧栏不够宽时拉宽，
// 关闭时在没被再拖过的情况下还原，打开时重新读取更改。
func (a *App) toggleChanges() {
	ch := a.current
	if ch.changes == nil {
		ch.changes = &changePanel{treeWidth: changeTreeDefault}
	}
	cp := ch.changes
	a.togglePanel(changesPanel, func() {
		a.widenSide(&cp.prevWidth)
		if cp.cwd == ch.cwd { // 之前开过：面板收起期间可能变过，没开过的由卡片第一次画时读
			a.refreshChanges(ch)
		}
	}, func() { a.restoreSide(&cp.prevWidth) })
}

// reset 在换了工作目录时清掉已读到的内容。
func (cp *changePanel) reset(cwd string) {
	cp.cwd = cwd
	cp.gen++
	cp.set, cp.loaded, cp.loading, cp.again = changeSet{}, false, false, false
	cp.roots, cp.kids, cp.files, cp.seen = nil, nil, nil, map[string]bool{}
	cp.tree.Open.Clear()
	cp.chosen.Clear()
	cp.selected, cp.view = "", nil
}

// refreshChanges 在后台重新读取更改；正在读时记下，读完后再读一次，不并发地跑两个 git。
func (a *App) refreshChanges(ch *chat) {
	cp := ch.changes
	if cp.loading {
		cp.again = true
		return
	}
	cp.loading = true
	cwd, gen := cp.cwd, cp.gen
	go func() {
		set := loadChanges(cwd)
		a.win.Update(func() {
			if cp.gen != gen { // 读的时候换了工作目录的，结果作废
				return
			}
			cp.loading = false
			cp.set, cp.loaded = set, true
			cp.buildTree()
			if _, ok := cp.files[cp.selected]; !ok {
				cp.selected = cp.firstFile()
			}
			cp.choose(cp.selected)
			a.loadDiff(ch)
			if cp.again {
				cp.again = false
				a.refreshChanges(ch)
			}
		})
	}()
}

// choose 在树里选中 rel；为空时不选。
func (cp *changePanel) choose(rel string) {
	cp.chosen.Clear()
	if rel != "" {
		cp.chosen.Add(rel)
	}
}

// showChosen 显示用户在树里选中的文件的对比；选的是目录，或文件已经显示着时什么也不做。
func (a *App) showChosen(ch *chat) {
	cp := ch.changes
	for item := range cp.chosen.All() {
		if _, ok := cp.files[item]; ok && item != cp.selected {
			cp.selected = item
			a.loadDiff(ch)
			return
		}
	}
}

// loadDiff 在后台读出选中文件的对比。是同一个文件时保留已有的对比，只换文本，滚动位置不变。
func (a *App) loadDiff(ch *chat) {
	cp := ch.changes
	chg, ok := cp.files[cp.selected]
	if !ok {
		cp.view = nil
		return
	}
	cwd, gen, hasHead := cp.cwd, cp.gen, cp.set.hasHead
	if cp.view == nil || cp.view.path != chg.path {
		cp.view = &changeView{path: chg.path}
	}
	view := cp.view
	view.seq++
	seq := view.seq
	go func() {
		before, after, note := readChange(cwd, chg, hasHead)
		a.win.Update(func() {
			if cp.gen != gen || cp.view != view || view.seq != seq { // 读的时候又换了的，结果作废
				return
			}
			view.show(before, after, note)
		})
	}()
}

// show 用读到的文本换掉对比里的；note 不空时不能显示。
func (v *changeView) show(before, after, note string) {
	v.note = note
	if note != "" {
		v.diff = nil
		return
	}
	// 末尾的换行不显示成多出来的一行。
	before, after = strings.TrimSuffix(before, "\n"), strings.TrimSuffix(after, "\n")
	if v.diff != nil {
		if v.diff.Old.String() != before {
			v.diff.Old.Reset(before)
		}
		if v.diff.New.String() != after {
			v.diff.New.Reset(after)
		}
		return
	}
	old, text := editor.NewBuffer(before), editor.NewBuffer(after)
	lang := editor.LanguageFor(v.path)
	old.SetLanguage(lang)
	text.SetLanguage(lang)
	v.diff = &editor.DiffState{Old: old, New: text, Wrap: true, Font: codeFont}
}

// buildTree 由读到的更改建出树：目录在前、文件在后，各自按名称不分大小写排序。第一次见到
// 的目录默认展开。
func (cp *changePanel) buildTree() {
	cp.kids, cp.files = map[string][]string{}, map[string]fileChange{}
	linked := map[string]bool{} // 已经挂到上一层的条目
	var top []string
	for _, f := range cp.set.files {
		cp.files[f.path] = f
		item := f.path
		for !linked[item] {
			linked[item] = true
			dir := path.Dir(item)
			if dir == "." {
				top = append(top, item)
				break
			}
			cp.kids[dir] = append(cp.kids[dir], item)
			item = dir
		}
	}
	byName := func(x, y string) int {
		if dx, dy := cp.kids[x] != nil, cp.kids[y] != nil; dx != dy {
			if dx {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(strings.ToLower(path.Base(x)), strings.ToLower(path.Base(y))),
			cmp.Compare(x, y))
	}
	slices.SortFunc(top, byName)
	cp.roots = top
	for dir, items := range cp.kids {
		slices.SortFunc(items, byName)
		if !cp.seen[dir] {
			cp.seen[dir] = true
			cp.tree.Open.Add(dir)
		}
	}
}

// firstFile 返回树里从上往下的第一个文件，没有文件时为空。
func (cp *changePanel) firstFile() string {
	var find func(items []string) string
	find = func(items []string) string {
		for _, item := range items {
			if cp.kids[item] == nil {
				return item
			}
			if f := find(cp.kids[item]); f != "" {
				return f
			}
		}
		return ""
	}
	return find(cp.roots)
}

// children 返回条目 item 的子项，文件没有（nil）。
func (cp *changePanel) children(item string) []string { return cp.kids[item] }
