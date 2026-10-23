package gui

import (
	"path"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// pathMenu 是文件树与筛选结果里一项（文件或目录）的右键菜单：复制名称或路径、
// 在文件管理器中显示、移到废纸篓。rel 是它相对工作目录的路径。
func (a *App) pathMenu(ch *chat, rel string) func(*ui.Menu) {
	abs := filepath.Join(ch.cwd, rel)
	return func(m *ui.Menu) {
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
		if m.Item(revealLabel()).Chosen() {
			mygo.Shell.ShowItemInFolder(abs)
		}
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
