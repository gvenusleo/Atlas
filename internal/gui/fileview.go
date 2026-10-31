package gui

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// imageInset 是文件面板里图片四周的留白。
const imageInset = 16

// filesCard 是右侧栏里的文件面板卡片：顶部是标签栏，其下左边是文件树，右边是当前标签的内容。
func (a *App) filesCard(c *ui.Context, t *ui.Theme, ch *chat, height float32) {
	fp := ch.files
	if fp.kids == nil || fp.cwd != ch.cwd {
		moved := fp.kids != nil // 之前是另一个工作目录，监视要换到新的
		fp.reset(ch.cwd)
		a.readDir(ch, "")
		if moved {
			a.watchFiles(ch)
		}
	}
	sideCard(c, t, "files-card", height).Children(func() {
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
		DragWindow().Children(func() {
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
		Label("文件树").Tooltip("文件树")
	color := t.TextMuted
	switch {
	case !fp.treeHidden:
		btn.Background(accentFg(t).Alpha(0.15))
		color = accentFg(t)
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

// fileHeader 画内容区顶部：路径，所在的目录淡一些、文件名在最后；能渲染的文件在右端有
// 预览与源码的切换按钮。
func fileHeader(c *ui.Context, t *ui.Theme, rel string, tb *fileTab) {
	ui.Row(c).AlignItems(ui.Center).Padding(4, 8, 4, 14).Children(func() {
		pathLabel(c, t, filepath.ToSlash(rel))
		renderToggle(c, t, tb)
	})
}

// renderToggle 是内容区右上角的按钮，在渲染后的样子（Markdown 与 SVG 的预览、CSV 的表格）
// 与源码之间切换；只有源码一种视图的文件不画。
func renderToggle(c *ui.Context, t *ui.Theme, tb *fileTab) {
	if tb.ed == nil || !(isMarkdown(tb.path) || isSVG(tb.path) || isCSV(tb.path)) {
		return
	}
	glyph, label := codeIcon, "查看源码"
	if !tb.render {
		glyph, label = eyeIcon, "预览"
		if isCSV(tb.path) {
			label = "查看表格"
		}
	}
	iconButton(c, t, glyph, label, barButton, func() { tb.render = !tb.render })
}

// pathLabel 画一个文件的路径（用 / 分隔，相对工作目录）：所在的目录淡一些、文件名在最后；
// 放不下时裁掉右边。文件面板与更改面板的内容区顶部共用。
func pathLabel(c *ui.Context, t *ui.Theme, rel string) {
	dir, name := path.Split(rel)
	dir = strings.ReplaceAll(strings.TrimSuffix(dir, "/"), "/", " / ")
	ui.Row(c).Grow(1).MinWidth(0).Gap(4).ClipX().Children(func() {
		if dir != "" {
			ui.Text(c, dir+" /").FontSize(textSize).TextColor(t.TextMuted).SingleLine().Shrink(0)
		}
		ui.Text(c, name).FontSize(textSize).SingleLine()
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

// fileBody 画标签的内容：读取中、图片、不能预览的原因、表格、Markdown 的预览，或编辑器。
func (a *App) fileBody(c *ui.Context, t *ui.Theme, tb *fileTab) {
	switch {
	case tb.loading:
		loading.Ring(c, loading.Options{Color: t.TextMuted}).Margin(ui.Auto)
	case tb.img != nil:
		imagePreview(c, tb.img, filepath.Base(tb.path))
	case tb.ed == nil:
		// 不能预览的文件：一句原因，以及在默认应用中打开的入口。
		ui.Column(c).Gap(12).AlignItems(ui.Center).Margin(ui.Auto).Children(func() {
			ui.Text(c, tb.note).FontSize(textSize).TextColor(t.TextMuted).TextAlign(ui.Center)
			if ui.Button(c, "在默认应用中打开").Disabled(tb.gone).Clicked() {
				if err := mygo.Shell.OpenPath(tb.path); err != nil {
					a.toast = "打开失败：" + err.Error()
				}
			}
		})
	case tb.render && isCSV(tb.path):
		a.fileTable(c, t, tb)
	case tb.render && isSVG(tb.path):
		a.svgPreview(c, t, tb)
	case tb.render:
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 28, 28).Children(func() {
				markdown.View(c, tb.previewText(), &a.mdFile)
			})
		})
	default:
		theme := editor.DefaultTheme(t)
		theme.Background = t.Background
		tb.ed.Theme = theme
		tb.ed.Font = codeFont
		editor.View(c, tb.ed).Grow(1).MinHeight(0)
	}
}

// imagePreview 在可滚动区域里画一张图：按面板宽度等比缩放，不放大。
func imagePreview(c *ui.Context, src ui.ImageSource, name string) {
	w, _ := ui.ImageSize(src)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(imageInset).Children(func() {
			ui.Image(c, src).FillWidth().MaxWidth(w).Radius(boxRound).Label(name)
		})
	})
}

// svgPreview 画 SVG 的预览：从缓冲区里的文本解析，改了源码预览跟着变。
func (a *App) svgPreview(c *ui.Context, t *ui.Theme, tb *fileTab) {
	img := tb.svg.get(tb.ed.Buffer.Version(), func() svgImage {
		return parseSVG(tb.ed.Buffer.String())
	})
	if img.note != "" {
		ui.Text(c, img.note).FontSize(smallSize).TextColor(t.TextMuted).Padding(8, 14)
		return
	}
	imagePreview(c, img.src, filepath.Base(tb.path))
}

// fileTable 画 CSV/TSV 的表格：首行作表头，只建视口里的行。解析结果按缓冲区版本缓存，
// 在源码里改过、切回来就是新的；排序只改显示顺序，不碰缓冲区。
func (a *App) fileTable(c *ui.Context, t *ui.Theme, tb *fileTab) {
	table := tb.csv.get(tb.ed.Buffer.Version(), func() csvTable {
		return parseCSV(tb.ed.Buffer.String(), csvComma(tb.path))
	})
	tb.table.Sort = &tb.sort
	if tb.sorted != tb.sort {
		// 排序变了就读回表头：排完还停在原处，看到的既不是开头也不是刚才那一行。
		tb.sorted = tb.sort
		tb.table.ScrollTo(0, ui.Start)
	}
	key := orderKey{
		version:    tb.ed.Buffer.Version(),
		column:     tb.sort.Column,
		descending: tb.sort.Descending,
	}
	order := tb.order.get(key, func() []int {
		col, err := strconv.Atoi(tb.sort.Column)
		if err != nil {
			col = -1
		}
		return table.order(col, tb.sort.Descending)
	})
	tb.table.Key = func(row int) any { return order[row] }
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		note := table.note()
		if note != "" {
			ui.Text(c, note).FontSize(smallSize).TextColor(t.TextMuted).Padding(6, 14)
		}
		if len(table.head) == 0 {
			if note == "" {
				ui.Text(c, "文件是空的。").FontSize(textSize).TextColor(t.TextMuted).Margin(ui.Auto)
			}
			return // 解析就失败了，原因上面已经写过
		}
		// 顶上的那条线把表格与上面的内容分开，表头下面的一条是表格自己的。
		ui.Table(c, &tb.table, tableColumns(table), len(order), func(row, col int) {
			ui.Text(c, table.cell(order[row], col)).SingleLine()
		}).Grow(1).MinHeight(0).BorderWidth(1, 0, 0, 0).BorderColor(t.Border)
	})
}

// tableColumns 把表头变成表格的列：标识用序号，表头可能有空的或重复的；数据行比表头宽时
// 后面的列没有标题。列可以排序。
func tableColumns(table csvTable) []ui.TableColumn {
	cols := make([]ui.TableColumn, table.width())
	for i := range cols {
		title := ""
		if i < len(table.head) {
			title = table.head[i]
		}
		cols[i] = ui.TableColumn{Title: title, ID: strconv.Itoa(i), Sortable: true}
	}
	return cols
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
