package gui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/host"
)

const (
	// groupLimit 是每组默认显示的会话数，其余收在“显示更多”之后。
	groupLimit = 8
	// timeWidth 是行尾时间的槽宽：宽度固定，各行标题截断在同一处，悬停换成按钮时也不跳动。
	timeWidth = 44
	// moreSize 是行尾 ⋯ 按钮的边长。
	moreSize = 20
	// runningSize 是行尾运行中指示器的边长。
	runningSize = 16
)

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

// sidebar 构建左栏：顶部是搜索框与入口，其下按工作目录分组列出会话；
// 选择与键盘操作由 ui.Sidebar 负责。
func (a *App) sidebar(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		// 顶部留给窗口控件，同时可以拖动窗口。
		ui.Row(c).Height(titleBarHeight(c)).DragWindow()
		a.entries(c, t)
		// 尚未创建的新对话不在列表里，此时列表中没有选中项。
		selected := a.current.id
		list := ui.Sidebar(c, &selected, func() {
			for _, g := range groupByDir(a.infos) {
				a.group(c, t, g, selected)
			}
		}).Grow(1).Padding(6, 8, 6)
		a.keyNav = list.FocusVisible()
		if list.Changed() {
			a.open(selected)
		}
		if a.err != "" {
			ui.Text(c, a.err).FontSize(smallSize).TextColor(t.Danger).Selectable().Padding(8)
		}
	})
}

// entries 画列表上方不随之滚动的部分：搜索框，以及新对话与定时任务两个入口。
func (a *App) entries(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Padding(0, 8, 6).Gap(2).Children(func() {
		ui.SearchField(c, &a.query).Label("搜索").Margin(0, 0, 6)
		entry(c, t, plusIcon, "新对话", true, a.newChat)
		entry(c, t, clockIcon, "定时任务", false, nil)
	})
}

// entry 画一个入口：图标加文字，外观与会话行一致；click 为空时是尚未实现的占位，点击没有反应。
// primary 的图标放在圆形底色里，突出它是主要操作。
func entry(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, primary bool, click func(),
) {
	btn := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(4, 8).
		Radius(controlRound).Cursor(ui.CursorPointer).Label(label)
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	if click != nil {
		btn.OnClick(click)
	}
	btn.Children(func() {
		slot := ui.Row(c).Size(20, 20).Shrink(0).Radius(10).Justify(ui.Center).AlignItems(ui.Center)
		size, color := float32(iconSize), t.TextMuted
		if primary {
			slot.Background(buttonFill(t))
			size, color = smallIconSize, t.Text
		}
		slot.Children(func() { ui.Icon(c, glyph).FontSize(size).TextColor(color) })
		ui.Text(c, label).SingleLine()
	})
}

// group 构建一个目录分组：标题可折叠，折叠后标题尾部显示会话数；
// 默认只列出最近的 groupLimit 个，其余在“显示更多”之后。
// 悬停标题显示完整路径，右键菜单可在该目录新建对话。
func (a *App) group(c *ui.Context, t *ui.Theme, g group, selected string) {
	open := a.sections[g.cwd]
	if open == nil {
		open = new(true)
		a.sections[g.cwd] = open
	}
	title := g.title
	if !*open {
		title += " · " + strconv.Itoa(len(g.infos))
	}
	shown := a.visible(g, selected)
	ui.SidebarSection(c, title, open, func() {
		for _, info := range shown {
			a.session(c, t, info, selected)
		}
		if hidden := len(g.infos) - len(shown); hidden > 0 {
			a.showMore(c, t, g.cwd, hidden)
		}
	}).Tooltip(g.cwd).ContextMenu(func(m *ui.Menu) {
		if m.Item("在此目录新建对话").Chosen() {
			a.cwd = g.cwd
			a.newChat()
		}
		m.Separator()
		dirItems(m, g.cwd)
	})
}

// visible 返回分组要列出的会话：默认只有最近的 groupLimit 个；点过“显示更多”，
// 或当前会话排在限额之外时是全部，免得当前会话从列表里消失。
func (a *App) visible(g group, selected string) []host.Info {
	if len(g.infos) <= groupLimit || a.more[g.cwd] {
		return g.infos
	}
	for _, info := range g.infos[groupLimit:] {
		if info.ID == selected {
			return g.infos
		}
	}
	return g.infos[:groupLimit]
}

// showMore 画分组末尾的“显示更多”，点一下列出该目录的全部会话。
func (a *App) showMore(c *ui.Context, t *ui.Theme, cwd string, hidden int) {
	btn := ui.ButtonBase(c).Justify(ui.Start).Padding(4, 8).Radius(controlRound).
		Cursor(ui.CursorPointer).Label("显示更多")
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	btn.OnClick(func() { a.more[cwd] = true })
	btn.Children(func() {
		text := fmt.Sprintf("显示更多（%d）", hidden)
		ui.Text(c, text).FontSize(textSize).TextColor(t.TextMuted)
	})
}

