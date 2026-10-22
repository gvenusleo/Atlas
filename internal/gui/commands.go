package gui

import (
	"strings"
	"unicode/utf8"
)

// compactCommand 是输入框里手动压缩上下文的命令。
const compactCommand = "/compact"

// commandList 是可用的命令：内置命令在前，其后是会话的技能。在输入框里输入 / 时列出。
// 技能补全后接着输入参数，不直接执行。
func (ch *chat) commandList() []option {
	list := []option{{
		name: compactCommand, hint: "把较早的对话压缩成摘要", insert: compactCommand, run: true,
	}}
	for _, s := range ch.skillList() {
		name := "/" + s.Name
		list = append(list, option{
			name: name, hint: s.Description, tag: "Skill", insert: name + " ",
		})
	}
	return list
}

// commandCompletion 返回 text 正在输入的命令：命令名以 text 开头。运行中没有。
func (ch *chat) commandCompletion(text string) completion {
	cur := completion{text: text, to: utf8.RuneCountInString(text)}
	if ch.running {
		return cur
	}
	prefix := strings.ToLower(text)
	for _, opt := range ch.commandList() {
		if strings.HasPrefix(opt.name, prefix) {
			cur.options = append(cur.options, opt)
		}
	}
	return cur
}
