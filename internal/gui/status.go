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

// folder 显示会话的工作目录；还没有保存的新会话可以点击更换。
func (a *App) folder(c *ui.Context, t *ui.Theme, ch *chat) {
	var row ui.Element
	if ch.fresh() && !ch.running && !ch.opening && !ch.switching {
		row = ui.ButtonBase(c).Cursor(ui.CursorPointer).Label("更换工作目录")
		if row.Hovered() {
			row.Background(rowFill(t))
		}
		row.OnClick(func() { a.pickDir(ch) })
	} else {
		row = ui.Row(c)
	}
	row.Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(controlRound)
	row.Children(func() {
		ui.Icon(c, folderIcon).FontSize(smallIconSize).TextColor(t.TextMuted)
		ui.Text(c, shortPath(ch.cwd)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
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

// pickable 报告现在能不能换模型：换模型是对会话做的，要等会话准备好；运行、准备和切换期间不能，
// 会话状态失效了也不能，要等重新恢复。
func (ch *chat) pickable() bool {
	return ch.session != nil && !ch.running && !ch.opening && !ch.switching &&
		!ch.session.State().NeedsReload
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
	model, thinking := a.selection(ch)
	dropdown(c, t, "模型", &model, refs, name, !ch.pickable(), func() {
		a.choose(ch, model, a.fit(model, thinking))
	})
}

// thinkingPicker 选择思考深度；模型不支持思考时不显示。
func (a *App) thinkingPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	model, thinking := a.selection(ch)
	info, ok := a.modelInfo(model)
	if !ok || len(info.ThinkingLevels) == 0 {
		return
	}
	name := func(l agent.ThinkingLevel) string { return capitalize(string(l)) }
	dropdown(c, t, "思考深度", &thinking, info.ThinkingLevels, name, !ch.pickable(), func() {
		a.choose(ch, model, thinking)
	})
}

// capitalize 把首字母大写；思考深度的取值都是 ASCII，尚未选定时为空。
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// dropdown 画一个安静的下拉：只显示当前值，选项在上方或下方展开。value 指向当前值的本帧副本，
// 选了新值后 onChange 里读它；disabled 时点不开。
func dropdown[T comparable](
	c *ui.Context, t *ui.Theme, label string, value *T, options []T, name func(T) string,
	disabled bool, onChange func(),
) {
	sel := ui.SelectBase(c.Key(label), value)
	sel.Trigger.AlignItems(ui.Center).Padding(3, 6).Radius(controlRound).
		Cursor(ui.CursorPointer).Label(label).Disabled(disabled).OnChange(onChange)
	if sel.Trigger.Hovered() {
		sel.Trigger.Background(rowFill(t))
	}
	sel.Trigger.Children(func() {
		ui.Text(c, name(*value)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		popupPanel(panel, t).Margin(6, 0, 6, 0)
		for _, option := range options {
			item := sel.Item(option).Padding(5, 10).Radius(controlRound)
			if item.Highlighted() {
				item.Background(t.Text.Alpha(0.07))
			}
			item.Children(func() {
				text := ui.Text(c, name(option)).FontSize(smallSize).SingleLine()
				if option == *value {
					text.TextColor(t.Accent)
				}
			})
		}
	})
}

// popupPanel 是弹出面板的外观，下拉选项与输入框的补全列表共用；与触发元素之间的间距由
// 各处自己的边距决定。
func popupPanel(panel ui.Element, t *ui.Theme) ui.Element {
	return panel.Padding(4).Radius(10).Background(t.Background).
		Border(1, t.Border).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.12))
}

// contextRing 用圆环显示上下文窗口的占用，悬停时给出具体数字；窗口大小未知时不显示。
func (a *App) contextRing(c *ui.Context, t *ui.Theme, ch *chat) {
	model, _ := a.selection(ch)
	info, ok := a.modelInfo(model)
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
