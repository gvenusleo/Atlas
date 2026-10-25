package gui

import (
	"fmt"
	"path"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// changesCard 画更改面板的卡片：顶部是标题栏，其下左边是更改的文件树，右边是选中文件的对比。
// height 为 0 时占满所在列余下的高度。
func (a *App) changesCard(c *ui.Context, t *ui.Theme, ch *chat, height float32) {
	cp := ch.changes
	if cp.cwd != ch.cwd {
		moved := cp.cwd != "" // 之前是另一个工作目录，监视要换到新的
		cp.reset(ch.cwd)
		a.refreshChanges(ch)
		if moved {
			a.watchFiles(ch)
		}
	}
	sideCard(c, t, "changes-card", height).Children(func() {
		a.changeBar(c, t, ch)
		switch {
		case cp.set.err != "":
			ui.Text(c, cp.set.err).TextColor(t.Danger).FontSize(textSize).Selectable().Padding(12)
		case !cp.loaded:
			ui.Text(c, "正在读取…").TextColor(t.TextMuted).FontSize(textSize).Padding(12)
		case len(cp.set.files) == 0:
			emptyChanges(c, t)
		default:
			cp.treeWidth = min(max(cp.treeWidth, treeMin), max(a.sideWidth/2, treeMin))
			ui.Split(c, &cp.treeWidth, func() { a.changeOutline(c, t, ch) },
				func() { a.changeContent(c, t, ch) }).Grow(1).MinHeight(0)
		}
	})
}

// changeBar 画卡片顶部的标题栏：标题与文件数，右端是刷新、对比布局的切换与关闭按钮；
// 空白处可拖动窗口。
func (a *App) changeBar(c *ui.Context, t *ui.Theme, ch *chat) {
	cp := ch.changes
	ui.Row(c).Height(tabBarHeight(c)).AlignItems(ui.Center).Gap(6).Padding(0, tabInset, 0, 12).
		DragWindow().Children(func() {
		ui.Text(c, "更改").FontSize(textSize)
		if n := len(cp.set.files); n > 0 {
			ui.Badge(c, strconv.Itoa(n))
		}
		ui.Row(c).Grow(1)
		label := "刷新"
		if cp.loading {
			label = "读取中…"
		}
		textButton(c, t, label, "重新读取更改", func() { a.refreshChanges(ch) })
		layout := "并排"
		if cp.split {
			layout = "合并"
		}
		textButton(c, t, layout, "切换并排与合并显示", func() { cp.split = !cp.split })
		a.cardActions(c, t, ch, changesPanel, "关闭更改面板", a.toggleChanges)
	})
}

// changeOutline 画更改的文件树：目录有箭头，文件有类型图标，其后是名称、增删行数与更改类型。
// 用户点一下文件，显示它的对比。
func (a *App) changeOutline(c *ui.Context, t *ui.Theme, ch *chat) {
	cp := ch.changes
	var clicked string // 点了的目录，树构建完再处理
	outlineTree(c, t, &cp.tree, &cp.chosen, cp.roots, cp.children,
		func(item string) { a.changeItem(c, t, ch, item, &clicked) }).
		OnChange(func() { a.showChosen(ch) })
	if clicked != "" {
		chooseDir(&cp.chosen, &cp.tree.Open, clicked)
	}
}

// changeItem 画树里一行的内容（箭头之后）：图标与名称，文件还有增删行数与更改类型。
// 右键菜单与文件面板的文件树一样。点了目录行的内容，目录记进 clicked。
func (a *App) changeItem(c *ui.Context, t *ui.Theme, ch *chat, item string, clicked *string) {
	chg, isFile := ch.changes.files[item]
	exists := !isFile || chg.kind != changeDeleted
	row := treeItem(c, t, isFile).
		ContextMenu(func(m *ui.Menu) { pathItems(m, ch.cwd, item, exists) })
	row.Children(func() {
		if isFile {
			ui.Image(c, fileIcon(item)).Size(smallIconSize, smallIconSize).Shrink(0)
		}
		ui.Text(c, path.Base(item)).FontSize(textSize).SingleLine().Grow(1).MinWidth(0)
		if isFile {
			changeTags(c, t, chg)
		}
	})
	if !isFile && row.Clicked() {
		*clicked = item
	}
}

// changeTags 画文件行右端的增删行数与更改类型。
func changeTags(c *ui.Context, t *ui.Theme, chg fileChange) {
	tag := func(text string, color ui.Color) {
		ui.Text(c, text).FontSize(smallSize).TextColor(color).SingleLine().Shrink(0)
	}
	if chg.added > 0 {
		tag("+"+strconv.Itoa(chg.added), t.Success)
	}
	if chg.removed > 0 {
		tag("−"+strconv.Itoa(chg.removed), t.Danger)
	}
	color := t.Warning
	switch chg.kind {
	case changeAdded:
		color = t.Success
	case changeDeleted, changeConflict:
		color = t.Danger
	case changeRenamed:
		color = t.Accent
	}
	tag(chg.kind.letter(), color)
}

// changeContent 画内容区：选中文件的路径、统计与对比。
func (a *App) changeContent(c *ui.Context, t *ui.Theme, ch *chat) {
	cp := ch.changes
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		v := cp.view
		if v == nil {
			emptyChanges(c, t)
			return
		}
		ui.Column(c.Key("change-" + v.path)).Grow(1).MinHeight(0).Children(func() {
			changeHeader(c, t, v)
			switch {
			case v.note != "":
				ui.Text(c, v.note).FontSize(smallSize).TextColor(t.TextMuted).Padding(8, 14)
			case v.diff == nil:
				ui.Text(c, "正在读取…").FontSize(smallSize).TextColor(t.TextMuted).Padding(8, 14)
			default:
				v.diff.Font = codeFont
				v.diff.Layout = editor.DiffUnified
				if cp.split {
					v.diff.Layout = editor.DiffSplit
				}
				editor.DiffView(c, v.diff).Grow(1).FillWidth()
			}
		})
	})
}

