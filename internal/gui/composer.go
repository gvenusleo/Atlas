package gui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 输入区的尺寸，单位为 DIP。
const (
	composerPad    = 6  // 输入框卡片的内边距
	composerButton = 30 // 输入框两侧圆形按钮的直径
	thumbSize      = 56 // 待发送图片的缩略图的边长
	removeSize     = 20 // 缩略图右上角移除按钮的直径
	runBarHeight   = 18 // 运行状态一行的高度
	runGap         = 2  // 运行状态上下的留白
	runDotsSize    = 14 // 其中跳动圆点的高度
	// composerInset 是输入框里左侧按钮的图标离卡片左缘的距离，上方的缩略图与运行状态与它对齐。
	composerInset = composerPad + (composerButton-iconSize)/2
)

// composer 把待发队列和输入框接在一起，下面留出状态栏的间距。
func (a *App) composer(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Column(c.Key("composer-stack")).Children(func() {
			a.promptQueuePanel(c, t, ch)
			card, input := a.composerInput(c, t, ch)
			a.completionMenu(c, t, ch, card, input)
		})
		a.statusRow(c, t, ch)
	})
}

func (a *App) composerInput(c *ui.Context, t *ui.Theme, ch *chat) (ui.Element, ui.Element) {
	locked := ch.queueEditing != nil && ch.queueEditing.saving
	card := ui.Column(c.Key("composer-card")).Radius(inputRound).Background(t.Background).
		Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.06))
	border := fieldBorder(t)
	if !locked {
		if files := card.DroppedFiles(); len(files) > 0 {
			a.dropFiles(ch, files)
		}
		if card.FileDragOver() {
			border = t.Accent
		}
	}
	card.Border(1, border)
	var input ui.Element
	card.Children(func() {
		a.queueEditActions(c, t, ch)
		a.imageStrip(c, t, ch, locked)
		ui.Row(c.Key("composer-row")).Gap(6).AlignItems(ui.Center).
			Padding(composerPad).Children(func() {
			a.imageButton(c, t, ch, locked)
			input = a.composerText(c, ch, locked)
			a.sendButton(c, t, ch)
		})
	})
	return card, input
}

// composerText 让队列编辑与普通草稿各自使用 MyGo 的撤销历史。
func (a *App) composerText(c *ui.Context, ch *chat, locked bool) ui.Element {
	draftKey := "input-" + ch.key
	var input ui.Element
	if edit := ch.queueEditing; edit != nil {
		// 普通输入框在编辑期间仍需参与构建，MyGo 才会保留它的撤销历史。
		ui.TextAreaBase(c.Key(draftKey), &edit.original.input).Hidden(true)
		input = ui.TextAreaBase(c.Key(edit), &ch.input)
	} else {
		input = ui.TextAreaBase(c.Key(draftKey), &ch.input)
	}
	input.FocusRing(false).
		Placeholder(composerHint(ch)).Lines(1, 10).Grow(1).Padding(5, 0).
		Disabled(ch.loading).ReadOnly(locked).
		OnPaste(func(text string) bool { return !locked && a.pasteImages(ch, text) }).
		OnTextKey(func(k ui.TextKey) bool { return a.composerKey(ch, k) })
	ch.menu.placeCaret(input)
	switch {
	case ch.focusInput && !ch.loading:
		input.Focus()
		ch.focusInput = false
	case !a.keyNav && !ch.panelOpen(termPanel):
		input.AutoFocus()
	}
	return input
}

func (a *App) composerKey(ch *chat, k ui.TextKey) bool {
	if a.completionKey(ch, k) {
		return true
	}
	if edit := ch.queueEditing; edit != nil {
		switch {
		case k.Key == ui.KeyEscape && k.Mods == 0:
			if !edit.saving && ch.preparing == 0 {
				ch.closeQueueEdit()
			}
			return true
		case k.Key == ui.KeyEnter && (k.Mods == 0 || k.Mods == ui.Cmd):
			a.saveQueueEdit(ch, k.Text())
			return true
		default:
			return false
		}
	}
	if k.Key != ui.KeyEnter || k.Mods != 0 && k.Mods != ui.Cmd {
		return false
	}
	if a.send(ch, k.Text(), k.Mods == ui.Cmd) {
		k.SetText("")
	}
	return true
}

func composerHint(ch *chat) string {
	switch {
	case ch.queueEditing != nil:
		return "编辑待发消息"
	case !ch.run.Running:
		return "发送消息"
	case ch.run.Kind == protocol.RunCompaction:
		return "⌘Enter 下一轮发送"
	default:
		return "Enter 插入本轮 · ⌘Enter 下一轮发送"
	}
}

