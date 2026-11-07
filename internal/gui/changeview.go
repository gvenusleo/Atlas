package gui

import (
	"fmt"
	"path"
	"slices"
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
	sideCard(c, t, "changes-card", height).Focusable().Bind(&cp.focus).Children(func() {
		a.changeShortcuts(c, ch)
		a.changeBar(c, t, ch)
		switch {
		case cp.set.err != "":
			ui.Text(c, cp.set.err).TextColor(t.Danger).FontSize(textSize).Selectable().Padding(12)
		case !cp.loaded:
			ui.Text(c, "正在读取…").TextColor(t.TextMuted).FontSize(textSize).Padding(12)
		case len(cp.set.files) == 0:
			emptyChanges(c, t)
		case cp.treeHidden:
			a.changeContent(c, t, ch)
		default:
			cp.treeWidth = min(max(cp.treeWidth, treeMin), max(a.sideWidth/2, treeMin))
			ui.Split(c, &cp.treeWidth, func() { a.changeOutline(c, t, ch) },
				func() { a.changeContent(c, t, ch) }).Grow(1).MinHeight(0)
		}
	})
}

// changeShortcuts 的作用域限于更改卡片，输入框与其他面板不接收这些操作。
func (a *App) changeShortcuts(c *ui.Context, ch *chat) {
	cp := ch.changes
	c.OnShortcut(ui.Alt, ui.KeyPageUp, func() {
		a.stepFile(ch, -1)
		cp.focus.Focus(c)
	})
	c.OnShortcut(ui.Alt, ui.KeyPageDown, func() {
		a.stepFile(ch, 1)
		cp.focus.Focus(c)
	})
	if v := cp.view; v != nil && v.diff != nil {
		c.OnShortcut(ui.Shift, ui.KeyF7, func() { v.diff.GoToHunk(v.findHunk(-1)) })
		c.OnShortcut(0, ui.KeyF7, func() { v.diff.GoToHunk(v.findHunk(1)) })
	}
}

// changeBar 画卡片顶部的标题栏：标题与文件数，右端是对比布局的切换与关闭按钮；
// 空白处可拖动窗口。边距与间距和文件面板的标签栏一致，两者的文件树开关与按钮对齐。
func (a *App) changeBar(c *ui.Context, t *ui.Theme, ch *chat) {
	cp := ch.changes
	ui.Row(c).Height(tabBarHeight(c)).AlignItems(ui.Center).Gap(2).Padding(0, tabInset).
		DragWindow().Children(func() {
		treeToggle(c, t, &cp.treeHidden)
		ui.Text(c, "更改").FontSize(textSize).Padding(0, 6)
		if n := len(cp.set.files); n > 0 {
			ui.Badge(c, strconv.Itoa(n))
		}
		ui.Row(c).Grow(1)
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
	if cp.reveal {
		for i := range cp.tree.Rows() {
			if cp.tree.Item(i) == cp.selected {
				cp.tree.List.ScrollIntoView(i)
				cp.reveal = false
				break
			}
		}
	}
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
		color = accentFg(t)
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
			a.changeHeader(c, t, ch)
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

// changeHeader 将文件导航与文件内的改动导航分开，文件树收起时仍可连续审阅。
func (a *App) changeHeader(c *ui.Context, t *ui.Theme, ch *chat) {
	cp, v := ch.changes, ch.changes.view
	ui.Column(c).Padding(4, 8, 4, 14).Children(func() {
		ui.Row(c).Height(tabHeight).AlignItems(ui.Center).Children(func() {
			pathLabel(c, t, v.path)
			chg := cp.files[v.path]
			textButton(c, t, "打开", "在文件面板打开", func() { a.openChangeFile(ch) }).
				Disabled(chg.kind == changeDeleted)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(2).ClipX().Children(func() {
			a.changeFiles(c, t, ch)
			ui.Spacer(c)
			if v.diff != nil {
				changeStats(c, t, v)
			}
		})
	})
}

// changeFiles 画文件间的导航：上一个、第几个、下一个。
func (a *App) changeFiles(c *ui.Context, t *ui.Theme, ch *chat) {
	cp := ch.changes
	index := slices.Index(cp.paths, cp.selected)
	changeButton(c.Key("previous-file"), t, chevronLeftIcon, "上一个文件", index <= 0,
		func() {
			a.stepFile(ch, -1)
			cp.focus.Focus(c)
		}).Tooltip("上一个文件（Alt+PageUp）")
	ui.Text(c, fmt.Sprintf("文件 %d / %d", index+1, len(cp.paths))).
		FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
	changeButton(c.Key("next-file"), t, chevronIcon, "下一个文件", index+1 >= len(cp.paths),
		func() {
			a.stepFile(ch, 1)
			cp.focus.Focus(c)
		}).Tooltip("下一个文件（Alt+PageDown）")
}

// changeStats 画选中文件的增删统计与改动间的导航：当前是第几处，以及上一处与下一处。
func changeStats(c *ui.Context, t *ui.Theme, v *changeView) {
	d := v.diff
	added, removed := d.Stats()
	if added+removed > 0 {
		diffPill(c, t, added, removed)
	}
	n := len(d.Hunks())
	if n == 0 {
		return
	}
	i, _ := d.CurrentHunk()
	ui.Text(c, fmt.Sprintf("改动 %d / %d", max(i+1, 0), n)).FontSize(smallSize).
		TextColor(t.TextMuted).SingleLine().Padding(0, 4)
	prev, next := v.findHunk(-1), v.findHunk(1)
	changeButton(c.Key("previous-hunk"), t, arrowUpIcon, "上一处改动", prev < 0,
		func() { d.GoToHunk(prev) }).Tooltip("上一处改动（Shift+F7）")
	changeButton(c.Key("next-hunk"), t, arrowDownIcon, "下一处改动", next >= n,
		func() { d.GoToHunk(next) }).Tooltip("下一处改动（F7）")
}

// changeButton 画审阅导航里的方形图标按钮；disabled 时图标变淡。
func changeButton(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, disabled bool, click func(),
) ui.Element {
	btn := quietButton(c, t, label, click).Size(barButton, barButton).Shrink(0).Disabled(disabled)
	color := t.TextMuted
	if disabled {
		color = color.Alpha(0.4)
	}
	btn.Children(func() {
		ui.Icon(c, glyph).FontSize(smallIconSize).TextColor(color)
	})
	return btn
}

// emptyChanges 在没有更改、或没有可显示的文件时说明情况。
func emptyChanges(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Padding(24).
		Children(func() {
			ui.Icon(c, diffIcon).FontSize(emptyIconSize).TextColor(t.TextMuted)
			ui.Text(c, "没有更改").FontSize(bodySize)
			ui.Text(c, "工作目录与 HEAD 一致").FontSize(smallSize).TextColor(t.TextMuted)
		})
}
