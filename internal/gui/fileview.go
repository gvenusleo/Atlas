package gui

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// filesCard 是右侧栏里的文件面板卡片：顶部是标签栏，其下左边是文件树，右边是当前标签的内容。
func (a *App) filesCard(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	if fp.dirs == nil || fp.cwd != ch.cwd {
		moved := fp.dirs != nil // 之前是另一个工作目录，监视要换到新的
		fp.reset(ch.cwd)
		a.readDir(ch, "")
		if moved {
			a.watchFiles(ch)
		}
	}
	sideCard(c, t, "files-card", 0).Children(func() {
		a.fileTabs(c, t, ch)
		if fp.treeHidden {
			a.fileContent(c, t, ch)
			return
		}
		// 文件树不超过卡片宽度的一半，内容区至少留得下。
		fp.treeWidth = min(max(fp.treeWidth, treeMin), max(a.sideWidth/2, treeMin))
		ui.Split(c, &fp.treeWidth, func() { a.fileTree(c, t, ch) },
			func() { a.fileContent(c, t, ch) }).Grow(1).MinHeight(0)
	})
}

// fileTabs 画卡片顶部的标签栏：左端是文件树开关，其后是打开的文件，右端是关闭按钮。
func (a *App) fileTabs(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	items := make([]tabItem, len(fp.tabs))
	for i, tb := range fp.tabs {
		items[i] = tabItem{
			id: tb.id, icon: fileIcon(tb.path), label: filepath.Base(tb.path),
			dirty: tb.dirty(), closeLabel: "关闭文件",
			pick: func() {
				fp.active, fp.reveal = i, tb.id
				a.syncTab(tb)
			},
			close: func() { a.closeFileTab(ch, tb) },
		}
	}
	ui.Row(c).Height(tabBarHeight(c)).AlignItems(ui.Center).Gap(2).Padding(0, tabInset).
		Children(func() {
			treeToggle(c, t, fp)
			if len(items) == 0 {
				ui.Text(c, "文件").FontSize(textSize).Padding(0, 6)
			}
			a.tabStrip(c, t, "file", items, fp.active, &fp.reveal)
			ui.Row(c).Grow(1)
			a.cardActions(c, t, ch, filesPanel, "关闭文件面板", a.toggleFiles)
		})
}

// treeToggle 是标签栏左端的文件树开关；文件树显示时用强调色标出。
func treeToggle(c *ui.Context, t *ui.Theme, fp *filePanel) {
	btn := ui.ButtonBase(c).Size(barButton, barButton).Shrink(0).Radius(controlRound).
		Cursor(ui.CursorPointer).Label("文件树").Tooltip("文件树")
	color := t.TextMuted
	switch {
	case !fp.treeHidden:
		btn.Background(t.Accent.Alpha(0.15))
		color = t.Accent
	case btn.Hovered():
		btn.Background(buttonFill(t))
	}
	btn.OnClick(func() { fp.treeHidden = !fp.treeHidden })
	btn.Children(func() { ui.Icon(c, listIcon).FontSize(iconSize).TextColor(color) })
}

// fileContent 画内容区：没有标签时是说明，否则是当前标签的路径与内容。
func (a *App) fileContent(c *ui.Context, t *ui.Theme, ch *chat) {
	fp := ch.files
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if len(fp.tabs) == 0 {
			emptyFiles(c, t)
			return
		}
		tb := fp.tabs[fp.active]
		ui.Column(c.Key("file-" + strconv.Itoa(tb.id))).Grow(1).MinHeight(0).Children(func() {
			fileHeader(c, t, relOrAbs(ch.cwd, tb.path), tb)
			a.fileBanner(c, t, tb)
			a.fileBody(c, t, tb)
		})
	})
}

// fileHeader 画内容区顶部：路径，所在的目录淡一些、文件名在最后；Markdown 文件在右端有预览与
// 源码的切换按钮。
func fileHeader(c *ui.Context, t *ui.Theme, rel string, tb *fileTab) {
	dir, name := path.Split(filepath.ToSlash(rel))
	ui.Row(c).AlignItems(ui.Center).Padding(4, 8, 4, 14).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(4).ClipX().Children(func() {
			if dir != "" {
				dir = strings.ReplaceAll(strings.TrimSuffix(dir, "/"), "/", " / ") + " /"
				ui.Text(c, dir).FontSize(textSize).TextColor(t.TextMuted).SingleLine().Shrink(0)
			}
			ui.Text(c, name).FontSize(textSize).SingleLine()
		})
		if tb.ed != nil && isMarkdown(tb.path) {
			glyph, label := codeIcon, "查看源码"
			if !tb.preview {
				glyph, label = eyeIcon, "预览"
			}
			iconButton(c, t, glyph, label, barButton, func() { tb.preview = !tb.preview })
		}
	})
}

// fileBanner 在路径下方提示标签的文件有什么问题：保存失败、磁盘上的文件被别处改了（可以
// 重新加载或保留自己的修改）、文件被删了。没有问题时什么也不画。
func (a *App) fileBanner(c *ui.Context, t *ui.Theme, tb *fileTab) {
	msg, color := "", t.Warning
	switch {
	case tb.err != "":
		msg, color = tb.err, t.Danger
	case tb.changed:
		msg = "文件已在磁盘上更改。"
	case tb.gone:
		msg = "文件已被删除，保存会重新创建它。"
	default:
		return
	}
	ui.Column(c).Gap(6).Padding(8, 14).Background(color.Alpha(0.12)).Children(func() {
		ui.Text(c, msg).FontSize(smallSize)
		if tb.changed && tb.err == "" {
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "重新加载").Clicked() {
					a.reloadTab(tb, true)
				}
				if ui.Button(c, "保留我的修改").Clicked() {
					tb.keep()
				}
			})
		}
	})
}

// fileBody 画标签的内容：读取中、不能预览的原因、Markdown 的预览，或编辑器。
func (a *App) fileBody(c *ui.Context, t *ui.Theme, tb *fileTab) {
	switch {
	case tb.loading:
		ui.Spinner(c).Margin(ui.Auto)
	case tb.ed == nil:
		ui.Text(c, tb.note).FontSize(textSize).TextColor(t.TextMuted).TextAlign(ui.Center).
			Margin(ui.Auto)
	case tb.preview:
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 28, 28).Children(func() {
				markdown.View(c, tb.previewText(), &a.mdFile)
			})
		})
	default:
		theme := editor.DefaultTheme(t)
		theme.Background = t.Background
		tb.ed.Theme = theme
		editor.View(c, tb.ed).Grow(1).MinHeight(0)
	}
}

// emptyFiles 是没有打开文件时内容区的说明。
func emptyFiles(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Padding(24).
		Children(func() {
			ui.Icon(c, folderIcon).FontSize(emptyIconSize).TextColor(t.TextMuted)
			ui.Text(c, "打开的文件显示在这里").FontSize(bodySize)
			ui.Text(c, "请在文件树中选择一个文件。").FontSize(smallSize).TextColor(t.TextMuted)
		})
}
