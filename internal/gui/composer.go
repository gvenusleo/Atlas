package gui

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// 输入区的尺寸，单位为 DIP。
const (
	composerPad    = 6  // 输入框卡片的内边距
	composerButton = 30 // 输入框两侧圆形按钮的直径
	thumbSize      = 56 // 待发送图片的缩略图的边长
	removeSize     = 20 // 缩略图右上角移除按钮的直径
)

// composer 构建输入区：圆角输入框，其下是工作目录、模型与上下文占用。
func (a *App) composer(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c).Gap(6).Children(func() {
		card := ui.Column(c).Radius(inputRound).Background(t.Background).
			Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.06))
		// 从别的应用拖来的图片可以放在输入框上；拖在上面时边框变成强调色。
		if files := card.DroppedFiles(); len(files) > 0 {
			a.dropFiles(ch, files)
		}
		border := t.Text.Alpha(0.12)
		if card.FileDragOver() {
			border = t.Accent
		}
		card.Border(1, border)
		var input ui.Element
		card.Children(func() {
			a.imageStrip(c, t, ch)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(composerPad).Children(func() {
				a.imageButton(c, t, ch)
				input = ui.TextAreaBase(c.Key("input-"+ch.key), &ch.input).FocusRing(false).
					Placeholder("发送消息").Lines(1, 10).Grow(1).Padding(5, 0).Disabled(ch.loading).
					OnPaste(func(text string) bool { return a.pasteImages(ch, text) }).
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
		!ch.running && (!ch.hasDraft() || ch.preparing > 0) {
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
	btn.Size(composerButton, composerButton).Padding(0).Radius(composerButton / 2).
		AlignSelf(ui.End).Label(label).Tooltip(label)
	btn.Children(func() {
		ui.Icon(c, glyph).FontSize(iconSize)
	})
}

// imageButton 是输入框左侧的按钮，点一下选图片。当前模型不接受图片时变淡，点了提示原因。
func (a *App) imageButton(c *ui.Context, t *ui.Theme, ch *chat) {
	can := a.acceptsImages(ch)
	label, color := "添加图片", t.TextMuted
	if !can {
		label, color = "当前模型不接受图片", t.TextMuted.Alpha(0.4)
	}
	btn := ui.ButtonBase(c).Size(composerButton, composerButton).Padding(0).
		Radius(composerButton / 2).AlignSelf(ui.End).Cursor(ui.CursorPointer).
		Label("添加图片").Tooltip(label)
	if btn.Hovered() {
		btn.Background(buttonFill(t))
	}
	btn.OnClick(func() {
		if can {
			a.pickImages(ch)
		} else {
			a.toast = noImagesHint
		}
	})
	btn.Children(func() {
		ui.Icon(c, plusIcon).FontSize(iconSize).TextColor(color)
	})
}

// imageStrip 画输入框上方待发送图片的缩略图，整理中的图片先是一个转圈的方块；
// 没有图片时什么也不画。左边与下面按钮的图标对齐。
func (a *App) imageStrip(c *ui.Context, t *ui.Theme, ch *chat) {
	if len(ch.attachments) == 0 && ch.preparing == 0 {
		return
	}
	const left = composerPad + (composerButton-iconSize)/2
	ui.Row(c).Wrap().Gap(8).Padding(10, 10, 0, left).Children(func() {
		for _, f := range ch.attachments {
			a.thumbnail(c, t, ch, f)
		}
		for range ch.preparing {
			ui.Row(c).Size(thumbSize, thumbSize).Justify(ui.Center).AlignItems(ui.Center).
				Radius(boxRound).Background(rowFill(t)).Children(func() {
				ui.Spinner(c).Size(iconSize, iconSize)
			})
		}
	})
}

// thumbnail 画一张待发送图片的缩略图；指针在其上时，右上角出现移除按钮。
func (a *App) thumbnail(c *ui.Context, t *ui.Theme, ch *chat, f *attachment) {
	tile := ui.Column(c.Key(f.id)).Size(thumbSize, thumbSize).Radius(boxRound).
		Background(rowFill(t)).Tooltip(f.name)
	hovered := tile.Hovered()
	tile.Children(func() {
		if f.bitmap != nil {
			ui.Image(c, f.bitmap).Size(thumbSize, thumbSize).Fit(ui.Cover).Radius(boxRound)
		}
		if !hovered {
			return
		}
		remove := ui.ButtonBase(c).Absolute().Top(4).Right(4).Size(removeSize, removeSize).
			Padding(0).Radius(removeSize / 2).Background(ui.RGBA(0, 0, 0, 0.6)).
			Cursor(ui.CursorPointer).Label("移除")
		remove.OnClick(func() {
			ch.attachments = slices.DeleteFunc(ch.attachments, func(p *attachment) bool {
				return p == f
			})
		})
		remove.Children(func() {
			ui.Icon(c, closeIcon).FontSize(smallIconSize).TextColor(t.AccentText)
		})
	})
}
