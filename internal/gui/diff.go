package gui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	// diffLines 是写入内容最多显示的行数。
	diffLines = 300
	// headerHeight 是 DiffView 给补丁每一块的头留的高度，文档写明是 32。
	headerHeight = 32
)

// editDiff 是编辑里一处替换的行级对比。
type editDiff struct {
	lines              []string // 补丁里的行：空格开头不变，- 删除，+ 新增
	oldCount, newCount int      // 替换前后的行数
	added, removed     int      // 增删的行数
	shift              int      // 这处替换使文件多出的换行数，累计到后面各处在新文件里的起始行
}

// diffEdit 对比一处替换的前后文本。找出改动与统计增删都用 DiffView 自己的 diff（不渲染
// 也能算），这里只把它找出的改动区间排成补丁里的行。
func diffEdit(oldText, newText string) editDiff {
	oldLines, newLines := textLines(oldText), textLines(newText)
	d := editDiff{oldCount: len(oldLines), newCount: len(newLines),
		shift: strings.Count(newText, "\n") - strings.Count(oldText, "\n")}
	if len(newLines) == 0 {
		// 删除：DiffView 把空文本当作一个空行，会算成新增，所以直接排。
		d.removed = len(oldLines)
		d.lines = prefixed(nil, "-", oldLines)
		return d
	}
	state := &editor.DiffState{Old: editor.NewBuffer(strings.Join(oldLines, "\n")),
		New: editor.NewBuffer(strings.Join(newLines, "\n"))}
	d.added, d.removed = state.Stats()
	pos := 0
	for _, h := range state.Hunks() {
		d.lines = prefixed(d.lines, " ", oldLines[pos:h.OldStart])
		d.lines = prefixed(d.lines, "-", oldLines[h.OldStart:h.OldEnd])
		d.lines = prefixed(d.lines, "+", newLines[h.NewStart:h.NewEnd])
		pos = h.OldEnd
	}
	d.lines = prefixed(d.lines, " ", oldLines[pos:])
	return d
}

// prefixed 给每一行加上补丁的行首符号，追加到 dst。
func prefixed(dst []string, mark string, lines []string) []string {
	for _, l := range lines {
		dst = append(dst, mark+l)
	}
	return dst
}

// deleteOnly 报告各处替换是否都只删除、没有新文本。
func deleteOnly(edits []editDiff) bool {
	return len(edits) > 0 && !slices.ContainsFunc(edits, func(e editDiff) bool {
		return e.newCount > 0
	})
}

// diffBody 画写入与编辑的内容：写入是整份新增，编辑是各处替换的前后对比。
func diffBody(c *ui.Context, t *ui.Theme, it *item, v *callView) {
	edit := it.tool == "edit"
	// 编辑的起始行在执行完附带的数据里，等到那时再建。
	if edit && (!it.done || len(v.starts) != len(v.edits)) {
		return
	}
	if edit && deleteOnly(v.edits) {
		deletedRows(c, t, v.args, v.starts)
		return
	}
	if v.diffs == nil {
		v.diffs = buildDiffs(it.tool, v)
	}
	if len(v.diffs) == 0 {
		return
	}
	diffBox(c, t, v.diffs, edit)
	if note := truncatedNote(v.args.Content); !edit && note != "" {
		ui.Text(c, note).FontSize(12).TextColor(t.TextMuted)
	}
}

// diffBox 把对比放进浅底的圆角框，超过 panelHeight 时在框内滚动。hideHeader 为真时把每份
// 补丁第一块的头用负的上外边距挤出框外：它只占地方；编辑的后面几块的头是分隔，留着。
func diffBox(c *ui.Context, t *ui.Theme, ds []*editor.DiffState, hideHeader bool) {
	ui.Column(c).Background(boxColor(t)).Radius(8).Clip().Children(func() {
		scrolled(c, func() {
			for _, d := range ds {
				d.Theme = codeTheme(t)
				view := editor.DiffView(c, d).FillWidth()
				if hideHeader {
					view.Margin(-headerHeight, 0, 0, 0)
				}
			}
		})
	})
}

// fileContent 画读取到的文件内容：用工具报告的实际窗口排成只有上下文行的补丁，
// 带真实行号；只读了文件的一部分时先说明读的是哪几行。
func fileContent(c *ui.Context, t *ui.Theme, it *item, v *callView) {
	w := v.window
	if w.Start < 1 || w.End < w.Start {
		// 空文件没有行可画；图片只有一句说明。
		if it.result != "" {
			note(c, it.result, 12, t.TextMuted)
		}
		return
	}
	if w.Start > 1 || w.End < w.Total {
		caption := fmt.Sprintf("第 %d–%d 行，共 %d 行", w.Start, w.End, w.Total)
		ui.Text(c, caption).FontSize(12).TextColor(t.TextMuted)
	}
	if v.diffs == nil {
		// 结果文字末尾可能还有给模型的续读说明，按窗口的行数只取读到的内容。
		text := headLines(it.result, w.End-w.Start+1)
		v.diffs = []*editor.DiffState{contextPatch(v.args.Path, text, w.Start)}
	}
	diffBox(c, t, v.diffs, true)
}

// overwritten 画覆盖已有文件时写入的内容：旧内容不在记录里，没有对比可看，
// 所以只显示新内容，行号从 1 数起。
func overwritten(c *ui.Context, t *ui.Theme, v *callView) {
	if v.diffs == nil {
		head := headLines(v.args.Content, diffLines)
		v.diffs = []*editor.DiffState{contextPatch(v.args.Path, head, 1)}
	}
	diffBox(c, t, v.diffs, true)
	if note := truncatedNote(v.args.Content); note != "" {
		ui.Text(c, note).FontSize(12).TextColor(t.TextMuted)
	}
}

