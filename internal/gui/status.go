package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
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
	if ch.fresh() {
		row = ui.ButtonBase(c).Label("更换工作目录")
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

// modelPicker 选择会话使用的模型，thinkingPicker 选择思考深度：都照着核心给的配置项画，
// 会话没打开或模型不支持思考时没有。运行中不能换。
func (a *App) modelPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	a.configPicker(c, t, ch, protocol.ConfigModel)
}

func (a *App) thinkingPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	a.configPicker(c, t, ch, protocol.ConfigThinking)
}

// configPicker 把一个配置项画成下拉。
func (a *App) configPicker(c *ui.Context, t *ui.Theme, ch *chat, id string) {
	opt, ok := ch.option(id)
	if !ok || len(opt.Options) == 0 {
		return
	}
	values := make([]string, len(opt.Options))
	for i, v := range opt.Options {
		values[i] = v.Value
	}
	name := func(value string) string {
		if i := slices.Index(values, value); i >= 0 {
			return opt.Options[i].Name
		}
		return value
	}
	value := opt.CurrentValue
	dropdown(c, t, opt.Name, &value, values, name, !ch.ready || ch.running, func() {
		a.choose(ch, id, value)
	})
}

// dropdown 画一个安静的下拉：只显示当前值，选项在上方或下方展开。value 指向当前值的本帧副本，
// 选了新值后 onChange 里读它；disabled 时点不开。
func dropdown[T comparable](
	c *ui.Context, t *ui.Theme, label string, value *T, options []T, name func(T) string,
	disabled bool, onChange func(),
) {
	sel := ui.SelectBase(c.Key(label), value)
	sel.Trigger.AlignItems(ui.Center).Padding(3, 6).Radius(controlRound).
		Label(label).Disabled(disabled).OnChange(onChange)
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
					text.TextColor(accentFg(t))
				}
			})
		}
	})
}

// sheetPanel 是居中浮层（启动器、设置页）的外观：遮罩半透明，面板宽 width、圆角、阴影较重、
// 没有内边距，内容自己排版。返回面板，调用方接着设高度等。
func sheetPanel(backdrop, panel ui.Element, t *ui.Theme, width float32) ui.Element {
	backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
	return popupPanel(panel, t).Width(width).Radius(cardRound).Padding(0).
		Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.25))
}

// popupPanel 是弹出面板的外观，下拉选项与输入框的补全列表共用；与触发元素之间的间距由
// 各处自己的边距决定。
func popupPanel(panel ui.Element, t *ui.Theme) ui.Element {
	return panel.Padding(4).Radius(10).Background(t.Background).
		Border(1, t.Border).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.12))
}

// contextRing 用圆环显示上下文窗口的占用，悬停时给出具体数字；窗口大小未知时不显示。
func (a *App) contextRing(c *ui.Context, t *ui.Theme, ch *chat) {
	if ch.usage.ContextWindow <= 0 {
		return
	}
	used := min(float32(ch.usage.Used)/float32(ch.usage.ContextWindow), 1)
	tip := fmt.Sprintf("上下文已用 %.0f%%（%s / %s）",
		used*100, tokens(ch.usage.Used), tokens(ch.usage.ContextWindow))
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