// changeHeader 画内容区顶部：第一行是路径，所在的目录淡一些、文件名在最后；第二行是增删统计
// 与改动间的跳转。
func changeHeader(c *ui.Context, t *ui.Theme, v *changeView) {
	ui.Column(c).Padding(4, 8, 4, 14).Children(func() {
		ui.Row(c).Height(tabHeight).AlignItems(ui.Center).Children(func() {
			pathLabel(c, t, v.path)
		})
		if v.diff != nil {
			ui.Row(c).AlignItems(ui.Center).Gap(2).ClipX().Children(func() {
				changeStats(c, t, v.diff)
			})
		}
	})
}

// changeStats 画增删统计与改动间的跳转：“+3 −1”、当前是第几处，以及上一处与下一处。
func changeStats(c *ui.Context, t *ui.Theme, d *editor.DiffState) {
	added, removed := d.Stats()
	if added+removed > 0 {
		diffPill(c, t, added, removed)
	}
	n := len(d.Hunks())
	if n == 0 {
		return
	}
	i, inside := d.CurrentHunk()
	label := fmt.Sprintf("%d 处", n)
	if i >= 0 {
		label = fmt.Sprintf("%d / %d", i+1, n)
	}
	ui.Text(c, label).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().Padding(0, 4)
	iconButton(c, t, arrowUpIcon, "上一处改动", barButton, func() {
		if inside {
			i--
		}
		if i >= 0 {
			d.GoToHunk(i)
		}
	})
	iconButton(c, t, arrowDownIcon, "下一处改动", barButton, func() {
		if i+1 < n {
			d.GoToHunk(i + 1)
		}
	})
}

// emptyChanges 在没有更改、或没有可显示的文件时说明情况。
func emptyChanges(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Padding(24).
		Children(func() {
			ui.Icon(c, diffIcon).FontSize(emptyIconSize).TextColor(t.TextMuted)
			ui.Text(c, "没有更改").FontSize(bodySize)
			ui.Text(c, "工作目录与 HEAD 一致。").FontSize(smallSize).TextColor(t.TextMuted)
		})
}
