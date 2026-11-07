package gui

import (
	"fmt"
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
	a.togglePanel(termPanel, func() {
		if len(ch.terms) == 0 {
			a.newTerminal(ch)
		}
	}, nil)
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
		Theme:     termTheme(activeScheme.light, activeScheme.lightANSI),
		DarkTheme: termTheme(activeScheme.dark, activeScheme.darkANSI),
		// 配色里的 ANSI 黑与白常与底色相同或相近，程序用它们写字会看不见。
		MinimumContrast: termContrast,
		// 终端随拖动条频繁改变大小；zsh 重画提示符前先清掉旧的，不留下错位的残行。
		ShellIntegration: true,
		OnTitle:          func(string) { a.win.Invalidate() },
		OnExit:           func(int) { a.win.Update(ch.pruneTerms) },
	})
	if err != nil {
		ch.termErr = err.Error()
		return
	}
	sh := &shell{id: a.shells, term: term}
	ch.termErr = ""
	ch.terms = append(ch.terms, sh)
	ch.termTab, ch.termReveal = len(ch.terms)-1, sh.id
	ch.focusTerm()
}

// focusTerm 把键盘焦点交给当前标签的终端；没有终端时什么也不做。
func (ch *chat) focusTerm() {
	if len(ch.terms) > 0 {
		ch.terms[ch.termTab].focus.Focus()
	}
}

// termFont 是终端的字体。
var termFont = terminal.Font{Family: monoFamily, Size: codeSize}

// termContrast 是终端文字与底色的最小对比度（WCAG 比值）；够用即止，不改程序刻意选的色相。
const termContrast = 3

// termTheme 用配色的前景、面板底色与选区色，加上给定的 16 色组成终端配色。
// 光标用前景色，搜索高亮用配色的黄与强调色；SelectionText 留零，
// 选中的文字就保留程序自己的颜色。
func termTheme(p palette, ansi [16]ui.Color) *terminal.Theme {
	return &terminal.Theme{
		Foreground:         p.text,
		Background:         p.canvas,
		Cursor:             p.text,
		CursorText:         p.canvas,
		Selection:          p.accent.Alpha(0.25),
		Search:             p.running.Alpha(0.35),
		SearchText:         p.text,
		SearchSelected:     p.accent.Alpha(0.55),
		SearchSelectedText: p.text,
		Palette:            ansi,
	}
}

// rethemeTerminals 把已打开的终端换成当前配色的颜色；字体不跟着变，新开的终端才用新字体。
func (a *App) rethemeTerminals() {
	light := termTheme(activeScheme.light, activeScheme.lightANSI)
	dark := termTheme(activeScheme.dark, activeScheme.darkANSI)
	for _, ch := range a.chats {
		for _, sh := range ch.terms {
			sh.term.SetTheme(light, dark)
		}
	}
	if a.chats[a.current.id] != a.current { // 还没创建的新会话不在 a.chats 里
		for _, sh := range a.current.terms {
			sh.term.SetTheme(light, dark)
		}
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
		ch.closePanel(termPanel)
	}
}

// closeTerms 关闭会话的全部终端，在会话被删除、丢弃或应用退出时调用。
func (ch *chat) closeTerms() {
	for _, sh := range ch.terms {
		sh.term.Close()
	}
	ch.terms, ch.termTab = nil, 0
	ch.closePanel(termPanel)
}

// closePanels 关闭会话的终端，并停止监视工作目录与 git 目录，在会话被删除、丢弃或应用退出时
// 调用。
func (ch *chat) closePanels() {
	ch.starts++ // 关闭后不再接受在途会话或分支结果
	ch.closeTerms()
	ch.stopWatch()
	ch.stopWatchHead()
}

// leave 在离开一个还没有保存的空白会话前关闭它的终端与文件面板：之后再也回不到它。文件面板里
// 有没保存的修改时不离开，提示一声并返回 false。
func (a *App) leave() bool {
	ch := a.current
	if ch == nil || !ch.fresh() {
		return true
	}
	if n := ch.dirtyTabs(); n > 0 {
		a.toast = fmt.Sprintf("文件面板里有 %d 个未保存的修改，先保存或关闭它们", n)
		return false
	}
	ch.closePanels()
	if ch.id != "" {
		a.discard(ch)
	}
	return true
}

// closeAllPanels 在应用退出时关闭所有会话的终端，并停止文件面板的监视。
func (a *App) closeAllPanels() {
	for _, ch := range a.chats {
		ch.closePanels()
	}
	a.current.closePanels()
}

// terminalCard 是右侧栏里的终端卡片：顶部是标签栏，其下是当前标签的终端。
func (a *App) terminalCard(c *ui.Context, t *ui.Theme, ch *chat, height float32) {
	sideCard(c, t, "term-card", height).Children(func() {
		a.termTabs(c, t, ch)
		switch {
		case len(ch.terms) > 0:
			sh := ch.terms[ch.termTab]
			terminal.View(c.Key("term-"+strconv.Itoa(sh.id)), sh.term).Bind(&sh.focus).
				Grow(1).Padding(8, 12, 12)
		case ch.termErr != "":
			ui.Text(c, ch.termErr).TextColor(t.Danger).FontSize(textSize).Selectable().Padding(12)
		}
	})
}

// termTabs 画卡片顶部的标签栏：各个终端、新建，以及右端的收起按钮。标签太多放不下时
// 滚动，新建与收起按钮始终可见。
func (a *App) termTabs(c *ui.Context, t *ui.Theme, ch *chat) {
	items := make([]tabItem, len(ch.terms))
	for i, sh := range ch.terms {
		items[i] = tabItem{
			id: sh.id, label: "终端 " + strconv.Itoa(i+1), note: sh.term.Title(),
			closeLabel: "关闭终端",
			pick: func() {
				ch.termTab, ch.termReveal = i, sh.id
				ch.focusTerm()
			},
			close: func() { ch.closeTerm(i) },
		}
	}
	ui.Row(c).Height(tabBarHeight(c)).AlignItems(ui.Center).Gap(2).Padding(0, tabInset).
		DragWindow().Children(func() {
		a.tabStrip(c, t, "term", items, ch.termTab, &ch.termReveal)
		iconButton(c, t, plusIcon, "新建终端", barButton, func() { a.newTerminal(ch) })
		ui.Row(c).Grow(1)
		a.cardActions(c, t, ch, termPanel, "收起终端", a.toggleTerminal)
	})
}
