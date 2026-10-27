package gui

import (
	"github.com/egoist/mygo/ui"
)

// 设置页面板的尺寸与留白。settingsPad 是内容离左右与底部边缘的留白，也是标题到第一项的间距，
// 与下面角落里标题和关闭叉离边缘的 18 一致；settingsGap 是各项之间的间距；fontBoxWidth 是
// 字体框的宽度，放得下最长的字体名。
const (
	settingsWidth  = 720
	settingsHeight = 480
	settingsNavW   = 180
	settingsPad    = 18
	settingsGap    = 14
	fontBoxWidth   = 220
)

// 角落里的“设置”“外观”标题与关闭的叉，看得见的部分离面板边缘都是 18 DIP。文字与图标在各自的
// 盒子里有内边距，所以盒子比 18 更靠近边缘，这几个数是量出来的：标题行（高 barButton）离顶部
// settingsTop，分类栏的左右留白 settingsNavSide 再加行内的 8；关闭按钮（barButton 见方）
// 单独定位，离顶部 settingsCloseTop、离右侧 settingsCloseRight。
const (
	settingsTop        = 11
	settingsNavSide    = 9
	settingsCloseTop   = 9
	settingsCloseRight = 9
)

// settingsCategory 是设置页左侧的一个分类。
type settingsCategory struct {
	name string
	icon *ui.SVG
}

// settingsCategories 是设置页左侧的分类。
var settingsCategories = []settingsCategory{{"外观", paletteIcon}}

// settingsDialog 在设置页打开时画它：左侧是分类，右侧是当前分类的各项。
func (a *App) settingsDialog(c *ui.Context, t *ui.Theme) {
	s := &a.settings
	width, height := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		sheetPanel(backdrop, panel, t, min(settingsWidth, width-2*gutter)).
			Height(min(settingsHeight, height-2*gutter)).Clip()
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.settingsNav(c, t)
			a.settingsContent(c, t)
		})
	})
}

// settingsNav 画左侧的分类列表。
func (a *App) settingsNav(c *ui.Context, t *ui.Theme) {
	s := &a.settings
	ui.Column(c).Width(settingsNavW).Shrink(0).Padding(settingsTop, settingsNavSide).Gap(2).
		Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Height(barButton).AlignItems(ui.Center).Padding(0, 8).Margin(0, 0, 12).
			Children(func() { ui.Text(c, "设置").FontSize(dialogSize).Bold() })
		for i, category := range settingsCategories {
			row := ui.ButtonBase(c).Justify(ui.Start).AlignItems(ui.Center).Gap(8).
				Padding(6, 8).Radius(controlRound).Label(category.name)
			color := t.TextMuted
			switch {
			case i == s.category:
				row.Background(t.Text.Alpha(0.09))
				color = t.Text
			case row.Hovered():
				row.Background(rowFill(t))
			}
			row.OnClick(func() { s.category = i })
			row.Children(func() {
				ui.Icon(c, category.icon).FontSize(iconSize).TextColor(color)
				ui.Text(c, category.name)
			})
		}
	})
}

// settingsContent 画右侧：分类的标题固定在顶部，关闭按钮在右上角，其下的各项在放不下时滚动。
func (a *App) settingsContent(c *ui.Context, t *ui.Theme) {
	s := &a.settings
	ui.Column(c).Grow(1).MinWidth(0).Padding(settingsTop, 0, 0).Children(func() {
		ui.Row(c).Height(barButton).AlignItems(ui.Center).Padding(0, 0, 0, settingsPad).
			Children(func() {
				ui.Text(c, settingsCategories[s.category].name).FontSize(dialogSize).Bold()
			})
		quietButton(c, t, "关闭设置", func() { s.open = false }).
			Absolute().Top(settingsCloseTop).Right(settingsCloseRight).Size(barButton, barButton).
			Children(func() {
				ui.Icon(c, closeIcon).FontSize(barButton * 0.6).TextColor(t.TextMuted)
			})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(settingsGap).Padding(settingsPad).Children(func() {
				a.appearance(c, t)
			})
		})
	})
}

// appearance 画“外观”：主题、界面字体、代码字体，以及按当前选择排版的预览。
func (a *App) appearance(c *ui.Context, t *ui.Theme) {
	s := &a.settings
	settingRow(c, "主题", func() { a.themeSwitch(c, t) })
	settingRow(c, "界面字体", func() { a.fontBox(c, t, "界面字体", &s.uiFont, s.uiFonts) })
	settingRow(c, "代码字体", func() { a.fontBox(c, t, "代码字体", &s.codeFont, s.codeFonts) })
	fontPreview(c, t)
}

// fontBox 画一个带搜索的字体下拉；下拉里的行用本应用安静的样式，与模型、思考深度的下拉一致，
// 不用 MyGo 默认的强调色高亮。选了字体就存进偏好。
func (a *App) fontBox(c *ui.Context, t *ui.Theme, label string, value *string, options []string) {
	quietRows(c, t, func() {
		// Combobox 的面板圆角是主题圆角加 2，选项行圆角是主题圆角，高亮的行用 Accent 做底：
		// 面板取与 popupPanel 相同的圆角，高亮用与 popupItem 相同的淡底。
		local := *c.Theme()
		local.Radius, local.Accent = popupRound-2, rowFill(t)
		c.SetTheme(&local)
		ui.Combobox(c.Key("settings-"+label), value, options).Label(label).
			Width(fontBoxWidth).Padding(3, 8).Radius(controlRound).FontSize(textSize).
			OnChange(a.chooseFonts)
	})
}

// themeSwitch 画主题的三个图标分段：太阳、月亮、显示器；悬停给出名称。
func (a *App) themeSwitch(c *ui.Context, t *ui.Theme) {
	s := &a.settings
	segmented(c, t, "settings-theme", "主题", &s.theme, len(themeChoices), 32,
		func() { a.chooseTheme(s.theme) },
		func(i int, color ui.Color) {
			ui.Icon(c, themeChoices[i].icon).FontSize(iconSize).TextColor(color)
		},
		func(i int) string { return themeChoices[i].label })
}

// settingRow 画一项设置：左边是名称，右边是 control 画的控件。
func settingRow(c *ui.Context, name string, control func()) {
	ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
		ui.Text(c, name).Grow(1).MinWidth(0)
		control()
	})
}

// fontPreview 画一段界面文字与一段代码，用的就是当前选的字体。
func fontPreview(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(6).Padding(12, 14).Radius(boxRound).Background(boxColor(t)).Children(func() {
		ui.Text(c, "今天想做点什么？ The quick brown fox")
		ui.Text(c, `fmt.Println("你好，Atlas", 0123456789)`).
			Font(codeFont.Family).FontSize(codeSize)
	})
}
