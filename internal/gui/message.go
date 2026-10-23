package gui

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// copiedFor 是点复制后，按钮显示“已复制”的时长。
const copiedFor = 1500 * time.Millisecond

// block 构建一组显示项：用户消息为靠右的浅色块，其余是一次 run 的输出，作为一个整体。
// final 为假表示这次 run 还在进行。
func (a *App) block(c *ui.Context, t *ui.Theme, b block, final bool) {
	if it := b.items[0]; it.kind == itemUser {
		a.userMessage(c, t, it)
		return
	}
	a.reply(c, t, b.items, final)
}

// reply 构建一次 run 的全部输出：助手文字直接排成正文，思考与工具为安静的单行，可点开
// 查看细节。一次 run 里的各段连成一体，只在末尾有一行操作，复制的是所有助手文字。
func (a *App) reply(c *ui.Context, t *ui.Theme, items []*item, final bool) {
	text, at := replyText(items)
	col := ui.Column(c)
	hovered := col.Hovered()
	col.Children(func() {
		ui.Column(c).Gap(10).Children(func() {
			for _, p := range parts(items) {
				ui.Column(c.Key(p[0].id)).Children(func() { a.part(c, t, p) })
			}
		})
		if text != "" {
			a.actions(c, t, items[0], at, text, hovered && final)
		}
	})
}

// part 画一次 run 输出里的一段。
func (a *App) part(c *ui.Context, t *ui.Theme, items []*item) {
	it := items[0]
	switch it.kind {
	case itemAssistant:
		if it.done {
			markdown.View(c, it.text, &a.md)
		} else {
			markdown.Stream(c, it.text, &a.md)
		}
	case itemThinking, itemTool:
		a.stepGroup(c, t, items)
	case itemNotice:
		ui.Text(c, it.text).FontSize(textSize).TextColor(t.TextMuted).Selectable()
	case itemCompaction:
		a.compaction(c, t, it)
	case itemSkill:
		a.skillRow(c, t, it)
	}
}

// compaction 画上下文压缩：进行中是转动的指示器，完成后是一行可展开的说明，展开是摘要。
func (a *App) compaction(c *ui.Context, t *ui.Theme, it *item) {
	if !it.done {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			loading.Ring(c, loading.Options{Size: smallIconSize, Color: t.TextMuted}).Shrink(0)
			ui.Text(c, "正在压缩上下文").FontSize(bodySize).TextColor(t.TextMuted).SingleLine()
		})
		return
	}
	const label = "已压缩较早的对话"
	expandable(c, t, &it.open, label, false, func() {
		ui.Text(c, label).FontSize(bodySize).TextColor(t.TextMuted).SingleLine()
	}, func() {
		panelBody(c, t, false, func() {
			scrolled(c, func() { markdown.View(c, it.text, &a.mdNote) })
		})
	})
}

// skillRow 画显式调用的技能：一行可展开的说明，展开是附给模型的 SKILL.md 全文。
func (a *App) skillRow(c *ui.Context, t *ui.Theme, it *item) {
	expandable(c, t, &it.open, "已载入技能 "+it.skill, false, func() {
		ui.Text(c, "已载入技能").FontSize(bodySize).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, it.skill).FontSize(bodySize).SingleLine()
	}, func() {
		panelBody(c, t, false, func() {
			scrolled(c, func() { markdown.View(c, it.text, &a.mdNote) })
		})
	})
}

// userMessage 把用户消息和它下面的一行操作放在一起：指针在其上时，操作行显示发送时间与
// 复制按钮。有图片时，图片排在文字上方，靠右对齐；只有图片时没有文字块与复制按钮。
func (a *App) userMessage(c *ui.Context, t *ui.Theme, it *item) {
	col := ui.Column(c).AlignItems(ui.End).Gap(6)
	hovered := col.Hovered()
	col.Children(func() {
		if len(it.pictures) > 0 {
			ui.Row(c).Wrap().Gap(8).Justify(ui.End).MaxWidth(bubbleWidth).Children(func() {
				for _, p := range it.pictures {
					a.userPicture(c, t, p)
				}
			})
		}
		if it.text != "" {
			ui.Text(c, it.text).Selectable().FontSize(bodySize).Padding(10, 16).Radius(14).
				Background(t.Text.Alpha(0.06)).MaxWidth(bubbleWidth)
		}
		a.actions(c, t, it, it.at, it.text, hovered && it.done)
	})
}

// actions 画消息下方的一行：时间与复制按钮，复制的是 text，只在 shown 时显示。
// 这一行始终占着高度，显示与否不会让下面的内容跳动。it 记着最近一次点复制的时刻。
func (a *App) actions(
	c *ui.Context, t *ui.Theme, it *item, at time.Time, text string, shown bool,
) {
	ui.Row(c).Height(barButton).AlignItems(ui.Center).Gap(2).Children(func() {
		if !shown {
			return
		}
		if !at.IsZero() {
			ui.Text(c, since(at)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().
				Padding(0, 4, 0, 0).Tooltip(at.Local().Format("2006-01-02 15:04:05"))
		}
		if text == "" {
			return // 没有文字可复制，如只有图片的消息
		}
		glyph, label := copyIcon, "复制"
		if left := copiedFor - time.Since(it.copiedAt); left > 0 {
			glyph, label = checkIcon, "已复制"
			c.After(left) // 到时再画一帧，把图标换回来
		}
		iconButton(c, t, glyph, label, barButton, func() {
			mygo.Clipboard.WriteText(text)
			it.copiedAt = time.Now()
		})
	})
}

// since 把消息的时间说成相对现在的“N 分钟前”。
func since(at time.Time) string {
	if s := ago(at); s != "刚刚" {
		return s + "前"
	}
	return "刚刚"
}

// truncate 截断到最多 n 个字，被截断时以省略号结尾。
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
