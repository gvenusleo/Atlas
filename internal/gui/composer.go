package gui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// composer 构建输入区：圆角输入框，其下是工作目录、模型与上下文占用。
func (a *App) composer(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c).Gap(6).Children(func() {
		card := ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(6, 6, 6, 16).Radius(inputRound).
			Background(t.Background).Border(1, t.Text.Alpha(0.12)).
			Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.06))
		var input ui.Element
		card.Children(func() {
			input = ui.TextAreaBase(c.Key("input-"+ch.key), &ch.input).FocusRing(false).
				Placeholder("发送消息").Lines(1, 10).Grow(1).Padding(5, 0).Disabled(ch.loading).
				OnTextKey(func(k ui.TextKey) bool {
					if a.completionKey(ch, k) {
						return true
					}
					if k.Key != ui.KeyEnter || k.Mods != 0 {
						return false
					}
					if a.send(ch, k.Text()) {
						k.SetText("")
					}
					return true
				})
			ch.menu.placeCaret(input)
			// 侧栏在用键盘、或终端面板打开时，新出现的输入框不抢走焦点。
			if !a.keyNav && !ch.termOpen {
				input.AutoFocus()
			}
			a.sendButton(c, t, ch)
		})
		a.completionMenu(c, t, ch, card, input)
		a.statusRow(c, t, ch)
	})
}

// sendButton 是输入框右侧的圆形按钮：有内容时发送，运行时停止，空闲且为空时为灰色。
// 可用时是浅色的强调底加强调色图标，与终端按钮的选中态一致。
func (a *App) sendButton(c *ui.Context, t *ui.Theme, ch *chat) {
	glyph, label := sendIcon, "发送"
	if ch.running {
		glyph, label = stopIcon, "停止"
	}
	var btn ui.Element
	// 刚发出时会话句柄还没就绪，此时不能停止。
	if ch.loading || ch.running && ch.session == nil ||
		!ch.running && strings.TrimSpace(ch.input) == "" {
		btn = ui.Button(c, "").Disabled(true)
	} else {
		btn = ui.ButtonBase(c).Cursor(ui.CursorPointer).TextColor(t.Accent).
			OnClick(func() {
				switch {
				case ch.running:
					ch.session.Cancel()
				case a.send(ch, ch.input):
					ch.input = ""
				}
			})
		alpha := float32(0.15)
		if btn.Hovered() {
			alpha = 0.25
		}
		btn.Background(t.Accent.Alpha(alpha))
	}
	btn.Size(30, 30).Padding(0).Radius(15).AlignSelf(ui.End).Label(label).Tooltip(label)
	btn.Children(func() {
		ui.Icon(c, glyph).FontSize(iconSize)
	})
}
