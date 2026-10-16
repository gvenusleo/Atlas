package gui

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// copiedFor 是点复制后，按钮显示“已复制”的时长。
const copiedFor = 1500 * time.Millisecond

// block 构建一组显示项：用户消息为靠右的浅色块，助手回复直接排成正文，
// 思考与工具为安静的单行，可点开查看细节。
func (a *App) block(c *ui.Context, t *ui.Theme, b block) {
	it := b.items[0]
	switch it.kind {
	case itemUser:
		a.message(c, t, it, ui.End, func() {
			ui.Text(c, it.text).Selectable().FontSize(bodySize).Padding(10, 16).Radius(14).
				Background(t.Text.Alpha(0.06)).MaxWidth(bubbleWidth)
		})
	case itemAssistant:
		a.message(c, t, it, ui.Stretch, func() {
			if it.done {
				markdown.View(c, it.text, &a.md)
			} else {
				markdown.Stream(c, it.text, &a.md)
			}
		})
	case itemThinking, itemTool:
		a.stepGroup(c, t, b.items)
	case itemNotice:
		ui.Text(c, it.text).FontSize(13).TextColor(t.TextMuted).Selectable()
	case itemCompaction:
		a.compaction(c, t, it)
	}
}

// compaction 画上下文压缩：进行中是转动的指示器，完成后是一行可展开的说明，展开是摘要。
func (a *App) compaction(c *ui.Context, t *ui.Theme, it *item) {
	if !it.done {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "正在压缩上下文…").FontSize(stepSize).TextColor(t.TextMuted).SingleLine()
			ui.Spinner(c).Size(12, 12).Shrink(0)
		})
		return
	}
	const label = "已压缩较早的对话"
	expandable(c, t, &it.open, label, false, func() {
		ui.Text(c, label).FontSize(stepSize).TextColor(t.TextMuted).SingleLine()
	}, func() {
		panelBody(c, t, false, func() {
			scrolled(c, func() { markdown.View(c, it.text, &a.mdNote) })
		})
	})
}

// message 把一条消息和它下面的一行操作放在一起：指针在其上时，操作行显示发送时间与复制按钮。
// align 是消息在栏中的对齐，用户消息靠右。
func (a *App) message(c *ui.Context, t *ui.Theme, it *item, align ui.Align, body func()) {
	col := ui.Column(c).AlignItems(align)
	hovered := col.Hovered()
	col.Children(func() {
		body()
		a.actions(c, t, it, hovered && it.done)
	})
}

// actions 画消息下方的一行：发送时间与复制按钮，只在 shown 时显示。
// 这一行始终占着高度，显示与否不会让下面的内容跳动。
func (a *App) actions(c *ui.Context, t *ui.Theme, it *item, shown bool) {
	ui.Row(c).Height(barButton).AlignItems(ui.Center).Gap(2).Children(func() {
		if !shown {
			return
		}
		if !it.at.IsZero() {
			ui.Text(c, since(it.at)).FontSize(12).TextColor(t.TextMuted).SingleLine().
				Padding(0, 4, 0, 0).Tooltip(it.at.Local().Format("2006-01-02 15:04:05"))
		}
		glyph, label := copyIcon, "复制"
		if left := copiedFor - time.Since(it.copiedAt); left > 0 {
			glyph, label = checkIcon, "已复制"
			c.After(left) // 到时再画一帧，把图标换回来
		}
		iconButton(c, t, glyph, label, barButton, func() {
			mygo.Clipboard.WriteText(it.text)
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
