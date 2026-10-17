package gui

import (
	"encoding/json/v2"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/gvenusleo/atlas/internal/tools"
)

const (
	// bodyRunes 是展开后单段文本显示的最大字数。
	bodyRunes = 4000
	// outputRunes 是命令输出最多显示的字数。
	outputRunes = 20000
	// panelHeight 是命令输出与内容对比的最大高度，超出时在框内滚动。
	panelHeight = 280
)

// codeFont 是命令、输出与内容对比用的等宽字体，与终端一致。
var codeFont = ui.Font{Family: termFont.Family, Size: 12}

// toolVerbs 是各类调用完成时与进行中的动词，键见 verbKey。
var toolVerbs = map[string][2]string{
	"shell":     {"已运行", "正在运行"},
	"read":      {"已读取", "正在读取"},
	"write":     {"已写入", "正在写入"},
	"create":    {"已创建", "正在创建"},
	"overwrite": {"已覆盖", "正在覆盖"},
	"edit":      {"已编辑", "正在编辑"},
}

// toolArgs 是工具参数里界面用得到的字段。
type toolArgs struct {
	Command string `json:"command"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	Edits   []struct {
		OldText string `json:"oldText"`
		NewText string `json:"newText"`
	} `json:"edits"`
}

// callView 是一次工具调用的展示数据；参数变化（流式期间）时重建。
type callView struct {
	src            string // 解析的参数原文
	args           toolArgs
	edits          []editDiff        // 编辑各处替换的行级对比，与 args.Edits 一一对应
	starts         []int             // 编辑各处替换在原文件中的起始行，来自结果附带的数据
	overwrote      bool              // 写入覆盖了已有的文件，来自结果附带的数据
	old            *string           // 覆盖前的内容，来自结果附带的数据；没有时为空
	window         tools.ReadDetails // 读取实际返回的窗口，来自结果附带的数据
	added, removed int               // 写入与编辑增删的行数
	cmd            *editor.State
	diffs          []*editor.DiffState // 对比，或读取、覆盖写入的内容
}

// view 返回调用的展示数据；参数没变时沿用上次的。
func (it *item) view() *callView {
	if it.call != nil && it.call.src == it.args {
		return it.call
	}
	v := &callView{src: it.args}
	if json.Unmarshal([]byte(it.args), &v.args) != nil {
		v.args = toolArgs{} // 流式期间参数不完整，解析不出时当作空
	}
	switch it.tool {
	case "read":
		v.window = it.decoded[tools.ReadDetails]()
	case "write":
		v.added = lineCount(v.args.Content)
		details := it.decoded[tools.WriteDetails]()
		v.overwrote, v.old = details.Overwrote, details.Old
		if v.old != nil {
			// 有旧内容时画真正的对比，增删的行数也由它算出。
			d := fileDiff(v.args.Path, *v.old, v.args.Content)
			v.diffs = []*editor.DiffState{d}
			v.added, v.removed = d.Stats()
		}
	case "edit":
		for _, e := range v.args.Edits {
			d := diffEdit(e.OldText, e.NewText)
			v.edits = append(v.edits, d)
			v.added, v.removed = v.added+d.added, v.removed+d.removed
		}
		v.starts = it.decoded[tools.EditDetails]().Starts
	}
	it.call = v
	return v
}

// decoded 把结果附带的结构化数据解码成 T；没有（进行中、失败）或格式不对时是 T 的零值。
func (it *item) decoded[T any]() (v T) {
	if len(it.details) > 0 {
		_ = json.Unmarshal(it.details, &v)
	}
	return v
}

// verbKey 返回调用在 toolVerbs 里的键：工具名，写入有了结果再按附带的数据分成新建与覆盖。
func (it *item) verbKey() string {
	if it.tool != "write" || len(it.details) == 0 {
		return it.tool
	}
	if it.view().overwrote {
		return "overwrite"
	}
	return "create"
}

// stats 返回写入与编辑增删的行数。覆盖已有文件时没有记录旧内容（文件太大或不是文本），
// 就没有可信的数字。
func (it *item) stats() (added, removed int) {
	v := it.view()
	if it.failed || it.verbKey() == "overwrite" && v.old == nil {
		return 0, 0
	}
	return v.added, v.removed
}

// lineCount 返回文本的行数，末尾的换行不另算一行。
func lineCount(s string) int {
	n := 0
	for range strings.Lines(s) {
		n++
	}
	return n
}

// title 返回一次调用的标题：动词（完成的或进行中的）、主体与完整的主体（作提示）。
func (it *item) title() (verb, subject, tip string) {
	v := it.view()
	verbs, ok := toolVerbs[it.verbKey()]
	if !ok {
		verbs = [2]string{"已调用", "正在调用"}
		subject, tip = it.tool, it.tool
	}
	verb = verbs[0]
	if !it.done {
		verb = verbs[1]
	}
	// 命令不进标题，展开后才看到；文件的标题是文件名。
	switch it.tool {
	case "read", "write", "edit":
		if v.args.Path != "" {
			subject, tip = filepath.Base(v.args.Path), v.args.Path
		}
	}
	return verb, subject, tip
}

// stepGroup 构建一组连续的思考与工具调用：只有一个时标题就是它自己的一行，
// 多个时标题按类别汇总，展开后是一张卡片，每行再展开看细节。
func (a *App) stepGroup(c *ui.Context, t *ui.Theme, items []*item) {
	if len(items) == 1 {
		a.stepRow(c, t, items[0], false)
		return
	}
	label, added, removed, running := groupSummary(items)
	expandable(c, t, &items[0].shown, label, false, func() {
		ui.Text(c, label).FontSize(stepSize).TextColor(t.TextMuted).SingleLine()
		if added+removed > 0 {
			diffPill(c, t, added, removed)
		}
		if running {
			ui.Spinner(c).Size(12, 12).Shrink(0)
		}
	}, func() {
		ui.Column(c).Margin(4, 0, 0).Radius(10).Border(1, t.Border).Clip().Children(func() {
			for i, it := range items {
				row := ui.Column(c.Key(it.id))
				if i > 0 {
					row.BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
				}
				row.Children(func() { a.stepRow(c, t, it, true) })
			}
		})
	})
}

// stepRow 画一步：思考或工具调用；card 为真时在卡片里，标题撑满整行。
func (a *App) stepRow(c *ui.Context, t *ui.Theme, it *item, card bool) {
	if it.kind == itemThinking {
		a.thinkStep(c, t, it, card)
		return
	}
	a.toolStep(c, t, it, card)
}

// thinkStep 画一次思考：标题是用时，展开后是思考的文字，太长时在框内滚动。
func (a *App) thinkStep(c *ui.Context, t *ui.Theme, it *item, card bool) {
	label := thinkingLabel(it)
	expandable(c, t, &it.open, label, card, func() {
		ui.Text(c, label).FontSize(stepSize).TextColor(t.TextMuted).SingleLine()
		if !it.done {
			ui.Spinner(c).Size(12, 12).Shrink(0)
		}
	}, func() {
		panelBody(c, t, card, func() {
			scrolled(c, func() { note(c, it.text, 13, t.TextMuted) })
		})
	})
}

// toolStep 画一次调用：标题行与展开后的细节；card 为真时在卡片里，标题撑满整行。
func (a *App) toolStep(c *ui.Context, t *ui.Theme, it *item, card bool) {
	verb, subject, tip := it.title()
	added, removed := it.stats()
	expandable(c, t, &it.open, strings.TrimSpace(verb+" "+subject), card, func() {
		ui.Text(c, verb).FontSize(stepSize).TextColor(t.TextMuted).SingleLine()
		if subject != "" {
			ui.Text(c, subject).FontSize(stepSize).SingleLine().Tooltip(tip)
		}
		if added+removed > 0 {
			diffPill(c, t, added, removed)
		}
		switch {
		case it.failed:
			ui.Text(c, "失败").FontSize(stepSize).TextColor(t.Danger).SingleLine()
		case !it.done:
			ui.Spinner(c).Size(12, 12).Shrink(0)
		}
	}, func() {
		panelBody(c, t, card, func() { a.toolDetail(c, t, it) })
	})
}

// panelBody 把步骤展开后的细节放进面板：在卡片里缩进到行内，单独的步骤则自带圆角框。
func panelBody(c *ui.Context, t *ui.Theme, card bool, fn func()) {
	if card {
		ui.Column(c).Padding(0, 12, 12).Children(fn)
		return
	}
	ui.Column(c).Margin(4, 0, 0).Padding(12).Radius(10).Border(1, t.Border).Children(fn)
}

// scrolled 把内容放进限高的框：超过 panelHeight 时在框内滚动。
func scrolled(c *ui.Context, fn func()) {
	ui.Scroll(c).MaxHeight(panelHeight).Children(fn)
}

// expandable 画一个可展开的步骤：标题行（head 画内容，其后是箭头）与展开后的面板，
// 展开与收起带动画；card 为真时标题撑满卡片的宽度，悬停有浅底。
func expandable(
	c *ui.Context, t *ui.Theme, open *bool, label string, card bool, head, body func(),
) {
	p := ui.CollapsibleBase(c, open)
	tr := p.Trigger.AlignItems(ui.Center).Gap(6).Cursor(ui.CursorPointer).Label(label)
	if card {
		tr.Padding(8, 12)
		if tr.Hovered() {
			tr.Background(t.Text.Alpha(0.04))
		}
	} else {
		tr.AlignSelf(ui.Start).Padding(2, 0)
	}
	tr.Children(func() {
		head()
		ui.Icon(c, chevronIcon).FontSize(13).TextColor(t.TextMuted).Rotate(90 * p.Progress())
	})
	p.Panel(body)
}

// 汇总里不在 toolVerbs 的两类的键：思考，以及界面不认识的工具。
const (
	thinkingKey = "thinking"
	otherKey    = "other"
)

// tally 是一组调用里同一类的汇总。
type tally struct {
	key    string // toolVerbs 的键，或 thinkingKey、otherKey
	n      int    // 调用次数
	failed int
	files  []string      // 涉及的不同文件
	took   time.Duration // 思考的总用时
}

// phrase 把汇总说成一句：“已运行 6 个命令（1 个失败）”“已读取 a.go”“已写入 2 个文件”。
func (g tally) phrase() string {
	verb := toolVerbs[g.key][0]
	var s string
	switch {
	case g.key == thinkingKey:
		return thought(g.took)
	case g.key == otherKey:
		s = fmt.Sprintf("已调用 %d 个工具", g.n)
	case g.key == "shell":
		s = fmt.Sprintf("%s %d 个命令", verb, g.n)
	case len(g.files) == 1:
		s = verb + " " + filepath.Base(g.files[0])
	default:
		s = fmt.Sprintf("%s %d 个文件", verb, max(len(g.files), g.n))
	}
	if g.failed > 0 {
		s += fmt.Sprintf("（%d 个失败）", g.failed)
	}
	return s
}

// groupSummary 汇总一组调用，各类按首次出现的顺序；进行中时改为最新一步的标题。
// 同时返回写入与编辑增删的总行数，以及是否还有调用在进行。
func groupSummary(items []*item) (label string, added, removed int, running bool) {
	var tallies []tally
	var live *item
	for _, it := range items {
		if !it.done {
			live = it
		}
		a, r := it.stats()
		added, removed = added+a, removed+r
		key := it.verbKey()
		if _, known := toolVerbs[key]; !known {
			key = otherKey
		}
		if it.kind == itemThinking {
			key = thinkingKey
		}
		i := slices.IndexFunc(tallies, func(g tally) bool { return g.key == key })
		if i < 0 {
			tallies = append(tallies, tally{key: key})
			i = len(tallies) - 1
		}
		g := &tallies[i]
		g.n++
		g.took += it.took
		if it.failed {
			g.failed++
		}
		if path := it.view().args.Path; path != "" && !slices.Contains(g.files, path) {
			g.files = append(g.files, path)
		}
	}
	if live != nil && live.kind == itemThinking {
		return thinkingLabel(live), added, removed, true
	}
	if live != nil {
		verb, subject, _ := live.title()
		return strings.TrimSpace(verb + " " + subject), added, removed, true
	}
	phrases := make([]string, len(tallies))
	for i, g := range tallies {
		phrases[i] = g.phrase()
	}
	return strings.Join(phrases, "，"), added, removed, false
}

// diffPill 画“+147 −0”的小标签：左半绿、右半红。
func diffPill(c *ui.Context, t *ui.Theme, added, removed int) {
	ui.Row(c).Shrink(0).Radius(4).Clip().Children(func() {
		ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(12).TextColor(t.Success).
			Background(t.Success.Alpha(0.15)).Padding(1, 6).SingleLine()
		ui.Text(c, fmt.Sprintf("−%d", removed)).FontSize(12).TextColor(t.Danger).
			Background(t.Danger.Alpha(0.15)).Padding(1, 6).SingleLine()
	})
}

// note 画展开细节里的一段可选中的小字。
func note(c *ui.Context, text string, size float32, color ui.Color) {
	ui.Text(c, text).Selectable().FontSize(size).TextColor(color)
}

// thinkingLabel 返回思考行的标题：进行中，或思考的用时。
func thinkingLabel(it *item) string {
	if !it.done {
		return "思考中…"
	}
	return thought(it.took)
}

// thought 把思考的用时说成“思考了 N 秒”；没有记录用时（历史记录）时是“已思考”。
func thought(took time.Duration) string {
	if took <= 0 {
		return "已思考"
	}
	return fmt.Sprintf("思考了 %d 秒", max(1, int(took.Round(time.Second).Seconds())))
}

// toolDetail 画展开后的细节：命令与输出、读取的路径与内容、写入与编辑的内容对比。
func (a *App) toolDetail(c *ui.Context, t *ui.Theme, it *item) {
	v := it.view()
	ui.Column(c).Gap(8).Children(func() {
		if v.args.Path != "" {
			ui.Text(c, v.args.Path).Selectable().FontSize(12).TextColor(t.TextMuted)
		}
		switch {
		case it.failed && it.tool != "shell":
			// 读取、写入、编辑失败时只说明原因，没有内容可看。
		case it.tool == "shell":
			shellCommand(c, t, v)
		case it.tool == "read":
			fileContent(c, t, it, v)
		case it.verbKey() == "overwrite":
			overwritten(c, t, v)
		case it.tool == "write" || it.tool == "edit":
			diffBody(c, t, it, v)
		default:
			if it.args != "" {
				note(c, truncate(it.args, bodyRunes), 12, t.TextMuted)
			}
		}
		_, known := toolVerbs[it.tool]
		if it.result != "" && (it.failed || it.tool == "shell" || !known) {
			color := t.Text
			if it.failed && it.tool != "shell" {
				color = t.Danger
			}
			shellOutput(c, truncate(it.result, outputRunes), color)
		}
	})
}

// boxColor 是代码框的底色：文字色淡淡地铺在背景上。
func boxColor(t *ui.Theme) ui.Color { return t.Text.Alpha(0.04).Over(t.Background) }

// codeTheme 是代码与对比的配色：窗口主题的色板，底色换成代码框的。
func codeTheme(t *ui.Theme) *editor.Theme {
	th := editor.DefaultTheme(t)
	th.Background = boxColor(t)
	return th
}

// shellCommand 画命令：浅底的圆角框里，美元符号后是只读且带高亮的命令。
func shellCommand(c *ui.Context, t *ui.Theme, v *callView) {
	if v.cmd == nil {
		buf := editor.NewBuffer(v.args.Command)
		buf.SetLanguage(editor.LanguageNamed("bash"))
		v.cmd = &editor.State{Buffer: buf, ReadOnly: true, SoftWrap: true, FitContent: true,
			LineNumbers: editor.LineNumbersOff, NoFolding: true, NoActiveLine: true,
			NoIndentGuides: true, NoScrollPastEnd: true, Font: codeFont, Label: "命令"}
	}
	v.cmd.Theme = codeTheme(t)
	ui.Row(c).AlignItems(ui.Start).Padding(8, 10).Background(boxColor(t)).Radius(8).
		Children(func() {
			ui.Text(c, "$").Font(codeFont.Family).FontSize(codeFont.Size).TextColor(t.TextMuted)
			editor.View(c, v.cmd).Grow(1).MinWidth(0)
		})
}

// shellOutput 画命令输出或错误：等宽字体，超过 panelHeight 时在框内滚动。
func shellOutput(c *ui.Context, text string, color ui.Color) {
	scrolled(c, func() {
		// 末尾的换行不显示成多出来的一行。
		ui.Text(c, strings.TrimRight(text, "\n")).Selectable().Font(codeFont.Family).
			FontSize(codeFont.Size).TextColor(color)
	})
}
