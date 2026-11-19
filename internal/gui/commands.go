package gui

import (
	"cmp"
	"strings"
	"unicode"
)

// findCommandCompletion 查找光标处的斜杠命令候选。内置命令必须单独发送，因为选中后会立即执行。
func (ch *chat) findCommandCompletion(text string, caret, end int) completion {
	if ch.running || caret != end {
		return completion{}
	}
	runes := []rune(text)
	if caret > len(runes) {
		return completion{}
	}
	from := caret
	for from > 0 && !unicode.IsSpace(runes[from-1]) {
		from--
	}
	if from == caret || runes[from] != '/' {
		return completion{}
	}
	to := caret
	for to < len(runes) && !unicode.IsSpace(runes[to]) {
		to++
	}
	word := string(runes[from:to])
	prefix := strings.ToLower(word)
	standalone := strings.TrimSpace(text) == word
	// 将已有的分隔空白原样插回，使补全后的光标落在分隔符之后。
	var separator string
	if to < len(runes) {
		separator = string(runes[to])
		to++
	}
	cur := completion{text: text, from: from, to: to}
	for _, cmd := range ch.commands {
		name := "/" + cmd.Name
		run := cmd.Input == nil
		if (run && !standalone) || !strings.HasPrefix(name, prefix) {
			continue
		}
		opt := option{name: name, hint: cmd.Description, insert: name + separator, run: run}
		if !run {
			opt.tag, opt.insert = "Skill", name+cmp.Or(separator, " ")
		}
		cur.options = append(cur.options, opt)
	}
	return cur
}
