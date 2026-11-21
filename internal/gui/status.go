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

// folder 显示会话的工作目录。新会话可以换目录，Git 工作树还可以打开管理入口。
func (a *App) folder(c *ui.Context, t *ui.Theme, ch *chat) {
	var row ui.Element
	git := !ch.cwdGone && ch.inRepo()
	switch {
	case git:
		label := "工作树"
		if ch.fresh() {
			label = "选择工作目录"
		}
		row = ui.ButtonBase(c.Key("worktree-folder")).Label(label).Tooltip(label).
			OnClick(func() { a.openWorktrees(ch) })
	case ch.fresh():
		row = ui.ButtonBase(c.Key("plain-folder")).Label("更换工作目录").
			OnClick(func() { a.pickDir(ch) })
	default:
		row = ui.Row(c)
	}
	if row.Hovered() && (ch.fresh() || git) {
		row.Background(rowFill(t))
	}
	// 允许路径收缩，避免长路径挤占分支与模型选择器的空间。
	row.Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(controlRound).MinWidth(0).Shrink(1)
	row.Children(func() {
		ui.Icon(c, folderIcon).FontSize(smallIconSize).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, shortPath(ch.cwd)).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().
			MinWidth(0)
	})
	if git {
		a.worktreeMenu(c, t, ch, row)
	}
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
// 会话没打开或模型不支持思考时没有。
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
	dropdown(c, t, opt.Name, nil, &value, values, name, !ch.ready || ch.choosing, func() {
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
		MinWidth(0).Shrink(1).Label(label).Disabled(disabled).DimDisabled(false).OnChange(onChange)
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
	dropdownOptions(c, t, sel, value, options, name)
}

// dropdownOptions 画下拉展开后的选项，字号与其它弹出层的行一致；当前值用强调色。触发器由
// 调用方画。
func dropdownOptions[T comparable](
	c *ui.Context, t *ui.Theme, sel ui.SelectParts[T], value *T, options []T, name func(T) string,
) {
	sel.Popup(func(panel ui.Element) {
		popupPanel(panel, t).Margin(6, 0, 6, 0).Children(func() {
			ui.Scroll(c).MaxHeight(dropdownMaxHeight).Children(func() {
				for _, option := range options {
					item := popupItem(sel.Item(option), t)
					item.Children(func() {
						text := ui.Text(c, name(option)).FontSize(textSize).SingleLine()
						if option == *value {
							text.TextColor(accentFg(t))
						}
					})
				}
			})
		})
	})
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
