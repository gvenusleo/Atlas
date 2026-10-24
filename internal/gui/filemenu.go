package gui

import (
	"path"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// pathItems 是所有文件树共用的菜单项：复制名称或路径、在文件管理器中显示。rel 是文件或目录
// 相对工作目录 cwd 的路径；exists 为假（文件已被删除）时，在文件管理器中显示不可选。
func pathItems(m *ui.Menu, cwd, rel string, exists bool) {
	abs := filepath.Join(cwd, rel)
	m.Submenu("复制", func(m *ui.Menu) {
		if m.Item("文件名").Chosen() {
			mygo.Clipboard.WriteText(path.Base(rel))
		}
		if m.Item("相对路径").Chosen() {
			mygo.Clipboard.WriteText(rel)
		}
		if m.Item("绝对路径").Chosen() {
			mygo.Clipboard.WriteText(abs)
		}
	})
	if m.Item(revealLabel()).Disabled(!exists).Chosen() {
		mygo.Shell.ShowItemInFolder(abs)
	}
}

// pathMenu 是文件面板的文件树与筛选结果里一项（文件或目录）的右键菜单：共用的菜单项，
// 以及移到废纸篓。
func (a *App) pathMenu(ch *chat, rel string) func(*ui.Menu) {
	return func(m *ui.Menu) {
		pathItems(m, ch.cwd, rel, true)
		m.Separator()
		if m.Item("删除").Chosen() {
			a.trash(ch, rel)
		}
	}
}

// trash 把 rel 移到废纸篓，失败时提示原因；成功后立即重读它所在的目录，不等文件监视。
func (a *App) trash(ch *chat, rel string) {
	if err := mygo.Shell.TrashItem(filepath.Join(ch.cwd, rel)); err != nil {
		a.toast = "删除失败：" + err.Error()
		return
	}
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	a.readDir(ch, dir)
}
