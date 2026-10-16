package gui

import (
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const (
	// summaryRunes 是工具行标题中参数摘要的最大字数。
	summaryRunes = 80
	// bodyRunes 是展开后单段文本显示的最大字数。
	bodyRunes = 4000
	// copiedFor 是点复制后，按钮显示“已复制”的时长。
	copiedFor = 1500 * time.Millisecond
)

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
	case itemThinking:
		disclosure(c, t, &it.open, thinkingLabel(it), func() {
			detail(c, t, func() { note(c, it.text, 13, t.TextMuted) })
		})
	case itemTool:
		if len(b.items) == 1 {
			toolRow(c, t, it)
			return
		}
		disclosure(c, t, &it.open, groupLabel(b.items), func() {
			ui.Column(c).Padding(0, 0, 0, 19).Children(func() {
				for _, tool := range b.items {
					toolRow(c, t, tool)
				}
			})
		})
	case itemNotice:
		ui.Text(c, it.text).FontSize(13).TextColor(t.TextMuted).Selectable()
	}
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

// disclosure 画一行可展开的标题：左侧箭头，点击切换 open，展开时显示 body。
func disclosure(c *ui.Context, t *ui.Theme, open *bool, label string, body func()) {
	ui.Column(c).Children(func() {
		head := ui.ButtonBase(c).Gap(6).AlignSelf(ui.Start).Padding(2, 0).
			Cursor(ui.CursorPointer).Label(label)
		head.OnClick(func() { *open = !*open })
		head.Children(func() {
			rotation := float32(0)
			if *open {
				rotation = 90
			}
			ui.Icon(c, chevronIcon).FontSize(13).TextColor(t.TextMuted).Rotate(rotation)
			ui.Text(c, label).FontSize(13).TextColor(t.TextMuted).SingleLine()
		})
		if *open {
			body()
		}
	})
}

// detail 把细节缩进到左侧细线旁。
func detail(c *ui.Context, t *ui.Theme, fn func()) {
	ui.Column(c).Gap(4).Padding(2, 0, 2, 10).Margin(0, 0, 2, 6).
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(fn)
}

// note 画展开细节里的一段可选中的小字。
func note(c *ui.Context, text string, size float32, color ui.Color) {
	ui.Text(c, text).Selectable().FontSize(size).TextColor(color)
}

// toolRow 画一次工具调用：图标、动词与参数摘要；点开显示参数与结果。
func toolRow(c *ui.Context, t *ui.Theme, it *item) {
	glyph, verb := toolInfo(it.tool)
	label := verb
	if s := summary(it.args); s != "" {
		label += " " + s
	}
	ui.Column(c.Key(it.id)).Children(func() {
		head := ui.ButtonBase(c).Gap(8).AlignSelf(ui.Start).Padding(2, 0).
			Cursor(ui.CursorPointer).Label(label)
		head.OnClick(func() { it.open = !it.open })
		head.Children(func() {
			ui.Icon(c, glyph).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, label).FontSize(13).TextColor(t.TextMuted).SingleLine()
			switch {
			case it.running || !it.done:
				ui.Spinner(c).Size(12, 12)
			case it.failed:
				ui.Icon(c, closeIcon).FontSize(14).TextColor(t.Danger)
			}
		})
		if !it.open {
			return
		}
		detail(c, t, func() {
			if it.args != "" {
				note(c, truncate(it.args, bodyRunes), 12, t.TextMuted)
			}
			if it.result != "" {
				color := t.Text
				if it.failed {
					color = t.Danger
				}
				note(c, truncate(it.result, bodyRunes), 12, color)
			}
		})
	})
}

// thinkingLabel 返回思考行的标题：进行中、用时或泛称。
func thinkingLabel(it *item) string {
	switch {
	case !it.done:
		return "思考中…"
	case it.took > 0:
		return fmt.Sprintf("思考了 %d 秒", max(1, int(it.took.Round(time.Second).Seconds())))
	}
	return "思考过程"
}

// groupLabel 返回一组工具调用的标题。
func groupLabel(items []*item) string {
	for _, it := range items {
		if it.running || !it.done {
			return fmt.Sprintf("正在使用 %d 个工具…", len(items))
		}
	}
	return fmt.Sprintf("使用了 %d 个工具", len(items))
}

// summary 取参数中最能说明调用的字段（命令或路径），否则用原始参数。
func summary(args string) string {
	var fields map[string]any
	if json.Unmarshal([]byte(args), &fields) == nil {
		for _, name := range []string{"command", "path"} {
			if s, ok := fields[name].(string); ok && s != "" {
				return truncate(oneLine(s), summaryRunes)
			}
		}
	}
	return truncate(oneLine(args), summaryRunes)
}

// oneLine 把空白折叠为单个空格。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncate 截断到最多 n 个字，被截断时以省略号结尾。
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
