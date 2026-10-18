package gui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/gvenusleo/atlas/internal/host"
)

// compactCommand 是输入框里手动压缩上下文的命令。
const compactCommand = "/compact"

// 命令列表的尺寸：menuHeight 是最大高度，命令多时在里面滚动；menuRows 是它大约能放下的
// 行数；menuNameSize 是命令名的字号，量宽度与绘制都用它。
const (
	menuHeight   = 240
	menuRows     = 10
	menuNameSize = 13
)

// command 是输入框里以 / 开头的命令：内置命令，或会话的技能。
type command struct {
	name, hint string
	skill      bool // 技能：补全后接着输入参数，不直接执行
}

// menuState 是输入框上方命令列表的状态。
type menuState struct {
	closed     string     // 被 Esc 关掉时输入框里的文本；文本变了就重新列出
	text       string     // index 对应的输入框文本；文本变了回到第一行
	index      int        // 选中的行
	moved      bool       // 按键移动了选中行，下一帧要滚到可见
	nameWidth  float32    // 上一帧视野内最长的命令名的宽度，没有时为 0
	pointer    [2]float32 // 指针上一次在窗口里的位置，还没见过时是 noPointer
	caretToEnd bool       // 补全了命令，下一帧把输入框的光标放到末尾
}

// follow 让选中的行跟着输入框的文本：文本变了就回到第一行。
func (m *menuState) follow(text string) {
	if text != m.text {
		m.text, m.index = text, 0
	}
}

// row 返回共 n 行的列表里选中的行。
func (m *menuState) row(n int) int { return min(m.index, n-1) }

// noPointer 是还没见过指针的位置：列表刚出现时，指针可能正停在某一行上，不算它动过。
var noPointer = [2]float32{-1, -1}

// pointerTo 记下指针在窗口里的位置 at，它正在第 i 行上：指针动了才选中这一行。
// 指针没动、只是列表在它底下滚动换了行时，不抢走键盘选中的行。
func (m *menuState) pointerTo(at [2]float32, i int) {
	if at == m.pointer {
		return
	}
	if m.pointer != noPointer {
		m.index = i
	}
	m.pointer = at
}

// complete 把命令补全到输入框，返回此后输入框里的文本：技能后面留一个空格，等着输入参数；
// 补全之后光标要到末尾，换了文本，光标还留在原处。
func (m *menuState) complete(cmd command) string {
	m.caretToEnd = true
	if cmd.skill {
		return cmd.name + " "
	}
	return cmd.name
}

// placeCaret 在补全之后的下一帧，把输入框的光标放到末尾；越界的偏移会被夹到文本内。
func (m *menuState) placeCaret(input ui.Element, text string) {
	if m.caretToEnd {
		input.SetTextSelection(len(text), len(text))
		m.caretToEnd = false
	}
}

// commandList 是可用的命令：内置命令在前，其后是会话的技能。在输入框里输入 / 时列出。
func (ch *chat) commandList() []command {
	list := []command{{name: compactCommand, hint: "把较早的对话压缩成摘要"}}
	for _, s := range ch.skillList() {
		list = append(list, command{name: "/" + s.Name, hint: s.Description, skill: true})
	}
	return list
}

// matches 返回 text 正在输入的命令：以 / 开头、还没有空白，且是命令名的前缀。
// 运行中、加载中或被 Esc 关掉时一个也没有。
func (ch *chat) matches(text string) []command {
	if ch.running || ch.loading || text == ch.menu.closed ||
		!strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t\r\n") {
		return nil
	}
	var found []command
	for _, cmd := range ch.commandList() {
		if strings.HasPrefix(cmd.name, strings.ToLower(text)) {
			found = append(found, cmd)
		}
	}
	return found
}

// loadSkills 在后台发现 ch 的工作目录下的技能，供还没有会话时的命令列表使用；
// 每个工作目录只发现一次，出错时当作没有。
func (a *App) loadSkills(ch *chat) {
	cwd := ch.cwd
	ch.skills, ch.skillsCwd = nil, cwd
	go func() {
		list, _ := host.DiscoverSkills(cwd)
		a.win.Update(func() {
			if ch.skillsCwd == cwd { // 发现期间换了工作目录的，结果作废
				ch.skills = list
			}
		})
	}()
}

// activate 选中一个命令，返回此后输入框里的文本：内置命令直接执行，输入框清空；
// 技能补全成“/名称 ”，等着输入参数。
func (a *App) activate(ch *chat, cmd command) string {
	if !cmd.skill && a.send(ch, cmd.name) {
		return ""
	}
	return ch.menu.complete(cmd)
}

