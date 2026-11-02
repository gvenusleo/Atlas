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
		ui.Row(c).Grow(1).MinWidth(0).Children(func() {
			a.folder(c, t, ch)
			a.branchPicker(c, t, ch)
		})
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
	// 允许路径收缩，避免长路径挤占分支与模型选择器的空间。
	row.Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(controlRound).MinWidth(0).Shrink(1)
	row.Children(func() {
		ui.Icon(c, folderIcon).FontSize(smallIconSize).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, shortPath(ch.cwd)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().
			MinWidth(0)
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

// branchPick 是分支列表里的一项：分支名，以及它是不是远程分支。
type branchPick struct {
	name   string
	remote bool
}

// branchPicker 是状态行里的分支下拉，与模型、思考深度是同一个控件：当前分支就是它的值，
// 选项是本地与远程分支，选中即切换；同一工作树有会话运行时禁用。
func (a *App) branchPicker(c *ui.Context, t *ui.Theme, ch *chat) {
	b := ch.branch
	if b.name == "" {
		return
	}
	options := make([]branchPick, 0, len(b.list.local)+len(b.list.remote))
	for _, name := range b.list.local {
		options = append(options, branchPick{name: name})
	}
	for _, name := range b.list.remote {
		options = append(options, branchPick{name: name, remote: true})
	}
	value := branchPick{name: b.name}
	dropdown(c, t, "分支", branchIcon, &value, options,
		func(p branchPick) string { return p.name }, a.branchBusy(ch), func() {
			if value.name != b.name { // 选了当前分支就不动
				a.chooseBranch(ch, value.name, value.remote)
			}
		})
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
	dropdown(c, t, opt.Name, nil, &value, values, name, !ch.ready || ch.running, func() {
		a.choose(ch, id, value)
	})
}

// dropdownMaxHeight 限制选项列表高度，选项较多时通过滚动查看。
const dropdownMaxHeight = 320

// dropdown 构建下拉选择器，glyph 可为空。value 指向当前值的本帧副本，
// onChange 在选择完成后读取它；disabled 为真时禁用。
func dropdown[T comparable](
	c *ui.Context, t *ui.Theme, label string, glyph *ui.SVG, value *T, options []T,
	name func(T) string, disabled bool, onChange func(),
) {
	sel := ui.SelectBase(c.Key(label), value)
	// 长名称允许收缩并省略，避免超出状态行宽度。
	sel.Trigger.AlignItems(ui.Center).Gap(6).Padding(3, 6).Radius(controlRound).
		MinWidth(0).Shrink(1).Label(label).Disabled(disabled).OnChange(onChange)
	if sel.Trigger.Hovered() {
		sel.Trigger.Background(rowFill(t))
	}
	sel.Trigger.Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).FontSize(smallIconSize).TextColor(t.TextMuted).Shrink(0)
		}
		ui.Text(c, name(*value)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().
			MinWidth(0)
	})
	dropdownOptions(c, t, sel, value, options, name, smallSize)
}

// dropdownOptions 画下拉展开后的选项，字号是 size；当前值用强调色。触发器由调用方画。
func dropdownOptions[T comparable](
	c *ui.Context, t *ui.Theme, sel ui.SelectParts[T], value *T, options []T, name func(T) string,
	size float32,
) {
	sel.Popup(func(panel ui.Element) {
		popupPanel(panel, t).Margin(6, 0, 6, 0).Children(func() {
			ui.Scroll(c).MaxHeight(dropdownMaxHeight).Children(func() {
				for _, option := range options {
					item := popupItem(sel.Item(option), t)
					item.Children(func() {
						text := ui.Text(c, name(option)).FontSize(size).SingleLine()
						if option == *value {
							text.TextColor(accentFg(t))
						}
					})
				}
			})
		})
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
	return panel.Padding(4).Radius(popupRound).Background(t.Background).
		Border(1, t.Border).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.12))
}

// popupItem 是弹出面板里一行的外观：下拉的选项、大纲、补全列表与启动器共用；键盘或鼠标停在
// 下拉的选项上时铺淡底。补全列表与启动器的行是按钮，没有这个状态，悬停与选中由它们自己画。
// 内边距与 MyGo 的 Combobox 的选项一致，设置里的字体选择器因此同高。
func popupItem(item ui.Element, t *ui.Theme) ui.Element {
	item.Padding(6, 10).Radius(controlRound)
	if item.Highlighted() {
		item.Background(rowFill(t))
	}
	return item
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
