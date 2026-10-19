package gui

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// treeRow 是文件树里要画的一行。
type treeRow struct {
	rel, name string // 相对工作目录的路径与文件名
	dir, open bool
	depth     int
}

// treeRows 把展开着的目录压平成要画的行；还没读完的目录先不列。
func (fp *filePanel) treeRows() []treeRow {
	var rows []treeRow
	var walk func(rel string, depth int)
	walk = func(rel string, depth int) {
		for _, e := range fp.dirs[rel] {
			p := path.Join(rel, e.name)
			open := e.dir && fp.expanded[p]
			rows = append(rows, treeRow{p, e.name, e.dir, open, depth})
			if open {
				walk(p, depth+1)
			}
		}
	}
	walk("", 0)
	return rows
}

// fileTree 画文件树一栏：上方是筛选框，其下是文件树，筛选时换成匹配的文件。
func (a *App) fileTree(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	ui.Column(c).Fill().Children(func() {
		// 筛选框不要底色、字小一点、上下内边距压紧，不抢文件树的视线。
		ui.SearchField(c, &fp.filter).Label("筛选文件").FontSize(12).
			Background(ui.Transparent).Padding(3, 8).Margin(8, 8, 4)
		if fp.filter != "" {
			a.filterResults(c, t, ch)
			return
		}
		rows := fp.treeRows()
		fp.list.Key = func(i int) any { return rows[i].rel }
		ui.List(c, &fp.list, len(rows), func(i int) { a.treeRowView(c, t, ch, rows[i]) }).
			Grow(1).Padding(4, 8, 8)
	})
}

// treeRowView 画文件树里的一行：目录是箭头，文件是类型图标，其后是名称；点一下，
// 目录展开或收起，文件在新标签里打开。当前标签的文件所在的行有底色。
func (a *App) treeRowView(c *ui.Context, t *ui.Theme, ch *chat, r treeRow) {
	row := ui.ButtonBase(c).Height(treeRowHeight).Justify(ui.Start).AlignItems(ui.Center).Gap(6).
		Padding(0, 6, 0, 6+float32(r.depth)*treeIndent).Radius(controlRound).
		Cursor(ui.CursorPointer).Label(r.name)
	switch {
	case !r.dir && ch.files.activePath() == filepath.Join(ch.cwd, r.rel):
		row.Background(t.SurfaceHover)
	case row.Hovered():
		row.Background(rowFill(t))
	}
	row.OnClick(func() {
		if r.dir {
			a.toggleDir(ch, r.rel)
			return
		}
		a.openFile(ch, r.rel)
	})
	row.Children(func() {
		ui.Row(c).Size(fileIconSize, fileIconSize).Shrink(0).Justify(ui.Center).
			AlignItems(ui.Center).Children(func() {
			if !r.dir {
				ui.Image(c, fileIcon(r.rel)).Size(fileIconSize, fileIconSize)
				return
			}
			angle := float32(0)
			if r.open {
				angle = 90
			}
			ui.Icon(c, chevronIcon).FontSize(14).TextColor(t.TextMuted).Rotate(angle)
		})
		ui.Text(c, r.name).FontSize(13).SingleLine()
	})
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
			ui.Text(c, "没有匹配的文件").FontSize(12).TextColor(t.TextMuted).Padding(8)
		}
	})
}

// resultRow 画筛选结果里的一行：类型图标，文件名，其下是所在的目录。
func (a *App) resultRow(c *ui.Context, t *ui.Theme, ch *chat, p string) {
	dir, name := path.Split(p)
	row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 6).
		Radius(controlRound).Cursor(ui.CursorPointer).Label(p)
	switch {
	case ch.files.activePath() == filepath.Join(ch.cwd, p):
		row.Background(t.SurfaceHover)
	case row.Hovered():
		row.Background(rowFill(t))
	}
	row.OnClick(func() { a.openFile(ch, p) })
	row.Children(func() {
		ui.Image(c, fileIcon(p)).Size(fileIconSize, fileIconSize).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(13).SingleLine()
			if dir != "" {
				ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(11).TextColor(t.TextMuted).
					SingleLine()
			}
		})
	})
}
