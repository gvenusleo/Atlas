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

// toggleTerminal 显示或收起当前会话的终端面板；首次打开时自动开一个终端。文件面板被扩展时，
// 终端已经开着就只还原，没开就还原后打开。
func (a *App) toggleTerminal() {
	ch := a.current
	if ch.restoreFor(termPanel) {
		return
	}
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

// closePanels 关闭会话的终端，并停止文件面板的监视，在会话被删除、丢弃或应用退出时调用。
func (ch *chat) closePanels() {
	ch.closeTerms()
	if ch.files != nil {
		ch.files.stopWatch()
	}
}

// leave 在离开一个还没有保存的空白会话前关闭它的终端与文件面板：之后再也回不到它。文件面板里
// 有没保存的修改时不离开，提示一声并返回 false。
func (a *App) leave() bool {
	ch := a.current
	if ch == nil || !ch.fresh() {
		return true
	}
	if n := ch.dirtyTabs(); n > 0 {
		a.toast = fmt.Sprintf("文件面板里有 %d 个未保存的修改，先保存或关闭它们。", n)
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
