package gui

import (
	"strings"
	"unicode/utf8"
)

// commandList 是可用的命令，来自核心：/compact 在前，其后是会话的技能。在输入框里输入 / 时
// 列出。带参数的命令（技能）补全后接着输入参数，不直接执行。
func (ch *chat) commandList() []option {
	var list []option
	for _, c := range ch.commands {
		name := "/" + c.Name
		opt := option{name: name, hint: c.Description, insert: name, run: c.Input == nil}
		if c.Input != nil {
			opt.tag, opt.insert = "Skill", name+" "
		}
		list = append(list, opt)
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
