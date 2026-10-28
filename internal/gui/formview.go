package gui

import "github.com/egoist/mygo/ui"

// 这个文件是表单对话框（新建任务、模型服务）共用的控件：输入框、一项一行的排版、下拉、标题栏
// 与底部按钮。

// 表单每一项的最小高度，表单里小控件的统一高度，以及服务表单里名称一列的宽度。
const (
	formRowHeight   = 32
	fieldHeight     = 28
	fieldLabelWidth = 84
)

// fieldBox 给输入框配上本应用的样式：淡的细边框与圆角，不用控件自带的强调色。
func fieldBox(input ui.Element, t *ui.Theme) ui.Element {
	return input.Radius(controlRound).Border(1, t.Text.Alpha(0.12))
}

// textField 画表单里的单行输入框，样式见 fieldBox。
func textField(c *ui.Context, t *ui.Theme, key string, value *string, hint string) ui.Element {
	return fieldBox(ui.TextInputBase(c.Key(key), value).Padding(5, 8).Placeholder(hint), t)
}

// formRow 画表单的一项：与设置页的项一样，左边是名称，右边是控件；但每项至少 formRowHeight
// 高，输入框、分段控件与小号的下拉排在一起，行距才匀。
func formRow(c *ui.Context, name string, control func()) {
	ui.Row(c).MinHeight(formRowHeight).AlignItems(ui.Center).Gap(16).Children(func() {
		ui.Text(c, name).Grow(1).MinWidth(0)
		control()
	})
}

// fieldRow 画表单的一项，排法与 formRow 不同：名称在左，定宽，各项的名称因此对齐成一列；
// 控件在右，由它自己决定宽度，输入框一般占满余下的。
func fieldRow(c *ui.Context, name string, control func()) {
	ui.Row(c).MinHeight(formRowHeight).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, name).Width(fieldLabelWidth).Shrink(0)
		control()
	})
}

// pillSelect 画表单里的下拉：触发器是淡底的小块，悬停时深一点，显示当前值与向下的箭头，
// 选项在其下展开。
func pillSelect[T comparable](
	c *ui.Context, t *ui.Theme, label string, value *T, options []T, name func(T) string,
) {
	sel := ui.SelectBase(c.Key(label), value)
	trigger := sel.Trigger.Label(label).AlignItems(ui.Center).Gap(6).Height(fieldHeight).
		Padding(0, 10).Radius(controlRound).Background(rowFill(t))
	if trigger.Hovered() {
		trigger.Background(buttonFill(t))
	}
	trigger.Children(func() {
		ui.Text(c, name(*value)).FontSize(textSize).SingleLine()
		ui.Icon(c, chevronIcon).FontSize(smallIconSize).TextColor(t.TextMuted).Rotate(90)
	})
	dropdownOptions(c, t, sel, value, options, name, textSize)
}

// dialogHeader 画对话框的标题与右上角的关闭，边距与设置页的角落一致。
func dialogHeader(c *ui.Context, t *ui.Theme, title string, closeDialog func()) {
	ui.Row(c).Height(barButton).AlignItems(ui.Center).Padding(0, 0, 0, settingsPad).
		Children(func() { ui.Text(c, title).FontSize(dialogSize).Bold() })
	quietButton(c, t, "关闭", closeDialog).
		Absolute().Top(settingsCloseTop).Right(settingsCloseRight).Size(barButton, barButton).
		Children(func() {
			ui.Icon(c, closeIcon).FontSize(barButton * 0.6).TextColor(t.TextMuted)
		})
}

// dialogButtons 画对话框底部：左边是 extra 画的内容（可为空），其后是校验的错误，右边是取消与
// 保存；保存用淡的强调色，不抢眼。
func dialogButtons(c *ui.Context, t *ui.Theme, err string, extra, cancel, save func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(12, settingsPad).Children(func() {
		if extra != nil {
			extra()
		}
		ui.Text(c, err).FontSize(textSize).TextColor(t.Danger).Grow(1).MinWidth(0)
		quietButton(c, t, "取消", cancel).Height(28).Padding(0, 14).
			Children(func() { ui.Text(c, "取消").FontSize(textSize) })
		btn := ui.ButtonBase(c).Height(28).Padding(0, 14).Radius(controlRound).Label("保存")
		alpha := float32(0.15)
		if btn.Hovered() {
			alpha = 0.25
		}
		btn.Background(accentFg(t).Alpha(alpha)).OnClick(save)
		btn.Children(func() { ui.Text(c, "保存").FontSize(textSize).TextColor(accentFg(t)) })
	})
}
