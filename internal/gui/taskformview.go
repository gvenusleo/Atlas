package gui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/schedule"
)

// taskFormWidth 是新建与编辑任务的对话框的宽度。
const taskFormWidth = 560

// taskFormDialog 在新建或编辑任务时画对话框：标题与关闭，表单，底部是错误与取消、保存；
// 内容多时表单在框内滚动。对话框自己关掉（按 Esc、点遮罩）时丢弃表单。
func (a *App) taskFormDialog(c *ui.Context, t *ui.Theme) {
	p := &a.tasks
	open := p.form != nil
	width, height := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		sheetPanel(backdrop, panel, t, min(taskFormWidth, width-2*gutter)).
			MaxHeight(height - 2*gutter).Clip()
		ui.Column(c).Grow(1).MinHeight(0).Padding(settingsTop, 0, 0).Children(func() {
			a.taskFormHeader(c, t)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { a.taskFormFields(c, t) })
			a.taskFormButtons(c, t)
		})
	})
	if !open {
		p.form = nil
	}
}

// taskFormHeader 画任务表单的标题栏。
func (a *App) taskFormHeader(c *ui.Context, t *ui.Theme) {
	title := "新建定时任务"
	if a.tasks.form.id != "" {
		title = "编辑定时任务"
	}
	dialogHeader(c, t, title, func() { a.tasks.form = nil })
}

// taskFormFields 画表单：每项左边是名称，右边是控件，与设置页一致。
func (a *App) taskFormFields(c *ui.Context, t *ui.Theme) {
	f := a.tasks.form
	ui.Column(c).Gap(settingsGap).Padding(settingsGap, settingsPad, settingsPad).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "名称")
			fieldBox(ui.TextInputBase(c.Key("task-name"), &f.name).Padding(5, 8).
				Placeholder("例如：每日整理待办"), t)
		})
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "指令")
			fieldBox(ui.TextAreaBase(c.Key("task-prompt"), &f.prompt).Lines(4, 10).
				Padding(8, 10).Placeholder("到点发给 Agent 的内容"), t)
		})
		formRow(c, "工作目录", func() { a.taskDirButton(c, t, f) })
		a.triggerRows(c, t, f)
		a.taskOption(c, t, "模型", protocol.ConfigModel, &f.model)
		a.taskOption(c, t, "思考深度", protocol.ConfigThinking, &f.thinking)
	})
}

// triggerRows 画触发方式：一行里先是规则种类的下拉，后面跟着这个种类的参数：间隔的分钟数、
// 每周的星期几（也是下拉）、日期与时刻。各个控件同高。
func (a *App) triggerRows(c *ui.Context, t *ui.Theme, f *taskForm) {
	formRow(c, "触发方式", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			pillSelect(c, t, "触发方式", &f.kind, kindChoices, kindName)
			switch ruleKinds[f.kind].kind {
			case schedule.Interval:
				fieldBox(ui.TextInputBase(c.Key("task-every"), &f.every).Width(64).
					Height(fieldHeight).Padding(0, 8), t)
				ui.Text(c, "分钟").TextColor(t.TextMuted)
			case schedule.Daily:
				timeField(c, f)
			case schedule.Weekly:
				pillSelect(c, t, "星期", &f.weekday, weekOrder, weekdayName)
				timeField(c, f)
			case schedule.Once:
				dateField(c, t, f)
				timeField(c, f)
			}
		})
	})
}

// timeField 画触发的时刻。
func timeField(c *ui.Context, f *taskForm) {
	ui.TimeInput(c.Key("task-at"), &f.at).Label("时刻").Height(fieldHeight)
}

// kindChoices 是规则种类下拉的各项，值是 ruleKinds 里的下标。
var kindChoices = func() []int {
	choices := make([]int, len(ruleKinds))
	for i := range choices {
		choices[i] = i
	}
	return choices
}()

func kindName(i int) string { return ruleKinds[i].label }

// dateField 画一次性任务的日期：外观与时刻一样，点开是月历，选了一天就收起。不用控件自带的
// DateInput，它带着一个日历图标，去不掉。
func dateField(c *ui.Context, t *ui.Theme, f *taskForm) {
	trigger := fieldBox(ui.ButtonBase(c.Key("task-date")).AlignItems(ui.Center).
		Height(fieldHeight).Padding(0, 10).Label("日期"), t)
	if trigger.Hovered() {
		trigger.Background(rowFill(t))
	}
	trigger.OnClick(func() { f.dateOpen = !f.dateOpen })
	trigger.Children(func() {
		ui.Text(c, f.at.Format("2006/01/02")).FontSize(textSize).SingleLine()
	})
	ui.PopoverBase(c, trigger, &f.dateOpen, func(panel ui.Element) {
		popupPanel(panel, t).Margin(6, 0, 0, 0)
		if ui.Calendar(c, &f.at).Changed() {
			f.dateOpen = false
		}
	})
}

// taskDirButton 画工作目录：点开选择目录的对话框；与表单里的输入框同高。
func (a *App) taskDirButton(c *ui.Context, t *ui.Theme, f *taskForm) {
	btn := ui.ButtonBase(c).AlignItems(ui.Center).Gap(6).Height(fieldHeight).Padding(0, 10).
		Radius(controlRound).Background(rowFill(t)).Label("选择工作目录")
	btn.OnClick(func() { a.pickTaskDir(f) })
	btn.Children(func() {
		ui.Icon(c, folderIcon).FontSize(smallIconSize).TextColor(t.TextMuted)
		text := "选择…"
		if f.cwd != "" {
			text = shortPath(f.cwd)
		}
		ui.Text(c, text).FontSize(textSize).SingleLine()
	})
}

// pickTaskDir 让用户为任务选择工作目录。
func (a *App) pickTaskDir(f *taskForm) {
	options := mygo.OpenDialogOptions{
		Parent: a.win, Title: "选择工作目录", DefaultPath: f.cwd, Directory: true,
	}
	go func() {
		paths, err := mygo.Dialog.Open(options)
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { f.cwd = paths[0] })
	}()
}

// taskChoices 是任务的模型或思考深度（配置项 id）可选的值与各值的显示名：选项取自当前会话的
// 配置项，第一项“默认”对应空值，即新会话自己的选择。任务指定的值 value 不在当前列表里
// （比如换过配置）时也照样列出，显示它本身。任务面板与表单用它显示得一样。
func (a *App) taskChoices(id, value string) ([]string, map[string]string) {
	values, names := []string{""}, map[string]string{"": "默认"}
	if opt, ok := a.current.option(id); ok {
		for _, v := range opt.Options {
			values = append(values, v.Value)
			names[v.Value] = v.Name
		}
	}
	if _, ok := names[value]; !ok {
		values = append(values, value)
		names[value] = value
	}
	return values, names
}

// taskOption 画模型或思考深度的下拉，选项见 taskChoices。
func (a *App) taskOption(c *ui.Context, t *ui.Theme, label, id string, value *string) {
	values, names := a.taskChoices(id, *value)
	formRow(c, label, func() {
		dropdown(c, t, "任务的"+label, value, values, func(v string) string { return names[v] },
			false, func() {})
	})
}

// taskFormButtons 画任务表单的底部。
func (a *App) taskFormButtons(c *ui.Context, t *ui.Theme) {
	dialogButtons(c, t, a.tasks.form.err, nil, func() { a.tasks.form = nil }, a.saveForm)
}
