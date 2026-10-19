package gui

import (
	"cmp"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// mentionLimit 是路径筛选最多列出的项数，@ 补全与文件树的筛选都用它。
const mentionLimit = 50

// mentionCompletion 返回光标处的 @ 词的候选：路径里包含它已输入的部分的文件与目录。
// 输入框里选了一段文字、或光标不在 @ 词里时没有。
func (a *App) mentionCompletion(ch *chat, input ui.Element) completion {
	caret, end := input.TextSelection()
	if caret != end {
		return completion{}
	}
	from, to, query := findMention(ch.input, caret)
	if from < 0 {
		return completion{}
	}
	if ch.index.stale(ch.cwd) {
		a.loadPaths(ch)
	}
	options := ch.menu.mention.get(rankKey{query, ch.index.version}, func() []option {
		return pathOptions(ch.index.paths, query)
	})
	return completion{text: ch.input, from: from, to: to, options: options}
}

// findMention 返回 text 里光标 caret（rune 偏移）处正在输入的 @ 词：它在 text 里的 rune
// 区间 [from, to)，与 @ 后面光标前已输入的路径；光标不在 @ 词里时 from 为 -1。
// 不带引号的词到光标为止，后面的字不属于它；带引号的词到闭合的引号为止。
func findMention(text string, caret int) (from, to int, query string) {
	runes := []rune(text)
	if caret > len(runes) {
		return -1, 0, ""
	}
	before := runes[:caret]
	if from = mentionStart(before); from < 0 {
		return -1, 0, ""
	}
	query, quoted := strings.CutPrefix(string(before[from+1:]), `"`)
	to = caret
	if quoted {
		rest := runes[caret:]
		i := slices.IndexFunc(rest, func(r rune) bool { return r == '"' || r == '\n' })
		if i >= 0 && rest[i] == '"' {
			to = caret + i + 1
		}
	}
	return from, to, query
}

// mentionStart 返回 before 末尾那个 @ 词的 @ 的下标，没有时返回 -1。@ 要在词首：前面
// 不能是 ASCII 字母或数字，这样邮箱不算，而紧跟在汉字后面的 @ 算。词里不含空白，
// 只有 @" 开头的、路径带空白的词可以含空白，直到再遇到引号。
func mentionStart(before []rune) int {
	for i, r := range slices.Backward(before) {
		if r == '\n' {
			break
		}
		if r == '"' {
			if i > 0 && before[i-1] == '@' && atWordStart(before, i-1) {
				return i - 1
			}
			break
		}
	}
	for i, r := range slices.Backward(before) {
		if unicode.IsSpace(r) {
			break
		}
		if r == '@' {
			if atWordStart(before, i) {
				return i
			}
			break
		}
	}
	return -1
}

// atWordStart 报告 runes[i] 前面是不是词的边界：开头，或不是 ASCII 字母与数字。
func atWordStart(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	r := runes[i-1]
	return r >= utf8.RuneSelf || !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// pathOptions 把路径按 rankPaths 排好，换成补全列表里的项。
func pathOptions(paths []string, query string) []option {
	var options []option
	for _, p := range rankPaths(paths, query, mentionLimit) {
		options = append(options, pathOption(p))
	}
	return options
}

// rankPaths 返回路径里包含 query（忽略大小写）的前 limit 项：文件名等于 query 的
// 最前，其次是文件名以它开头的、文件名包含它的、仅路径包含它的；同一档里浅的在前，
// 再按路径长短与字母序。
func rankPaths(paths []string, query string, limit int) []string {
	type hit struct {
		path         string
		score, depth int
	}
	query = strings.ToLower(query)
	var hits []hit
	for _, p := range paths {
		if score := scorePath(p, query); score > 0 {
			hits = append(hits, hit{p, score, strings.Count(strings.TrimSuffix(p, "/"), "/")})
		}
	}
	slices.SortFunc(hits, func(x, y hit) int {
		return cmp.Or(cmp.Compare(y.score, x.score), cmp.Compare(x.depth, y.depth),
			cmp.Compare(len(x.path), len(y.path)), cmp.Compare(x.path, y.path))
	})
	var ranked []string
	for _, h := range hits[:min(len(hits), limit)] {
		ranked = append(ranked, h.path)
	}
	return ranked
}

// scorePath 给路径与已转小写的 query 的贴近程度打分，不包含时为 0。
func scorePath(p, query string) int {
	name := strings.ToLower(path.Base(p))
	switch {
	case name == query:
		return 4
	case strings.HasPrefix(name, query):
		return 3
	case strings.Contains(name, query):
		return 2
	case strings.Contains(strings.ToLower(p), query):
		return 1
	}
	return 0
}

// pathOption 是一个文件或目录（以 / 结尾）在列表里的一项：图标按类型，名称是文件名，
// 说明是所在的目录，选中后换成 @路径，路径带空白时加引号。
func pathOption(p string) option {
	dir, name := path.Split(strings.TrimSuffix(p, "/"))
	if strings.HasSuffix(p, "/") {
		name += "/"
	}
	mention := "@" + p
	if strings.ContainsFunc(p, unicode.IsSpace) {
		mention = `@"` + p + `"`
	}
	return option{
		name: name, hint: strings.TrimSuffix(dir, "/"), icon: fileIcon(p), insert: mention + " ",
	}
}
