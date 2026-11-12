package gui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

func (a *App) fileCreateDialog(c *ui.Context) {
	f := a.fileCreate
	open := f != nil && f.matchesChat(a.current)
	ui.Modal(c.Key("file-create"), &open, func() {
		title := "新建文本文件"
		if f.dir {
			title = "新建目录"
		}
		ui.Text(c, title).FontSize(dialogSize).Bold()
		ui.Text(c, "创建到："+shortPath(filepath.Join(f.cwd, f.parent))).Width(320).
			FontSize(smallSize).TextColor(c.Theme().TextMuted)
		ui.TextInput(c.Key("name"), &f.name).Label("名称或相对路径").AutoFocus().Width(320).
			Disabled(f.creating).OnSubmit(func() { a.createEntry(f) })
		ui.Text(c, "可输入 docs/test.md，自动创建父目录").Width(320).
			FontSize(smallSize).TextColor(c.Theme().TextMuted)
		if f.err != "" {
			ui.Text(c, f.err).Width(320).TextColor(c.Theme().Danger)
		}
		ui.Row(c.Key("actions")).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").Disabled(f.creating).OnClick(func() { a.fileCreate = nil })
			label := "创建"
			if f.creating {
				label = "正在创建…"
			}
			ui.PrimaryButton(c, label).Disabled(f.creating).OnClick(func() { a.createEntry(f) })
		})
	})
	// 已提交的文件系统操作无法撤销，完成前不以 Esc 或遮罩关闭。
	if !open && (f == nil || !f.creating || !f.matchesChat(a.current)) {
		a.fileCreate = nil
	}
}