// sidebarItem 构建一个侧栏项，外观换成 Atlas 的：行更紧凑，选中时是浅色底而不是强调色。
func sidebarItem(c *ui.Context, t *ui.Theme, id, label, selected string) ui.Element {
	row := ui.SidebarItem(c, id, nil, label).Padding(4, 8)
	if id == selected {
		row.Background(t.Text.Alpha(0.09)).TextColor(t.Text)
	}
	return row
}

// session 构建一个会话项：标题之后是状态与行尾；行尾平时是相对时间，运行中是脉动的
// 指示器，悬停时换成 ⋯ 菜单。右键菜单与 ⋯ 相同；悬停提示给出完整标题、目录与更新时间。
func (a *App) session(c *ui.Context, t *ui.Theme, info host.Info, selected string) {
	label := info.Name
	if label == "" {
		label = "新会话"
	}
	menu := a.sessionMenu(info)
	tip := label + "\n" + info.Cwd + "\n" + info.UpdatedAt.Local().Format("2006-01-02 15:04")
	row := sidebarItem(c, t, info.ID, label, selected).ContextMenu(menu).Tooltip(tip)
	hovered := row.Hovered()
	ch := a.chats[info.ID]
	row.Children(func() {
		status(c, t, ch)
		trail(c, t, info.UpdatedAt, ch != nil && ch.running, hovered, menu)
	})
}

// status 在行尾之前画会话的状态点：出错是红点，运行结束但还没看过是蓝点。
// 运行中由行尾的指示器表示。
func status(c *ui.Context, t *ui.Theme, ch *chat) {
	if ch == nil {
		return
	}
	dot := func(color ui.Color) { ui.Row(c).Size(8, 8).Shrink(0).Radius(4).Background(color) }
	switch {
	case ch.err != "":
		dot(t.Danger)
	case ch.unread:
		dot(t.Accent)
	}
}

// trail 画行尾：定宽的槽里平时是相对时间，运行中换成脉动的指示器，悬停时换成 ⋯ 按钮。
// 按钮始终存在，不悬停时只是零尺寸且禁用：菜单的选择要在它所在的下一帧回报，
// 那时指针已在菜单上，悬停的状态可能已经没了。它用固定的键标识，因为它前面的时间或
// 指示器时有时无，按位置算的标识会变，选择就找不到它了。
func trail(
	c *ui.Context, t *ui.Theme, updated time.Time, running, hovered bool, menu func(*ui.Menu),
) {
	ui.Row(c).Width(timeWidth).Height(moreSize).Shrink(0).Justify(ui.End).AlignItems(ui.Center).
		Children(func() {
			size := float32(0)
			switch {
			case hovered:
				size = moreSize
			case running:
				pulse := loading.Options{Size: runningSize, Color: t.TextMuted}
				loading.Pulse(c, pulse).Label("运行中")
			default:
				ui.Text(c, ago(updated)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
			}
			more := ui.ButtonBase(c.Key("more")).Size(size, size).Shrink(0).Radius(controlRound).
				Cursor(ui.CursorPointer).Label("更多").Disabled(!hovered)
			if hovered && more.Hovered() {
				more.Background(t.Text.Alpha(0.1))
			}
			more.Menu(menu)
			if hovered {
				more.Children(func() {
					ui.Icon(c, moreIcon).FontSize(iconSize).TextColor(t.TextMuted)
				})
			}
		})
}

// ago 把过去的时刻说成简短的相对时间，用在会话行的行尾。
func ago(at time.Time) string {
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d 天", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 周", int(d.Hours()/(7*24)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d 个月", int(d.Hours()/(30*24)))
	}
	return fmt.Sprintf("%d 年", int(d.Hours()/(365*24)))
}

// sessionMenu 是会话的菜单，右键与行尾的 ⋯ 共用。
func (a *App) sessionMenu(info host.Info) func(*ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item("重命名…").Chosen() {
			a.renameID, a.renameText, a.renameOpen = info.ID, info.Name, true
		}
		m.Separator()
		dirItems(m, info.Cwd)
		m.Separator()
		if m.Item("删除").Chosen() {
			a.deleteID, a.deleteOpen = info.ID, true
		}
	}
}

// dirItems 是针对一个目录的菜单项：在文件管理器中显示，复制路径。
func dirItems(m *ui.Menu, dir string) {
	if m.Item(revealLabel()).Chosen() {
		mygo.Shell.ShowItemInFolder(dir)
	}
	if m.Item("复制目录路径").Chosen() {
		mygo.Clipboard.WriteText(dir)
	}
}

// revealLabel 是“在文件管理器中显示”在当前平台上的叫法。
func revealLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "在访达中显示"
	case "windows":
		return "在资源管理器中显示"
	}
	return "在文件管理器中显示"
}