// drawRunStatus 在消息列表外保留固定高度，避免运行开始或结束时列表内容跳动。
// 时间每秒变一次，所以在下一个整秒再画一帧。
func (a *App) drawRunStatus(c *ui.Context, t *ui.Theme, ch *chat) {
	row := ui.Row(c.Key("run-status")).Height(runBarHeight).MarginY(runGap).
		AlignItems(ui.Center).Gap(8).Padding(0, 0, 0, composerInset)
	if !ch.run.Running {
		return
	}
	d := time.Since(ch.run.Since)
	c.After(time.Second - d%time.Second)
	row.Children(func() {
		dots := loading.Options{Size: runDotsSize, Color: t.TextMuted}
		// 圆点在它的盒子里偏上，往下挪一点，才与文字居中。
		loading.BouncingDots(c, dots).Label("运行中").Margin(3, 0, 0)
		ui.Text(c, runTime(d)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
	})
}

// runTime 把运行时长说成“12 秒”“3 分 05 秒”“1 小时 02 分”。
func runTime(d time.Duration) string {
	switch s := int(d.Seconds()); {
	case s < 60:
		return fmt.Sprintf("%d 秒", s)
	case s < 3600:
		return fmt.Sprintf("%d 分 %02d 秒", s/60, s%60)
	default:
		return fmt.Sprintf("%d 小时 %02d 分", s/3600, s%3600/60)
	}
}

// sendButton 在运行时保留独立的停止按钮，输入框有内容时也可通过鼠标发送。
func (a *App) sendButton(c *ui.Context, t *ui.Theme, ch *chat) {
	if ch.queueEditing == nil && (!ch.run.Running || ch.hasDraft()) {
		label := "发送"
		if ch.run.Running {
			label = "插入本轮"
			if ch.run.Kind == protocol.RunCompaction {
				label = "下一轮发送"
			}
		}
		composerAction(c.Key("send"), t, sendIcon, label, !ch.hasDraft() || !ch.canSend(), func() {
			if a.send(ch, ch.input, ch.run.Kind == protocol.RunCompaction) {
				ch.input = ""
			}
		})
	}
	if ch.run.Running {
		req := protocol.CancelTurnRequest{SessionID: ch.id, RunID: ch.run.RunID}
		composerAction(c.Key("stop"), t, stopIcon, "停止", false, func() {
			go func() { _ = a.service.CancelTurn(context.Background(), req) }()
		})
	}
}

func composerAction(
	c *ui.Context, t *ui.Theme, glyph *ui.SVG, label string, disabled bool, click func(),
) {
	btn := ui.ButtonBase(c).Disabled(disabled).OnClick(click)
	if !disabled {
		alpha := float32(0.15)
		if btn.Hovered() {
			alpha = 0.25
		}
		btn.Background(accentFg(t).Alpha(alpha)).TextColor(accentFg(t))
	}
	btn.Size(composerButton, composerButton).Padding(0).Radius(composerButton / 2).
		AlignSelf(ui.End).Label(label).Tooltip(label)
	btn.Children(func() { ui.Icon(c, glyph).FontSize(iconSize) })
}

// imageButton 是输入框左侧的按钮，点一下选图片。当前模型不接受图片时变淡，点了提示原因。
func (a *App) imageButton(c *ui.Context, t *ui.Theme, ch *chat, locked bool) {
	can := ch.acceptsImages()
	label, color := "添加图片", t.TextMuted
	if !can {
		label, color = "当前模型不接受图片", t.TextMuted.Alpha(0.4)
	}
	btn := ui.ButtonBase(c).Size(composerButton, composerButton).Padding(0).
		Radius(composerButton / 2).AlignSelf(ui.End).Label("添加图片").Tooltip(label).Disabled(locked)
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
func (a *App) imageStrip(c *ui.Context, t *ui.Theme, ch *chat, locked bool) {
	if len(ch.attachments) == 0 && ch.preparing == 0 {
		return
	}
	ui.Row(c).Wrap().Gap(8).Padding(10, 10, 0, composerInset).Children(func() {
		for _, f := range ch.attachments {
			a.thumbnail(c, t, ch, f, locked)
		}
		for range ch.preparing {
			ui.Row(c).Size(thumbSize, thumbSize).Justify(ui.Center).AlignItems(ui.Center).
				Radius(boxRound).Background(rowFill(t)).Children(func() {
				loading.Ring(c, loading.Options{Size: iconSize})
			})
		}
	})
}

// thumbnail 画一张待发送图片的缩略图；指针在其上时，右上角出现移除按钮。
func (a *App) thumbnail(c *ui.Context, t *ui.Theme, ch *chat, f *attachment, locked bool) {
	tile := ui.Column(c.Key(f.id)).Size(thumbSize, thumbSize).Radius(boxRound).
		Background(rowFill(t)).Tooltip(f.name)
	hovered := tile.Hovered()
	tile.Children(func() {
		if f.bitmap != nil {
			ui.Image(c, f.bitmap).Size(thumbSize, thumbSize).Fit(ui.Cover).Radius(boxRound)
		}
		if !hovered || locked {
			return
		}
		remove := ui.ButtonBase(c).Absolute().Top(4).Right(4).Size(removeSize, removeSize).
			Padding(0).Radius(removeSize / 2).Background(ui.RGBA(0, 0, 0, 0.6)).Label("移除")
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