// commandKey 处理命令列表显示时的按键：↑↓ 移动选中的行，Enter 选中、Tab 补全选中的命令，
// Esc 关闭；返回是否已处理，没处理的按键照常输入。
func (a *App) commandKey(ch *chat, k ui.TextKey) bool {
	text := k.Text()
	cmds := ch.matches(text)
	if len(cmds) == 0 || k.Mods != 0 {
		return false
	}
	m := &ch.menu
	m.follow(text)
	i, n := m.row(len(cmds)), len(cmds)
	switch k.Key {
	case ui.KeyUp:
		m.index, m.moved = (i+n-1)%n, true
	case ui.KeyDown:
		m.index, m.moved = (i+1)%n, true
	case ui.KeyTab:
		k.SetText(m.complete(cmds[i]))
	case ui.KeyEscape:
		m.closed = text
	case ui.KeyEnter:
		k.SetText(a.activate(ch, cmds[i]))
	default:
		return false
	}
	return true
}

// commandMenu 在输入框上方列出可用的命令。选项不抢走输入框的焦点，所以点选之后
// 还能接着输入。
func (a *App) commandMenu(c *ui.Context, t *ui.Theme, ch *chat, card, input ui.Element) {
	m := &ch.menu
	// 文本离开了被 Esc 关掉时的样子，就不再算关掉：再输入同样的内容要重新列出。
	if ch.input != m.closed {
		m.closed = ""
	}
	if strings.HasPrefix(ch.input, "/") && ch.session == nil && ch.skillsCwd != ch.cwd {
		a.loadSkills(ch)
	}
	cmds := ch.matches(ch.input)
	if len(cmds) == 0 || !input.Focused() {
		m.pointer = noPointer
		return
	}
	m.follow(ch.input)
	// 命令名后面的内容在视野内左对齐：名称一列的宽度取上一帧视野内最长的命令名；
	// 还没有上一帧时，取列表顶端一屏的。
	nameW := m.nameWidth
	if nameW == 0 {
		for _, cmd := range cmds[:min(len(cmds), menuRows)] {
			nameW = max(nameW, measureName(c, cmd.name))
		}
	}
	var rows []ui.Element
	var visibleW float32
	ui.Overlay(c, func() {
		panel := ui.Column(c).Width(card.Bounds().W).
			AttachTo(card, ui.AnchorTopStart, ui.AnchorBottomStart).KeepFocus()
		// 上下边距之差是它与输入框的间距：贴着输入框上沿时，下边距越大离得越远。
		popupPanel(panel, t).Margin(0, 0, 6, 0)
		panel.Children(func() {
			ui.Scroll(c).MaxHeight(menuHeight).Children(func() {
				for i, cmd := range cmds {
					row := a.commandRow(c, t, ch, cmd, nameW)
					rows = append(rows, row)
					if x, y, over := row.PointerPosition(); over {
						b := row.Bounds()
						m.pointerTo([2]float32{b.X + x, b.Y + y}, i)
					}
					if row.InView() {
						visibleW = max(visibleW, measureName(c, cmd.name))
					}
				}
			})
		})
	})
	// 选中的行是 Enter 与 Tab 作用的命令，所以高亮；键盘与指针选的是同一行，只有一行高亮。
	if selected := m.row(len(cmds)); selected < len(rows) {
		rows[selected].Background(t.SurfaceHover)
		if m.moved {
			rows[selected].ScrollIntoView()
		}
	}
	// 视野变了、名称一列该换宽度时，再画一帧。
	if visibleW != 0 && visibleW != nameW {
		c.After(0)
	}
	m.nameWidth, m.moved = visibleW, false
}

// commandRow 画命令列表里的一行：命令名（占 nameW 宽，后面的内容因此对齐）、技能的标记
// 和描述；点一下就是选中它。
func (a *App) commandRow(
	c *ui.Context, t *ui.Theme, ch *chat, cmd command, nameW float32,
) ui.Element {
	row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(10).
		Padding(5, 10).Radius(controlRound).Cursor(ui.CursorPointer).Label(cmd.name)
	row.OnClick(func() { ch.input = a.activate(ch, cmd) })
	row.Children(func() {
		ui.Text(c, cmd.name).FontSize(menuNameSize).SingleLine().Width(nameW).Shrink(0)
		if cmd.skill {
			ui.Text(c, "[Skill]").FontSize(12).TextColor(t.Accent).SingleLine().Shrink(0)
		}
		// 描述可能分了好几行，列表里只要一行。
		hint := strings.Join(strings.Fields(cmd.hint), " ")
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted).SingleLine().MinWidth(0)
	})
	return row
}

// measureName 是命令名在列表里占的宽度，多留 1 DIP 免得被省略。
func measureName(c *ui.Context, name string) float32 {
	w, _ := c.MeasureText(1e6, ui.Span{Text: name, Size: menuNameSize})
	return w + 1
}
