package gui

import (
	"os"
	"slices"
	"strconv"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// shell 是会话终端面板里的一个标签。
type shell struct {
	id    int
	term  *terminal.Terminal
	focus ui.Handle // 终端视图的标识，创建后用它把键盘焦点交给终端
}

// toggleTerminal 显示或收起当前会话的终端面板；首次打开时自动开一个终端。
func (a *App) toggleTerminal() {
	ch := a.current
	ch.termOpen = !ch.termOpen
	if ch.termOpen && len(ch.terms) == 0 {
		a.newTerminal(ch)
	}
}

// newTerminal 在会话的工作目录开一个终端标签并切换过去；失败时错误显示在面板里。
func (a *App) newTerminal(ch *chat) {
	a.shells++
	// 会话的目录已经不存在时，shell 无法启动，改用主目录（空表示主目录）。
	dir := ch.cwd
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = ""
	}
	term, err := terminal.New(terminal.Options{
		Dir:       dir,
		Font:      termFont,
		Theme:     termTheme(light, lightANSI),
		DarkTheme: termTheme(dark, darkANSI),
		OnTitle:   func(string) { a.win.Invalidate() },
		OnExit:    func(int) { a.win.Update(ch.pruneTerms) },
	})
	if err != nil {
		ch.termErr = err.Error()
		return
	}
	sh := &shell{id: a.shells, term: term}
	ch.termErr = ""
	ch.terms = append(ch.terms, sh)
	ch.termTab, ch.termReveal = len(ch.terms)-1, sh.id
	sh.focus.Focus()
}

// termFont 是终端的字体：JetBrainsMono Nerd Font Mono，未安装时退回系统等宽字体。
var termFont = terminal.Font{Family: "JetBrainsMono Nerd Font Mono, monospace", Size: 13}

// 终端的 16 色，取自 GitHub 的 VS Code 主题：黑红绿黄蓝品青白，再是它们的亮色。
var (
	lightANSI = [16]ui.Color{
		ui.Hex("#24292f"), ui.Hex("#cf222e"), ui.Hex("#116329"), ui.Hex("#4d2d00"),
		ui.Hex("#0969da"), ui.Hex("#8250df"), ui.Hex("#1b7c83"), ui.Hex("#6e7781"),
		ui.Hex("#57606a"), ui.Hex("#a40e26"), ui.Hex("#1a7f37"), ui.Hex("#633c01"),
		ui.Hex("#218bff"), ui.Hex("#a475f9"), ui.Hex("#3192aa"), ui.Hex("#8c959f"),
	}
	darkANSI = [16]ui.Color{
		ui.Hex("#484f58"), ui.Hex("#ff7b72"), ui.Hex("#3fb950"), ui.Hex("#d29922"),
		ui.Hex("#58a6ff"), ui.Hex("#bc8cff"), ui.Hex("#39c5cf"), ui.Hex("#b1bac4"),
		ui.Hex("#6e7681"), ui.Hex("#ffa198"), ui.Hex("#56d364"), ui.Hex("#e3b341"),
		ui.Hex("#79c0ff"), ui.Hex("#d2a8ff"), ui.Hex("#56d4dd"), ui.Hex("#f0f6fc"),
	}
)

// termTheme 用 Atlas 的前景、面板底色与选区色，加上给定的 16 色组成终端配色。
func termTheme(p palette, ansi [16]ui.Color) *terminal.Theme {
	return &terminal.Theme{
		Foreground: p.text,
		Background: p.canvas,
		Selection:  p.accent.Alpha(0.25),
		Palette:    ansi,
	}
}

// closeTerm 关闭第 i 个标签的终端；最后一个关闭后收起面板。
func (ch *chat) closeTerm(i int) {
	ch.terms[i].term.Close()
	ch.terms = slices.Delete(ch.terms, i, i+1)
	if i < ch.termTab {
		ch.termTab--
	}
	ch.fitTab()
}

// pruneTerms 移除已经退出的终端，比如在里面执行了 exit。
func (ch *chat) pruneTerms() {
	ch.terms = slices.DeleteFunc(ch.terms, func(sh *shell) bool {
		select {
		case <-sh.term.Done():
			sh.term.Close()
			return true
		default:
			return false
		}
	})
	ch.fitTab()
}

// fitTab 让当前标签落在有效范围内；没有终端时收起面板。
func (ch *chat) fitTab() {
	ch.termTab = max(min(ch.termTab, len(ch.terms)-1), 0)
	if len(ch.terms) == 0 {
		ch.termOpen = false
	}
}

// closeTerms 关闭会话的全部终端，在会话被删除、丢弃或应用退出时调用。
func (ch *chat) closeTerms() {
	for _, sh := range ch.terms {
		sh.term.Close()
	}
	ch.terms, ch.termTab, ch.termOpen = nil, 0, false
}

// leave 在离开一个尚未创建的空白会话前关闭它的终端：之后再也回不到它。
func (a *App) leave() {
	if ch := a.current; ch != nil && ch.id == "" && !ch.running {
		ch.closeTerms()
	}
}

// closeAllTerminals 在应用退出时关闭所有会话的终端。
func (a *App) closeAllTerminals() {
	for _, ch := range a.chats {
		ch.closeTerms()
	}
	a.current.closeTerms()
}

