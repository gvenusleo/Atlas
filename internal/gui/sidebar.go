package gui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/host"
)

// newChatID 是侧栏中“新对话”项的标识，不会与会话 ID 冲突。
const newChatID = "new"

// group 是侧栏中同一工作目录下的会话。
type group struct {
	cwd   string
	title string
	infos []host.Info
}

// groupByDir 按工作目录分组，目录名为组名；组与组内会话都保持 infos 的顺序（最近更新在前）。
// 同名的不同目录用上一级目录区分。
func groupByDir(infos []host.Info) []group {
	var groups []group
	index := map[string]int{}
	for _, info := range infos {
		i, ok := index[info.Cwd]
		if !ok {
			i = len(groups)
			index[info.Cwd] = i
			groups = append(groups, group{cwd: info.Cwd, title: filepath.Base(info.Cwd)})
		}
		groups[i].infos = append(groups[i].infos, info)
	}
	names := map[string]int{}
	for _, g := range groups {
		names[g.title]++
	}
	for i, g := range groups {
		if names[g.title] > 1 {
			groups[i].title = filepath.Base(filepath.Dir(g.cwd)) + "/" + g.title
		}
	}
	return groups
}

// sidebar 构建会话列表：顶部是新对话，其下按工作目录分组；选择与键盘操作由 ui.Sidebar 负责。
func (a *App) sidebar(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		// 顶部留给窗口控件，同时可以拖动窗口。
		ui.Row(c).Height(titleBarHeight(c)).DragWindow()
		selected := a.current.id
		if selected == "" {
			selected = newChatID
		}
		list := ui.Sidebar(c, &selected, func() {
			sidebarItem(c, t, newChatID, "新对话", selected)
			for _, g := range groupByDir(a.infos) {
				ui.SidebarSection(c, g.title, nil, func() {
					for _, info := range g.infos {
						a.session(c, t, info, selected)
					}
				})
			}
		}).Grow(1).Padding(6, 8, 6)
		a.keyNav = list.FocusVisible()
		if list.Changed() {
			a.choose(selected)
		}
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Selectable().Padding(8)
		}
	})
}

// choose 响应侧栏的选择：新对话项开始空白会话，其余打开对应会话。
func (a *App) choose(id string) {
	if id == newChatID {
		a.newChat()
		return
	}
	a.open(id)
}

// sidebarItem 构建一个侧栏项，外观换成 Atlas 的：行更紧凑，选中时是浅色底而不是强调色。
func sidebarItem(c *ui.Context, t *ui.Theme, id, label, selected string) ui.Element {
	row := ui.SidebarItem(c, id, nil, label).Padding(4, 8)
	if id == selected {
		row.Background(t.Text.Alpha(0.09)).TextColor(t.Text)
	}
	return row
}

// session 构建一个会话项；运行中的会话末尾有一个小圆点，右键菜单可重命名与删除。
func (a *App) session(c *ui.Context, t *ui.Theme, info host.Info, selected string) {
	label := info.Name
	if label == "" {
		label = "新会话"
	}
	row := sidebarItem(c, t, info.ID, label, selected).ContextMenu(func(m *ui.Menu) {
		if m.Item("重命名…").Chosen() {
			a.renameID, a.renameText, a.renameOpen = info.ID, info.Name, true
		}
		m.Separator()
		if m.Item("删除").Chosen() {
			a.deleteID, a.deleteOpen = info.ID, true
		}
	})
	if ch := a.chats[info.ID]; ch != nil && ch.running {
		row.Children(func() {
			ui.Row(c).Size(8, 8).Radius(4).Background(t.Warning)
		})
	}
}
