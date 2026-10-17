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

// diffLines 是写入内容最多显示的行数。
const diffLines = 300

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
	diffBox(c, t, v.diffs)
	if note := truncatedNote(v.args.Content); !edit && note != "" {
		ui.Text(c, note).FontSize(12).TextColor(t.TextMuted)
	}
}

// diffBox 把对比放进浅底的圆角框，超过 panelHeight 时在框内滚动。
func diffBox(c *ui.Context, t *ui.Theme, ds []*editor.DiffState) {
	ui.Column(c).Children(func() {
		ui.Column(c).Background(boxColor(t)).Radius(boxRound).Clip().Children(func() {
			scrolled(c, func() {
				for _, d := range ds {
					d.Theme = codeTheme(t)
					editor.DiffView(c, d).FillWidth()
				}
			})
		})
		roundCorners(c, t)
	})
}

// roundCorners 在圆角框的四角盖上窗口的背景色，让圆角在 GPU 上也是圆的。圆角框里套着矩形
// 裁剪（滚动容器的、DiffView 自己的）时，GPU 只让最里层的裁剪成形，外层裁剪的圆角丢了，
// CPU 渲染则不会，所以只靠 Radius 与 Clip，真窗口里框是方角的。这个圆角相框画在框之上、
// 向外多出 boxRound，内缘正好是框的圆角；指针穿过它。
func roundCorners(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Top(-boxRound).Right(-boxRound).Bottom(-boxRound).Left(-boxRound).
		Radius(2*boxRound).Border(boxRound, t.Background).PassThrough()
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
	diffBox(c, t, v.diffs)
}

// overwritten 画覆盖已有文件时写入的内容：有旧内容时是前后的对比（视图数据里已经建好），
// 没有（文件太大或不是文本）就没有对比可看，只显示新内容，行号从 1 数起。
func overwritten(c *ui.Context, t *ui.Theme, v *callView) {
	if v.old != nil {
		diffBox(c, t, v.diffs)
		return
	}
	if v.diffs == nil {
		head := headLines(v.args.Content, diffLines)
		v.diffs = []*editor.DiffState{contextPatch(v.args.Path, head, 1)}
	}
	diffBox(c, t, v.diffs)
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
	return []*editor.DiffState{fileDiff(v.args.Path, "", headLines(v.args.Content, diffLines))}
}

// diffState 是对比的统一样式：上下合并，行首有 + 和 -，长行换行，与外层的滚动框配合。
func diffState() *editor.DiffState {
	return &editor.DiffState{Layout: editor.DiffUnified, Indicators: editor.DiffClassic,
		ExpandUnchanged: true, Wrap: true, FitContent: true, Font: codeFont}
}

// fileDiff 对比文件前后的内容，行号从 1 数起：新建文件时 old 为空，整份都是新增；
// 覆盖时没有改动的行按 DiffView 原生的做法折叠。
func fileDiff(path, old, text string) *editor.DiffState {
	// 末尾的换行不显示成多出来的一行。
	before := editor.NewBuffer(strings.TrimSuffix(old, "\n"))
	after := editor.NewBuffer(strings.TrimSuffix(text, "\n"))
	lang := editor.LanguageFor(path)
	before.SetLanguage(lang)
	after.SetLanguage(lang)
	d := diffState()
	d.ExpandUnchanged = false
	d.Old, d.New = before, after
	return d
}

// editPatch 把一次编辑的各处替换排成一份补丁，行号是文件里真实的行号：
// 各处按在原文件中的位置从前到后，新文件的起始行要计入前面各处增删的行数。
func editPatch(path string, edits []editDiff, starts []int) *editor.DiffState {
	var hunks []editor.PatchHunk
	shift := 0
	for _, i := range inFileOrder(starts) {
		e := edits[i]
		hunks = append(hunks, editor.PatchHunk{
			OldStart: starts[i], OldLines: e.oldCount,
			NewStart: starts[i] + shift, NewLines: e.newCount,
			Lines: e.lines,
		})
		shift += e.shift
	}
	d := diffState()
	d.SetPatch(editor.FilePatch{OldName: path, NewName: path, Hunks: hunks})
	return d
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
	ui.Column(c).Children(func() {
		ui.Column(c).Radius(boxRound).Clip().Children(func() {
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
		roundCorners(c, t)
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
