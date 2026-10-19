package gui

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// 补全列表的尺寸：menuHeight 是最大高度，选项多时在里面滚动；menuRows 是它大约能放下的
// 行数；menuNameMax 是名称一列的最大宽度，过长的文件名省略，不挤掉后面的内容。
const (
	menuHeight  = 240
	menuRows    = 10
	menuNameMax = 280
)

// option 是补全列表里的一项：命令、技能或文件。
type option struct {
	name, hint string
	icon       *ui.SVG // 名称前的图标，没有时为空
	tag        string  // 名称后的标记，如技能的 Skill
	insert     string  // 选中后换进输入框的文本
	run        bool    // 内置命令：选中就执行，不是补全
}

// completion 是输入框里正在补全的词与它的候选。
type completion struct {
	text     string   // 列出候选时输入框里的文本
	from, to int      // text 里会被选中项换掉的部分，rune 偏移
	options  []option // 没有候选时为空
}

// menuState 是输入框上方补全列表的状态。
type menuState struct {
	closed    string                  // 被 Esc 关掉、或刚补全完时输入框里的文本；文本变了就重新列出
	index     int                     // 选中的行；文本变了回到第一行
	moved     bool                    // 按键移动了选中行，下一帧要滚到可见
	nameWidth float32                 // 上一帧视野内最长的名称的宽度，没有时为 0
	pointer   [2]float32              // 指针上一次在窗口里的位置，还没见过时是 noPointer
	caretAt   int                     // 补全之后光标该在的位置，rune 偏移
	caretDue  bool                    // 下一帧要把输入框的光标放到 caretAt
	cur       completion              // 上一帧列出的候选，按键作用于它
	mention   memo[rankKey, []option] // @ 补全按查询排好的候选
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

// apply 把选中的项换进输入框里正在补全的词，返回此后输入框里的文本。换了文本，光标
// 还留在原处，所以记下它该在的位置，下一帧放到换进去的文本后面；这段文本也不再列出。
func (m *menuState) apply(cur completion, opt option) string {
	runes := []rune(cur.text)
	text := string(runes[:cur.from]) + opt.insert + string(runes[cur.to:])
	m.caretAt, m.caretDue = cur.from+utf8.RuneCountInString(opt.insert), true
	m.closed = text
	return text
}

// placeCaret 在补全之后的下一帧，把输入框的光标放到该在的位置。
func (m *menuState) placeCaret(input ui.Element) {
	if m.caretDue {
		input.SetTextSelection(m.caretAt, m.caretAt)
		m.caretDue = false
	}
}

// completionOf 返回输入框里正在补全的词：以 / 开头且还没有空白的整段文本是命令，
// 否则光标处的 @ 词是文件。加载中或被 Esc 关掉时没有。命令列表里的技能与 @ 的路径，
// 都在第一次用到时才在后台取。
func (a *App) completionOf(ch *chat, input ui.Element) completion {
	text := ch.input
	switch {
	case ch.loading || text == ch.menu.closed:
		return completion{}
	case strings.HasPrefix(text, "/") && !strings.ContainsAny(text, " \t\r\n"):
		if ch.session == nil && ch.skillsCwd != ch.cwd {
			a.loadSkills(ch)
		}
		return ch.commandCompletion(text)
	}
	return a.mentionCompletion(ch, input)
}

// activate 选中一个选项，返回此后输入框里的文本：内置命令直接执行，输入框清空；
// 其余换进输入框里正在补全的词。
func (a *App) activate(ch *chat, cur completion, opt option) string {
	if opt.run && a.send(ch, opt.name) {
		return ""
	}
	return ch.menu.apply(cur, opt)
}

// completionKey 处理补全列表显示时的按键：↑↓ 移动选中的行，Enter 选中、Tab 补全选中的项，
// Esc 关闭；返回是否已处理，没处理的按键照常输入。
func (a *App) completionKey(ch *chat, k ui.TextKey) bool {
	m := &ch.menu
	cur, text := m.cur, k.Text()
	if len(cur.options) == 0 || k.Mods != 0 || text != cur.text {
		return false
	}
	i, n := m.row(len(cur.options)), len(cur.options)
	switch k.Key {
	case ui.KeyUp:
		m.index, m.moved = (i+n-1)%n, true
	case ui.KeyDown:
		m.index, m.moved = (i+1)%n, true
	case ui.KeyTab:
		k.SetText(m.apply(cur, cur.options[i]))
	case ui.KeyEscape:
		m.closed = text
	case ui.KeyEnter:
		k.SetText(a.activate(ch, cur, cur.options[i]))
	default:
		return false
	}
	return true
}

// completionMenu 在输入框上方列出补全的候选。选项不抢走输入框的焦点，所以点选之后
// 还能接着输入。
func (a *App) completionMenu(c *ui.Context, t *ui.Theme, ch *chat, card, input ui.Element) {
	m := &ch.menu
	// 文本离开了被 Esc 关掉时的样子，就不再算关掉：再输入同样的内容要重新列出。
	if ch.input != m.closed {
		m.closed = ""
	}
	cur := a.completionOf(ch, input)
	if len(cur.options) == 0 || !input.Focused() {
		m.cur, m.pointer = completion{}, noPointer
		return
	}
	if cur.text != m.cur.text {
		m.index = 0
	}
	m.cur = cur
	// 名称后面的内容在视野内左对齐：名称一列的宽度取上一帧视野内最长的名称；
	// 还没有上一帧时，取列表顶端一屏的。
	nameW := m.nameWidth
	if nameW == 0 {
		for _, opt := range cur.options[:min(len(cur.options), menuRows)] {
			nameW = max(nameW, measureName(c, opt.name))
		}
	}
	rows, visibleW := a.menuPanel(c, t, ch, card, nameW)
	// 选中的行是 Enter 与 Tab 作用的项，所以高亮；键盘与指针选的是同一行，只有一行高亮。
	if selected := m.row(len(cur.options)); selected < len(rows) {
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

// menuPanel 画贴在 card 上沿的补全列表，返回各行，以及视野内最长的名称的宽度。
func (a *App) menuPanel(
	c *ui.Context, t *ui.Theme, ch *chat, card ui.Element, nameW float32,
) ([]ui.Element, float32) {
	m := &ch.menu
	var rows []ui.Element
	var visibleW float32
	ui.Overlay(c, func() {
		panel := ui.Column(c).Width(card.Bounds().W).
			AttachTo(card, ui.AnchorTopStart, ui.AnchorBottomStart).KeepFocus()
		// 上下边距之差是它与输入框的间距：贴着输入框上沿时，下边距越大离得越远。
		popupPanel(panel, t).Margin(0, 0, 6, 0)
		panel.Children(func() {
			ui.Scroll(c).MaxHeight(menuHeight).Children(func() {
				for i, opt := range m.cur.options {
					pick := func() { ch.input = a.activate(ch, m.cur, opt) }
					row := completionRow(c, t, opt, nameW, pick)
					rows = append(rows, row)
					if x, y, over := row.PointerPosition(); over {
						b := row.Bounds()
						m.pointerTo([2]float32{b.X + x, b.Y + y}, i)
					}
					if row.InView() {
						visibleW = max(visibleW, measureName(c, opt.name))
					}
				}
			})
		})
	})
	return rows, visibleW
}

// completionRow 画补全列表里的一行：名称（占 nameW 宽，后面的内容因此对齐）、标记和
// 说明；点一下就调用 pick 选中它。
func completionRow(
	c *ui.Context, t *ui.Theme, opt option, nameW float32, pick func(),
) ui.Element {
	row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(10).
		Padding(5, 10).Radius(controlRound).Cursor(ui.CursorPointer).Label(opt.name)
	row.OnClick(pick)
	row.Children(func() {
		if opt.icon != nil {
			ui.Image(c, opt.icon).Size(smallIconSize, smallIconSize).Shrink(0)
		}
		ui.Text(c, opt.name).FontSize(textSize).SingleLine().Width(nameW).Shrink(0)
		if opt.tag != "" {
			ui.Text(c, "["+opt.tag+"]").FontSize(smallSize).TextColor(t.Accent).
				SingleLine().Shrink(0)
		}
		// 说明可能分了好几行，列表里只要一行。
		hint := strings.Join(strings.Fields(opt.hint), " ")
		ui.Text(c, hint).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().MinWidth(0)
	})
	return row
}

// measureName 是名称在列表里占的宽度，多留 1 DIP 免得被省略。
func measureName(c *ui.Context, name string) float32 {
	w, _ := c.MeasureText(1e6, ui.Span{Text: name, Size: textSize})
	return min(w+1, menuNameMax)
}
