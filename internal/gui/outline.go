package gui

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 会话大纲的尺寸，单位为 DIP：outlineHeight 是最大高度，提问多时在里面滚动。
const (
	outlineWidth  = 320
	outlineHeight = 360
)

// outline 是一个会话的大纲：标题栏上的下拉，每条提问一行，选一条就滚到那条提问。
type outline struct {
	older   []prompt // 查到的、前面还没加载进对话的更早的提问，正序
	fetched bool     // 已经去查过更早的提问
	picked  int      // 下拉绑定的值，见 outlineButton
}

// prompt 是大纲里的一条提问：已加载进对话的有显示项 ID，从历史里查到的只有记录序号。
type prompt struct {
	id   int
	seq  int64
	text string
	at   time.Time
	row  int // 已加载的提问在消息列表里的行号；未加载的为 -1
}

// hasOutline 报告会话是否值得有大纲：至少两条提问，或前面还有没加载的记录。
func (ch *chat) hasOutline() bool {
	users := 0
	for _, it := range ch.items {
		if it.kind == itemUser {
			users++
		}
	}
	return users >= 2 || ch.oldest > 1
}

// outlinePrompts 列出大纲里的提问：先是还没加载的更早的，再是对话里的。查到之后又加载进来的，
// 由对话里的那份取代，所以加载更早的记录不改变条数与顺序。
func (ch *chat) outlinePrompts(blocks []block) []prompt {
	var out []prompt
	for _, p := range ch.outline.older {
		if p.seq < ch.oldest {
			out = append(out, p)
		}
	}
	for i, b := range blocks {
		if it := b.items[0]; it.kind == itemUser {
			out = append(out, prompt{id: it.id, seq: it.seq, at: it.at, row: i,
				text: promptLine(it.text, len(it.pictures) > 0)})
		}
	}
	return out
}

// olderPrompts 把历史里查到的用户消息排成大纲里的条目。
func olderPrompts(entries []protocol.UserMessageSummary) []prompt {
	out := make([]prompt, 0, len(entries))
	for _, e := range entries {
		out = append(out, prompt{seq: e.Seq, at: e.At, row: -1, text: promptLine(e.Text, e.Images)})
	}
	return out
}

// promptLine 是提问在大纲里的一行：文字的第一行；没有文字时，有图片是“[图片]”。
func promptLine(text string, pictures bool) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	switch {
	case line != "":
		return truncate(line, 80)
	case pictures:
		return "[图片]"
	}
	return "（无文字）"
}

// currentPrompt 是视野顶端所在的提问：行号不超过列表第一个可见行的最后一条，没有时为 -1。
func currentPrompt(prompts []prompt, first int) int {
	current := -1
	for i, p := range prompts {
		if p.row >= 0 && p.row <= first {
			current = i
		}
	}
	return current
}

// fetchOlder 在后台从历史里查出前面还没加载进对话的提问，每个会话只查一次：更早的记录不会
// 再变，之后加载进对话的那部分由 outlinePrompts 滤掉。
func (a *App) fetchOlder(ch *chat) {
	o := &ch.outline
	if o.fetched || ch.id == "" || ch.oldest <= 1 {
		return
	}
	o.fetched = true
	id, before := ch.id, ch.oldest
	go func() {
		resp, err := a.service.ListUserMessages(context.Background(),
			protocol.ListUserMessagesRequest{SessionID: id, Before: before})
		a.win.Update(func() {
			if err != nil {
				ch.err = err.Error()
				return
			}
			o.older = olderPrompts(resp.Messages)
		})
	}()
}

// outlineButton 在标题栏画大纲：一个下拉，每条提问一行，选一条就滚到它。下拉的键盘操作、
// 打开时滚到选中项、悬停高亮都由 mygo 的 Select 负责。
func (a *App) outlineButton(c *ui.Context, t *ui.Theme, ch *chat, slot int) {
	a.fetchOlder(ch)
	o := &ch.outline
	prompts := ch.outlinePrompts(ch.blocks())
	// 视野里的提问以顶端那条为准；停在末尾时，顶端多半是上一轮的尾巴，以末尾为准。
	top, bottom := ch.list.Visible()
	if ch.list.AtEnd() {
		top = bottom
	}
	current := currentPrompt(prompts, top)

	sel := ui.SelectBase(c.Key("outline"), &o.picked)
	styleBarButton(c, t, sel.Trigger, slot, outlineIcon, "大纲", sel.Open())
	sel.Popup(func(panel ui.Element) {
		popupPanel(panel, t).Width(outlineWidth).Margin(6, 0, 6, 0)
		ui.Scroll(c).MaxHeight(outlineHeight).Children(func() {
			for i, p := range prompts {
				outlineRow(c, t, sel.Item(i), p, i == current)
			}
		})
	})
	if sel.Trigger.Changed() {
		ch.jump(prompts[o.picked])
	}
	// 下拉绑定的值：关着时设成当前的提问，打开时下拉就从它开始高亮；打开之后设成 -1，
	// 这样选哪一条都算一次新的选择（Changed），包括当前这条，选它是要回到它的开头。
	o.picked = current
	if sel.Open() {
		o.picked = -1
	}
}

// outlineRow 画大纲里的一行：提问的第一行文字，和它发出的时间；current 的一行文字用强调色。
func outlineRow(c *ui.Context, t *ui.Theme, item ui.Element, p prompt, current bool) {
	item.Padding(5, 10).Radius(controlRound).AlignItems(ui.Center).Gap(10)
	if item.Highlighted() {
		item.Background(t.Text.Alpha(0.07))
	}
	item.Children(func() {
		text := ui.Text(c, p.text).FontSize(textSize).SingleLine().Grow(1).MinWidth(0)
		if current {
			text.TextColor(accentFg(t))
		}
		if !p.at.IsZero() {
			ui.Text(c, since(p.at)).FontSize(smallSize).TextColor(t.TextMuted).
				SingleLine().Shrink(0)
		}
	})
}

// jump 滚到大纲里的一条提问，让它排在视野顶端。它还没加载进对话时，记下要跳到它，
// 由 loadOlder 一路加载过来，到了再由 seek 滚过去。
func (ch *chat) jump(p prompt) {
	if row := rowOf(ch.blocks(), p); row >= 0 {
		ch.list.ScrollTo(row, ui.Start)
		return
	}
	ch.seekSeq = p.seq
}

// seek 在要跳到的提问加载进来之后滚到它。
func (ch *chat) seek(blocks []block) {
	if ch.seekSeq == 0 {
		return
	}
	if row := rowOf(blocks, prompt{seq: ch.seekSeq}); row >= 0 {
		ch.list.ScrollTo(row, ui.Start)
		ch.seekSeq = 0
	}
}

// rowOf 返回大纲里的提问所在的行；还没加载进对话时为 -1。
func rowOf(blocks []block, p prompt) int {
	return slices.IndexFunc(blocks, func(b block) bool {
		it := b.items[0]
		return it.kind == itemUser && (it.id == p.id || p.seq != 0 && it.seq == p.seq)
	})
}