// terminalButton 是标题栏右端的终端开关；面板打开时用强调色标出。
func (a *App) terminalButton(c *ui.Context, t *ui.Theme) {
	const inset = 12
	open := a.current.termOpen
	top := (titleBarHeight(c) - barButton) / 2
	// 绝对定位在标题栏的右端，不影响标题与阅读栏的对齐。
	btn := ui.ButtonBase(c).Absolute().Top(top).Right(inset+c.TitleBar().Right).
		Size(barButton, barButton).Radius(controlRound).Cursor(ui.CursorPointer).
		Label("终端").Tooltip("终端")
	color := t.TextMuted
	switch {
	case open:
		btn.Background(t.Accent.Alpha(0.15))
		color = t.Accent
	case btn.Hovered():
		btn.Background(t.Text.Alpha(0.08))
	}
	btn.OnClick(a.toggleTerminal)
	btn.Children(func() { ui.Icon(c, terminalIcon).FontSize(16).TextColor(color) })
}

// termHandle 是对话区与终端卡片之间的拖动条：不可见，悬停时显示左右拖动的光标，
// 左右拖动调整卡片宽度。
func (a *App) termHandle(c *ui.Context) {
	handle := ui.Box(c).Width(handleWidth).Shrink(0).Cursor(ui.CursorResizeEW)
	if dx, _, ok := handle.Dragged(); ok {
		a.termWidth -= dx
	}
}

// terminalCard 是右侧浮动的终端卡片：圆角边框，离窗口边缘留出 edge 的空隙；
// 顶部是标签栏，其下是当前标签的终端。
func (a *App) terminalCard(c *ui.Context, t *ui.Theme, ch *chat) {
	// 窗口控件在右上角的平台上，卡片从标题栏下方开始。
	top := float32(edge)
	if c.TitleBar().Right > 0 {
		top = titleBarHeight(c)
	}
	card := ui.Column(c.Key("term-card")).Width(a.termWidth).Shrink(0).Margin(top, edge, edge, 0).
		Radius(cardRound).Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.06)).Clip() // 终端是方角的，裁到卡片的圆角里
	card.Children(func() {
		a.termTabs(c, t, ch)
		switch {
		case len(ch.terms) > 0:
			sh := ch.terms[ch.termTab]
			terminal.View(c.Key("term-"+strconv.Itoa(sh.id)), sh.term).Bind(&sh.focus).
				Grow(1).Padding(8, 12, 12)
		case ch.termErr != "":
			ui.Text(c, ch.termErr).TextColor(t.Danger).FontSize(13).Selectable().Padding(12)
		}
	})
}

// termTabs 画卡片顶部的标签栏：各个终端、新建，以及右端的收起按钮。
func (a *App) termTabs(c *ui.Context, t *ui.Theme, ch *chat) {
	// 卡片上沿离窗口 edge，边框占 1：标签栏高度取标题栏减去上下各 edge 和边框，
	// 这样标签与标题栏里的按钮、交通灯在同一条水平线上。
	ui.Row(c).Height(titleBarHeight(c)-2*edge-2).AlignItems(ui.Center).Gap(2).
		Padding(0, tabInset).Children(func() {
		// 标签放在可横向滚动的行里：太多放不下时滚动，新建与收起按钮始终可见。
		ui.ScrollHorizontal(c).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Center).
			Children(func() {
				for i, sh := range ch.terms {
					a.termTab(c, t, ch, i, sh)
				}
			})
		iconButton(c, t, plusIcon, "新建终端", barButton, func() { a.newTerminal(ch) })
		ui.Row(c).Grow(1)
		iconButton(c, t, closeIcon, "收起终端", barButton, a.toggleTerminal)
	})
}

// termTab 画一个标签：“终端 N”加程序设置的标题，右侧是关闭按钮。
func (a *App) termTab(c *ui.Context, t *ui.Theme, ch *chat, i int, sh *shell) {
	label := "终端 " + strconv.Itoa(i+1)
	active := i == ch.termTab
	row := ui.Row(c.Key("tab-"+strconv.Itoa(sh.id))).Height(tabHeight).AlignItems(ui.Center).
		Gap(4).Shrink(0).Radius(controlRound).Padding(0, 4, 0, 8)
	// 只有一个标签时它无从区分，不加底色。
	if active && len(ch.terms) > 1 {
		row.Background(t.Text.Alpha(0.08))
	}
	// 新建或切换到的标签滚进可见范围；只在那一帧请求，之后用户可以自由滚走。
	if ch.termReveal == sh.id {
		row.ScrollIntoView()
		a.win.Update(func() { ch.termReveal = 0 })
	}
	row.Children(func() {
		pick := ui.ButtonBase(c).Gap(6).Cursor(ui.CursorPointer).Label(label)
		pick.OnClick(func() {
			ch.termTab, ch.termReveal = i, sh.id
			sh.focus.Focus()
		})
		pick.Children(func() {
			ui.Text(c, label).FontSize(13).SingleLine().TextColor(t.TextMuted)
			if title := sh.term.Title(); title != "" {
				ui.Text(c, truncate(title, 16)).FontSize(13).TextColor(t.TextMuted).SingleLine()
			}
		})
		iconButton(c, t, closeIcon, "关闭终端", 18, func() { ch.closeTerm(i) })
	})
}

// iconButton 画一个只有图标的方形小按钮，图标取边长的 0.6。
func iconButton(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, size float32, click func(),
) {
	btn := ui.ButtonBase(c).Size(size, size).Radius(controlRound).Cursor(ui.CursorPointer).
		Label(label).Tooltip(label)
	if btn.Hovered() {
		btn.Background(t.Text.Alpha(0.08))
	}
	btn.OnClick(click)
	btn.Children(func() { ui.Icon(c, glyph).FontSize(size * 0.6).TextColor(t.TextMuted) })
}