// contextPatch 把一段文本排成只有上下文行的补丁，让 DiffView 以真实行号显示它：
// start 是第一行在文件里的行号。
func contextPatch(path, text string, start int) *editor.DiffState {
	lines := textLines(text)
	d := diffState()
	d.SetPatch(editor.FilePatch{OldName: path, NewName: path, Hunks: []editor.PatchHunk{{
		OldStart: start, OldLines: len(lines), NewStart: start, NewLines: len(lines),
		Lines: prefixed(nil, " ", lines),
	}}})
	return d
}

// truncatedNote 在写入的内容超过 diffLines 行时说明只显示了前面一部分。
func truncatedNote(content string) string {
	if n := lineCount(content); n > diffLines {
		return fmt.Sprintf("仅显示前 %d 行，共 %d 行", diffLines, n)
	}
	return ""
}

// buildDiffs 建写入或编辑的对比：写入对比空文本，编辑排成带真实行号的补丁。
func buildDiffs(tool string, v *callView) []*editor.DiffState {
	if tool == "edit" {
		return []*editor.DiffState{editPatch(v.args.Path, v.edits, v.starts)}
	}
	if v.args.Content == "" {
		return nil
	}
	return []*editor.DiffState{newFileDiff(v.args.Path, headLines(v.args.Content, diffLines))}
}

// diffState 是对比的统一样式：上下合并，行首有 + 和 -，长行换行，与外层的滚动框配合。
func diffState() *editor.DiffState {
	return &editor.DiffState{Layout: editor.DiffUnified, Indicators: editor.DiffClassic,
		ExpandUnchanged: true, Wrap: true, FitContent: true, Font: codeFont}
}

// newFileDiff 对比空文本与新文件的内容：整份都是新增，行号从 1 数起。
func newFileDiff(path, text string) *editor.DiffState {
	// 末尾的换行不显示成多出来的一行。
	after := editor.NewBuffer(strings.TrimSuffix(text, "\n"))
	after.SetLanguage(editor.LanguageFor(path))
	d := diffState()
	d.Old, d.New = editor.NewBuffer(""), after
	return d
}

// editPatch 把一次编辑的各处替换排成一份补丁，行号是文件里真实的行号：
// 各处按在原文件中的位置从前到后，新文件的起始行要计入前面各处增删的行数。
func editPatch(path string, edits []editDiff, starts []int) *editor.DiffState {
	var hunks []editor.PatchHunk
	gaps := map[int]int{} // 各块的原起始行 → 它与上一块之间没改动的行数
	shift, prevEnd := 0, 0
	for k, i := range inFileOrder(starts) {
		e := edits[i]
		hunks = append(hunks, editor.PatchHunk{
			OldStart: starts[i], OldLines: e.oldCount,
			NewStart: starts[i] + shift, NewLines: e.newCount,
			Lines: e.lines,
		})
		if k > 0 {
			gaps[starts[i]] = starts[i] - prevEnd
		}
		prevEnd = starts[i] + e.oldCount
		shift += e.shift
	}
	d := diffState()
	d.RenderSeparator = hunkDivider(gaps)
	d.SetPatch(editor.FilePatch{OldName: path, NewName: path, Hunks: hunks})
	return d
}

// hunkDivider 画补丁里每一块的头，取代 @@ 头：一条细线，中间隔着没改动的行时再加上
// 淡淡的“N 行未修改”。第一块的头被挤出了框外，只有后面几块的看得到。
func hunkDivider(gaps map[int]int) func(*ui.Context, ui.Element, *editor.DiffSeparator) {
	return func(c *ui.Context, sep ui.Element, s *editor.DiffSeparator) {
		var line int
		if n, _ := fmt.Sscanf(s.Header, "@@ -%d", &line); n != 1 {
			editor.DefaultSeparator(c, sep, s)
			return
		}
		t := c.Theme()
		sep.Row().AlignItems(ui.Center).Padding(0, 10).BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
		if gap := gaps[line]; gap > 0 {
			sep.Children(func() {
				ui.Text(c, fmt.Sprintf("%d 行未修改", gap)).FontSize(12).TextColor(t.TextMuted)
			})
		}
	}
}

// inFileOrder 返回各处替换按在原文件中的位置从前到后的下标。
func inFileOrder(starts []int) []int {
	order := make([]int, len(starts))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(x, y int) int { return cmp.Compare(starts[x], starts[y]) })
	return order
}

// deletedRows 画只删除、没有新增的编辑：新文本为空时 DiffView 会多出一行空行，
// 所以这种编辑自己画红色的删除行。
func deletedRows(c *ui.Context, t *ui.Theme, a toolArgs, starts []int) {
	mono := func(s string) ui.Element {
		return ui.Text(c, s).Font(codeFont.Family).FontSize(codeFont.Size)
	}
	ui.Column(c).Radius(8).Clip().Children(func() {
		scrolled(c, func() {
			for _, i := range inFileOrder(starts) {
				for j, line := range textLines(a.Edits[i].OldText) {
					ui.Row(c).MinHeight(18).Gap(8).Padding(0, 10).
						Background(t.Danger.Alpha(0.12)).Children(func() {
						mono(strconv.Itoa(starts[i] + j)).TextColor(t.Danger).Width(32).
							TextAlign(ui.End)
						mono("-").TextColor(t.Danger)
						mono(line).Selectable().Grow(1)
					})
				}
			}
		})
	})
}

// textLines 把文本拆成行，末尾的换行不另算一行；空文本没有行。
func textLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// headLines 返回文本的前 n 行。
func headLines(s string, n int) string {
	end := 0
	for line := range strings.Lines(s) {
		if n == 0 {
			break
		}
		end += len(line)
		n--
	}
	return s[:end]
}
