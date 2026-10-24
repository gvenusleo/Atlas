package gui

import "github.com/egoist/mygo/ui"

// 文件面板的文件树与更改面板的更改树共用的部分。树是 MyGo 的 Outline，条目是相对工作目录的
// 路径（用 / 分隔），目录与文件各有自己的路径；选中的条目放在 ui.Selection 里，按路径记，
// 不依赖行号，行因展开收起而移动时选中跟着条目走。

// outlineTree 画一棵用本应用样式的树：children 返回目录的子项、文件为 nil，item 画一行箭头
// 之后的内容，chosen 是选中的条目。返回树的元素，调用方用 OnChange 接用户的选择：只有用户
// 点击或按方向键时才会触发，程序改 chosen 不会。
func outlineTree(
	c *ui.Context, t *ui.Theme, tree *ui.OutlineState[string], chosen *ui.Selection[string],
	roots []string, children func(string) []string, item func(string),
) ui.Element {
	tree.List.Selection = chosen
	tree.Indent = treeIndent
	var el ui.Element
	quietRows(c, t, func() {
		el = ui.Outline(c, tree, roots, children, item).Grow(1).Padding(4, 8, 8)
	})
	return el
}

// treeItem 是树里一行箭头之后的内容的容器。Outline 给每一行（文件也一样）留一个箭头的位置，
// 文件缩进过多；leaf 为真时把内容左移这个宽度，让文件的图标落在目录箭头的位置。点击不夺走
// 树的键盘焦点。
func treeItem(c *ui.Context, t *ui.Theme, leaf bool) ui.Element {
	row := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1)).KeepFocus()
	if leaf {
		row.Margin(0, 0, 0, -(t.Space(4) + t.Space(1)))
	}
	return row
}

// chooseDir 选中目录 dir 并展开或收起它：点目录行的内容与点它的箭头一样。目录行的内容容器
// 接走了点击，列表不会再选中这一行，所以自己选。要等树构建完再调用，不在构建中改集合。
func chooseDir(chosen, open *ui.Selection[string], dir string) {
	chosen.Clear()
	chosen.Add(dir)
	if open.Has(dir) {
		open.Remove(dir)
	} else {
		open.Add(dir)
	}
}
