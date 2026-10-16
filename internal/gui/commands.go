package gui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// compactCommand 是输入框里手动压缩上下文的命令。
const compactCommand = "/compact"

// command 是输入框里以 / 开头的命令。
type command struct {
	name, hint string
}

// commands 是可用的命令；在输入框里输入 / 时列出。
var commands = []command{
	{compactCommand, "压缩上下文：把较早的对话换成摘要"},
}

// matches 返回 text 正在输入的命令：以 / 开头、还没有空白，且是命令名的前缀。
// 运行中、加载中或被 Esc 关掉时一个也没有。
func (ch *chat) matches(text string) []command {
	if ch.running || ch.loading || text == ch.menuClosed ||
		!strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t\r\n") {
		return nil
	}
	var found []command
	for _, cmd := range commands {
		if strings.HasPrefix(cmd.name, strings.ToLower(text)) {
			found = append(found, cmd)
		}
	}
	return found
}

// commandKey 处理命令列表显示时的按键：Enter 执行、Tab 补全第一个匹配的命令，Esc 关闭；
// 返回是否已处理，没处理的按键照常输入。
func (a *App) commandKey(ch *chat, k ui.TextKey) bool {
	text := k.Text()
	cmds := ch.matches(text)
	if len(cmds) == 0 || k.Mods != 0 {
		return false
	}
	switch k.Key {
	case ui.KeyTab:
		k.SetText(cmds[0].name)
	case ui.KeyEscape:
		ch.menuClosed = text
	case ui.KeyEnter:
		if a.send(ch, cmds[0].name) {
			k.SetText("")
		}
	default:
		return false
	}
	return true
}

// commandMenu 在输入框上方列出可用的命令。选项不抢走输入框的焦点，所以点选之后
// 还能接着输入。
func (a *App) commandMenu(c *ui.Context, t *ui.Theme, ch *chat, card, input ui.Element) {
	// 文本离开了被 Esc 关掉时的样子，就不再算关掉：再输入同样的内容要重新列出。
	if ch.input != ch.menuClosed {
		ch.menuClosed = ""
	}
	cmds := ch.matches(ch.input)
	if len(cmds) == 0 || !input.Focused() {
		return
	}
	ui.Overlay(c, func() {
		panel := ui.Column(c).MinWidth(card.Bounds().W).
			AttachTo(card, ui.AnchorTopStart, ui.AnchorBottomStart).KeepFocus()
		// 上下边距之差是它与输入框的间距：贴着输入框上沿时，下边距越大离得越远。
		popupPanel(panel, t).Margin(0, 0, 6, 0)
		panel.Children(func() {
			for i, cmd := range cmds {
				item := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(10).
					Padding(5, 10).Radius(controlRound).Cursor(ui.CursorPointer).Label(cmd.name)
				// 第一行是 Enter 与 Tab 作用的命令，所以一直高亮。
				if i == 0 || item.Hovered() {
					item.Background(t.SurfaceHover)
				}
				item.OnClick(func() {
					if a.send(ch, cmd.name) {
						ch.input = ""
					}
				})
				item.Children(func() {
					ui.Text(c, cmd.name).FontSize(13).SingleLine()
					ui.Text(c, cmd.hint).FontSize(12).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
	})
}
