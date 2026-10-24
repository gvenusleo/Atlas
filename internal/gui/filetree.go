package gui

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// noKids 是还没读完的目录的子项：空而不是 nil，树据此知道它是目录，先显示箭头。
var noKids = []string{}

// children 返回条目 item 的子项，文件没有（nil）。
func (fp *filePanel) children(item string) []string {
	if kids, read := fp.kids[item]; read {
		return kids
	}
	if fp.isDir[item] {
		return noKids
	}
	return nil
}

// activeRel 返回当前标签的文件相对工作目录 cwd 的路径；没有标签，或文件不在 cwd 下时为空。
func (fp *filePanel) activeRel(cwd string) string {
	abs := fp.activePath()
	if abs == "" {
		return ""
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// fileTree 画文件树一栏：上方是筛选框，其下是文件树，筛选时换成匹配的文件。
func (a *App) fileTree(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	ui.Column(c).Fill().Children(func() {
		// 筛选框不要底色、字小一点、上下内边距压紧，不抢文件树的视线。
		ui.SearchField(c, &fp.filter).Label("筛选文件").FontSize(smallSize).
			Background(ui.Transparent).Padding(3, 8).Margin(8, 8, 4)
		if fp.filter != "" {
			a.filterResults(c, t, ch)
			return
		}
		a.fileOutline(c, t, ch)
	})
}

// fileOutline 画文件树：目录有箭头，文件有类型图标，其后是名称。用户点一下文件，在标签里打开
// 它；当前标签的文件是选中的。新展开的目录在这里读取。
func (a *App) fileOutline(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	if active := fp.activeRel(ch.cwd); active != fp.shown { // 当前标签的文件换了：选中它
		fp.shown = active
		fp.chosen.Clear()
		if active != "" {
			fp.chosen.Add(active)
		}
	}
	var clicked string // 点了的目录，树构建完再处理
	outlineTree(c, t, &fp.tree, &fp.chosen, fp.kids[""], fp.children,
		func(item string) { a.fileItem(c, t, ch, item, &clicked) }).
		OnChange(func() { a.openChosen(ch) })
	if clicked != "" {
		chooseDir(&fp.chosen, &fp.tree.Open, clicked)
	}
	opened := make(map[string]bool)
	for dir := range fp.tree.Open.All() {
		opened[dir] = true
		if !fp.opened[dir] && fp.isDir[dir] {
			a.readDir(ch, dir) // 新展开的：读取，读完之前先显示旧的
		}
	}
	fp.opened = opened
}

// openChosen 打开用户在树里选中的文件；选的是目录，或文件已经是当前标签时什么也不做。
func (a *App) openChosen(ch *chat) {
	fp := ch.files
	active := fp.activeRel(ch.cwd)
	for item := range fp.chosen.All() {
		if !fp.isDir[item] && item != active {
			a.openFile(ch, item)
			return
		}
	}
}

// fileItem 画文件树里一行的内容（箭头之后）：文件的类型图标与名称。右键菜单见 pathMenu。
// 点了目录行的内容，目录记进 clicked。
func (a *App) fileItem(c *ui.Context, t *ui.Theme, ch *chat, item string, clicked *string) {
	isDir := ch.files.isDir[item]
	row := treeItem(c, t, !isDir).ContextMenu(a.pathMenu(ch, item))
	row.Children(func() {
		if !isDir {
			ui.Image(c, fileIcon(item)).Size(smallIconSize, smallIconSize).Shrink(0)
		}
		ui.Text(c, path.Base(item)).FontSize(textSize).SingleLine().Grow(1).MinWidth(0)
	})
	if isDir && row.Clicked() {
		*clicked = item
	}
}

// filterResults 画筛选框的结果：路径里包含输入内容（忽略大小写）的文件，不含目录。
func (a *App) filterResults(c *ui.Context, t *ui.Theme, ch *chat) {
	fp, ix := ch.files, &ch.index
	if ix.stale(ch.cwd) {
		a.loadPaths(ch)
	}
	found := fp.found.get(rankKey{fp.filter, ix.version}, func() []string {
		files := slices.DeleteFunc(slices.Clone(ix.paths), func(p string) bool {
			return strings.HasSuffix(p, "/")
		})
		return rankPaths(files, fp.filter, mentionLimit)
	})
	ui.Scroll(c).Grow(1).Padding(4, 8, 8).Children(func() {
		for _, p := range found {
			a.resultRow(c, t, ch, p)
		}
		if len(found) == 0 {
			ui.Text(c, "没有匹配的文件").FontSize(smallSize).TextColor(t.TextMuted).Padding(8)
		}
	})
}

// resultRow 画筛选结果里的一行：类型图标，文件名，其下是所在的目录。
func (a *App) resultRow(c *ui.Context, t *ui.Theme, ch *chat, p string) {
	dir, name := path.Split(p)
	row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 6).
		Radius(controlRound).Cursor(ui.CursorPointer).Label(p).ContextMenu(a.pathMenu(ch, p))
	switch {
	case ch.files.activePath() == filepath.Join(ch.cwd, p):
		row.Background(t.SurfaceHover)
	case row.Hovered():
		row.Background(rowFill(t))
	}
	row.OnClick(func() { a.openFile(ch, p) })
	row.Children(func() {
		ui.Image(c, fileIcon(p)).Size(smallIconSize, smallIconSize).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(textSize).SingleLine()
			if dir != "" {
				ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(smallSize).TextColor(t.TextMuted).
					SingleLine()
			}
		})
	})
}
