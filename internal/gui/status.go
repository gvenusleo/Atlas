package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/agent"
)

// statusRow 是输入框下方的一行：左侧是工作目录，右侧是模型、思考深度与上下文占用。
func (a *App) statusRow(c *ui.Context, t *ui.Theme, ch *chat) {
	ui.Row(c).AlignItems(ui.Center).Gap(2).Padding(0, 6).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Children(func() { a.folder(c, t, ch) })
		a.modelPicker(c, t, ch)
		a.thinkingPicker(c, t, ch)
		a.contextRing(c, t, ch)
	})
}

// folder 显示会话的工作目录；尚未创建的会话可以点击更换。
func (a *App) folder(c *ui.Context, t *ui.Theme, ch *chat) {
	var row ui.Element
	if ch.id == "" && !ch.running {
		row = ui.ButtonBase(c).Cursor(ui.CursorPointer).Label("更换工作目录")
		if row.Hovered() {
			row.Background(t.Text.Alpha(0.06))
		}
		row.OnClick(func() { a.pickDir(ch) })
	} else {
		row = ui.Row(c)
	}
	row.Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(controlRound)
	row.Children(func() {
		ui.Icon(c, folderIcon).FontSize(14).TextColor(t.TextMuted)
		ui.Text(c, shortPath(ch.cwd)).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

// shortPath 把主目录缩写为 ~。
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "~"
		}
		return filepath.Join("~", rel)
	}
	return path
}

// modelPicker 选择会话使用的模型；模型清单没有加载时不显示。
func (a *App) modelPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	if len(a.models) == 0 {
		return
	}
	refs := make([]string, len(a.models))
	for i, m := range a.models {
		refs[i] = m.Ref
	}
	name := func(ref string) string {
		if m, ok := a.modelInfo(ref); ok {
			return m.Name
		}
		return ref
	}
	if dropdown(c, t, "模型", &ch.model, refs, name) {
		a.fitThinking(ch)
	}
}

// thinkingPicker 选择思考深度；模型不支持思考时不显示。
func (a *App) thinkingPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	info, ok := a.modelInfo(ch.model)
	if !ok || len(info.ThinkingLevels) == 0 {
		return
	}
	name := func(l agent.ThinkingLevel) string { return capitalize(string(l)) }
	dropdown(c, t, "思考深度", &ch.thinking, info.ThinkingLevels, name)
}

// capitalize 把首字母大写；思考深度的取值都是 ASCII，尚未选定时为空。
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// dropdown 画一个安静的下拉：只显示当前值，选项在上方或下方展开；返回是否选了新值。
func dropdown[T comparable](
	c *ui.Context, t *ui.Theme, label string, value *T, options []T, name func(T) string,
) bool {
	sel := ui.SelectBase(c.Key(label), value)
	sel.Trigger.AlignItems(ui.Center).Padding(3, 6).Radius(controlRound).
		Cursor(ui.CursorPointer).Label(label)
	if sel.Trigger.Hovered() {
		sel.Trigger.Background(t.Text.Alpha(0.06))
	}
	sel.Trigger.Children(func() {
		ui.Text(c, name(*value)).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(6, 0, 6, 0).Padding(4).Radius(10).Background(t.Background).
			Border(1, t.Border).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.12))
		for _, option := range options {
			item := sel.Item(option).Padding(5, 10).Radius(controlRound)
			if item.Highlighted() {
				item.Background(t.Text.Alpha(0.07))
			}
			item.Children(func() { ui.Text(c, name(option)).FontSize(13).SingleLine() })
		}
	})
	return sel.Trigger.Changed()
}

// contextRing 用圆环显示上下文窗口的占用，悬停时给出具体数字；窗口大小未知时不显示。
func (a *App) contextRing(c *ui.Context, t *ui.Theme, ch *chat) {
	info, ok := a.modelInfo(ch.model)
	if !ok || info.ContextWindow <= 0 {
		return
	}
	used := min(float32(ch.usage)/float32(info.ContextWindow), 1)
	tip := fmt.Sprintf("上下文已用 %.0f%%（%s / %s）",
		used*100, tokens(ch.usage), tokens(info.ContextWindow))
	// 圆环从正上方开始，所以把元素逆时针转 90 度。
	ui.Box(c).Size(16, 16).Margin(0, 0, 0, 6).Rotate(-90).Label(tip).Tooltip(tip).
		Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, r.W/2-1.5
			p.StrokePath((&ui.Path{}).Circle(cx, cy, radius), 2, t.Text.Alpha(0.15))
			if used > 0 {
				p.StrokePath((&ui.Path{}).Circle(cx, cy, radius).Trim(0, used), 2, t.Accent)
			}
		})
}

// tokens 把词元数缩写为 k 或 M。
func tokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	}
	return strconv.FormatInt(n, 10)
}
