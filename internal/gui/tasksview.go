package gui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/schedule"
)

// dayNames 按星期几（周日为 0）给出一个字的名称；weekOrder 是界面上从周一排起的次序。
var (
	dayNames  = []string{"日", "一", "二", "三", "四", "五", "六"}
	weekOrder = []time.Weekday{1, 2, 3, 4, 5, 6, 0}
)

// weekdayName 是下拉里星期几的名称。
func weekdayName(d time.Weekday) string { return "周" + dayNames[d] }

// taskTabs 是视图的两个标签，次序与 tasksPage.tab 一致。
var taskTabs = []string{"任务", "历史"}

// taskPanelInfoWidth 是任务面板右边一列（指令、工作目录等）的宽度。
const taskPanelInfoWidth = 260

// openedTask 是任务面板里打开的任务；没有打开，或任务已经不在了时 ok 为假。
func (a *App) openedTask() (task schedule.Task, ok bool) {
	p := &a.tasks
	i := slices.IndexFunc(p.list, func(t schedule.Task) bool { return t.ID == p.detail })
	if p.detail == "" || i < 0 {
		return schedule.Task{}, false
	}
	return p.list[i], true
}

// tasksView 画主区里的定时任务视图：留给窗口控件的标题栏，其下是任务列表与历史，或者
// 一个任务的面板；新建与编辑在对话框里（见 taskformview.go）。
func (a *App) tasksView(c *ui.Context, t *ui.Theme) {
	task, open := a.openedTask()
	ui.Column(c.Key("tasks")).Fill().Background(t.Background).Children(func() {
		a.tasksBar(c, t, open)
		if !open {
			column(c, 0, func() { a.tasksHeader(c, t) })
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			column(c, 0, func() {
				switch {
				case open:
					a.taskPanel(c, t, task)
				case a.tasks.tab == 0:
					a.taskCards(c, t)
				default:
					a.history(c, t)
				}
			})
		})
	})
}

// tasksBar 画标题栏：留给窗口控件，可以拖动窗口；任务面板里左边是面包屑“定时任务 / 任务名”。
func (a *App) tasksBar(c *ui.Context, t *ui.Theme, open bool) {
	bar := c.TitleBar()
	// 侧栏收起时，标题栏的内容让开悬浮的侧栏开关；随展开程度平滑过渡。
	left := gutter + (1-a.shown)*max(bar.Left+40-gutter, 0)
	ui.Row(c).Height(titleBarHeight(c)).AlignItems(ui.Center).
		Padding(0, bar.Right+barInset, 0, left).DragWindow().Children(func() {
		if !open {
			return
		}
		task, _ := a.openedTask()
		if a.crumbs(c, "定时任务", task.Name) == 0 {
			a.closeTask()
		}
	})
}

// runCrumbs 画从定时任务视图打开的对话的面包屑：“定时任务 / 任务名 / 标题”，从任务面板进来的才有
// 任务名。
func (a *App) runCrumbs(c *ui.Context, title string) {
	path := []string{"定时任务"}
	if task, ok := a.openedTask(); ok {
		path = append(path, task.Name)
	}
	path = append(path, cmp.Or(title, "任务会话"))
	switch a.crumbs(c, path...) {
	case 0:
		a.showTasks, a.fromTasks = true, false
		a.closeTask()
	case 1: // 只有三项时第 1 项才是链接：回到任务面板
		a.showTasks, a.fromTasks = true, false
		a.refreshHistory()
	}
}

// crumbs 画标题栏的面包屑，路径是 path；点了前面的某一项时返回它的序号，否则是 -1。
// 最后一项是现在所在的页面，不是链接。
func (a *App) crumbs(c *ui.Context, path ...string) int {
	if ui.Breadcrumbs(c, path, &a.crumb).Label("路径").Changed() {
		return a.crumb
	}
	return -1
}

// tasksHeader 画页面标题，其下左边是“任务”“历史”两个标签，右边是“新建任务”。
func (a *App) tasksHeader(c *ui.Context, t *ui.Theme) {
	p := &a.tasks
	ui.Column(c).Gap(16).Padding(12, 0, 0).Children(func() {
		ui.Text(c, "定时任务").FontSize(titleSize)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			segmented(c, t, "tasks-tab", "定时任务", &p.tab, len(taskTabs), 0, func() {},
				func(i int, color ui.Color) {
					ui.Text(c, taskTabs[i]).FontSize(textSize).TextColor(color)
				},
				func(i int) string { return taskTabs[i] })
			ui.Box(c).Grow(1)
			fillButton(c, t, plusIcon, "新建任务", func() {
				p.form = newTaskForm(a.cwd, time.Now())
			})
		})
	})
}

// fillButton 画本应用安静的按钮：淡底，悬停时深一点；icon 为空时只有文字。
func fillButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, label string, click func()) {
	btn := ui.ButtonBase(c).AlignItems(ui.Center).Gap(6).Height(28).Padding(0, 12).
		Radius(controlRound).Background(rowFill(t)).Label(label)
	if btn.Hovered() {
		btn.Background(buttonFill(t))
	}
	btn.OnClick(click)
	btn.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(smallIconSize).TextColor(t.TextMuted)
		}
		ui.Text(c, label).FontSize(textSize)
	})
}

// emptyState 画列表为空时的说明：图标、一句话与一句补充。
func emptyState(c *ui.Context, t *ui.Theme, title, hint string) {
	ui.Column(c).Padding(96, 0).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Icon(c, clockIcon).FontSize(emptyIconSize).TextColor(t.TextMuted)
		ui.Text(c, title).TextColor(t.TextMuted)
		ui.Text(c, hint).FontSize(smallSize).TextColor(t.TextMuted)
	})
}

// taskCards 画“任务”标签：一个任务一张卡片；没有任务时是一句说明。
func (a *App) taskCards(c *ui.Context, t *ui.Theme) {
	p := &a.tasks
	if len(p.list) == 0 {
		emptyState(c, t, "还没有定时任务", "到点自动新建会话，发出你写好的消息")
		return
	}
	now := time.Now()
	ui.Column(c).Gap(10).Padding(16, 0, 32).Children(func() {
		for _, task := range p.list {
			a.taskCard(c, t, task, now)
		}
	})
}

// taskCard 画一个任务：名称，用户消息的开头，状态、规则与工作目录，错过或失败时再加一行提示，
// 行尾是菜单。点卡片进入任务面板；停用的任务名称变淡。
func (a *App) taskCard(c *ui.Context, t *ui.Theme, task schedule.Task, now time.Time) {
	name := t.Text
	if !task.Enabled {
		name = t.TextMuted
	}
	card := ui.ButtonBase(c.Key(task.ID)).Justify(ui.Start).AlignItems(ui.Start).Gap(8).
		Padding(12, 14).Radius(boxRound).Border(1, t.Text.Alpha(0.1)).Label(task.Name)
	if card.Hovered() {
		card.Background(rowFill(t))
	}
	card.OnClick(func() { a.openTask(task.ID) })
	card.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, task.Name).FontSize(bodySize).TextColor(name).SingleLine()
			first, _, _ := strings.Cut(strings.TrimSpace(task.Prompt), "\n")
			ui.Text(c, first).FontSize(textSize).TextColor(t.TextMuted).SingleLine()
			info := strings.Join([]string{
				taskState(task), ruleText(task.Rule, now), shortPath(task.Cwd)}, " · ")
			ui.Text(c, info).FontSize(smallSize).TextColor(t.TextMuted).SingleLine()
			if text, color := a.taskAlert(t, task, now); text != "" {
				ui.Text(c, text).FontSize(smallSize).TextColor(color).SingleLine()
			}
		})
		a.taskMenu(c, t, task, false)
	})
}

// taskMenu 画 ⋯ 按钮与它的菜单：启用或停用、删除；卡片上还有立即运行与编辑，任务面板里它们
// 是页面上的按钮，panel 为真时菜单里没有。
func (a *App) taskMenu(c *ui.Context, t *ui.Theme, task schedule.Task, panel bool) {
	p := &a.tasks
	more := ui.ButtonBase(c.Key("more-"+task.ID)).Size(moreSize, moreSize).Shrink(0).
		Radius(controlRound).Label("更多")
	if panel {
		// 任务面板上与旁边的按钮同高同底色。
		more.Size(28, 28).Background(rowFill(t))
	}
	if more.Hovered() {
		more.Background(buttonFill(t))
	}
	more.Menu(func(m *ui.Menu) {
		if !panel {
			if m.Item("立即运行").Chosen() {
				a.runTask(task, time.Now())
			}
			if m.Item("编辑").Chosen() {
				p.form = formOf(task, time.Now())
			}
		}
		toggle := "停用"
		if !task.Enabled {
			toggle = "启用"
		}
		if m.Item(toggle).Chosen() {
			a.setEnabled(task.ID, !task.Enabled)
		}
		m.Separator()
		if m.Item("删除").Chosen() {
			p.deleteID, p.deleteOpen = task.ID, true
		}
	})
	more.Children(func() { ui.Icon(c, moreIcon).FontSize(iconSize).TextColor(t.TextMuted) })
}

// taskPanel 画一个任务自己的一页：任务名、状态与下次触发，右上角是立即运行、编辑与菜单；
// 其下左边是这个任务的历史记录，右边是指令、工作目录、触发方式、模型与思考深度。
func (a *App) taskPanel(c *ui.Context, t *ui.Theme, task schedule.Task) {
	now := time.Now()
	ui.Column(c).Gap(16).Padding(12, 0, 32).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, task.Name).FontSize(titleSize).SingleLine().Grow(1).MinWidth(0)
				fillButton(c, t, nil, "立即运行", func() { a.runTask(task, time.Now()) })
				fillButton(c, t, nil, "编辑", func() { a.tasks.form = formOf(task, time.Now()) })
				a.taskMenu(c, t, task, true)
			})
			ui.Text(c, taskSummary(task, now)).FontSize(textSize).TextColor(t.TextMuted)
			if text, color := a.taskAlert(t, task, now); text != "" {
				ui.Text(c, text).FontSize(textSize).TextColor(color)
			}
		})
		ui.Box(c).Height(1).Background(t.Text.Alpha(0.08))
		ui.Row(c).Gap(40).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
				ui.Text(c, "历史记录").FontSize(bodySize).Bold().Margin(0, 8)
				if len(a.tasks.history) == 0 {
					ui.Text(c, "还没有运行记录").FontSize(textSize).TextColor(t.TextMuted)
					return
				}
				a.historyRows(c, t, false)
			})
			a.taskInfo(c, t, task, now)
		})
	})
}

// taskInfo 画任务面板右边一列：每项一个小标题，其下是内容，项与项之间只靠间距分开。
func (a *App) taskInfo(c *ui.Context, t *ui.Theme, task schedule.Task, now time.Time) {
	section := func(title, text string) {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(textSize).Bold()
			ui.Text(c, text).FontSize(textSize).TextColor(t.TextMuted).Selectable()
		})
	}
	ui.Column(c).Width(taskPanelInfoWidth).Shrink(0).Gap(20).Children(func() {
		section("指令", task.Prompt)
		section("工作目录", shortPath(task.Cwd))
		section("触发方式", ruleText(task.Rule, now))
		_, models := a.taskChoices(protocol.ConfigModel, task.Model)
		_, thinking := a.taskChoices(protocol.ConfigThinking, task.Thinking)
		section("模型", models[task.Model])
		section("思考深度", thinking[task.Thinking])
	})
}

// history 画“历史”标签：全部任务会话；没有时是一句说明。
func (a *App) history(c *ui.Context, t *ui.Theme) {
	if len(a.tasks.history) == 0 {
		emptyState(c, t, "还没有运行记录", "任务到点运行之后，会话会出现在这里")
		return
	}
	ui.Column(c).Padding(16, 0, 32).Children(func() { a.historyRows(c, t, true) })
}

// historyRows 画已读进来的任务会话，新的在前，默认只列最近的 historyShown 个；showTask 为真时
// 每行带着任务名，任务面板里是同一个任务的，不带。
func (a *App) historyRows(c *ui.Context, t *ui.Theme, showTask bool) {
	p := &a.tasks
	shown := p.history[:min(len(p.history), p.historyShown)]
	ui.Column(c).Gap(2).Children(func() {
		for _, info := range shown {
			a.historyRow(c, t, info, showTask)
		}
		if hidden := len(p.history) - len(shown); hidden > 0 || p.historyNext != "" {
			showMore(c, t, hidden, rowFill(t), a.showMoreHistory)
		}
	})
}

// historyRow 画一条任务会话，样式与侧栏的会话行一致：任务名与触发的日期时间，行尾是相对时间、
// 运行中的指示器或状态点，悬停时是 ⋯ 菜单。点一下在主区打开它。
func (a *App) historyRow(c *ui.Context, t *ui.Theme, info protocol.SessionInfo, showTask bool) {
	when := cmp.Or(info.Title, "任务会话")
	ch := a.chats[info.SessionID]
	row := ui.ButtonBase(c.Key(info.SessionID)).Justify(ui.Start).AlignItems(ui.Center).Gap(8).
		Padding(4, 8).Radius(controlRound).Label(when)
	hovered := row.Hovered()
	if hovered {
		row.Background(rowFill(t))
	}
	row.OnClick(func() { a.openRun(info) })
	menu := func(m *ui.Menu) {
		if m.Item("删除").Chosen() {
			a.deleteID, a.deleteOpen = info.SessionID, true
		}
	}
	row.Children(func() {
		if showTask {
			ui.Text(c, a.taskName(info.OwnerID)).FontSize(textSize).SingleLine()
			ui.Text(c, when).FontSize(smallSize).TextColor(t.TextMuted).SingleLine().Grow(1).
				MinWidth(0)
		} else {
			ui.Text(c, when).FontSize(textSize).SingleLine().Grow(1).MinWidth(0)
		}
		trail(c, t, info.UpdatedAt, ch, hovered, menu)
	})
}

// taskName 是任务会话所属任务的名称；任务已经不在了（删除时没删干净）时说明这一点。
func (a *App) taskName(id string) string {
	i := slices.IndexFunc(a.tasks.list, func(t schedule.Task) bool { return t.ID == id })
	if i < 0 {
		return "已删除的任务"
	}
	return a.tasks.list[i].Name
}

// taskState 是任务的状态：已启用、已停用，一次性的任务触发之后是已完成。
func taskState(task schedule.Task) string {
	switch {
	case task.Enabled:
		return "已启用"
	case task.Rule.Kind == schedule.Once && !task.LastRun.IsZero():
		return "已完成"
	}
	return "已停用"
}

// taskSummary 是任务面板标题下的一句：状态，已启用的再加下次触发。
func taskSummary(task schedule.Task, now time.Time) string {
	state := taskState(task)
	if next, ok := task.Next(now); ok && task.Enabled {
		return state + " · 下次运行：" + whenText(next, now)
	}
	return state
}

// taskAlert 是需要提醒的一句：上次触发失败的原因，或者错过的触发点；没有时为空。
func (a *App) taskAlert(t *ui.Theme, task schedule.Task, now time.Time) (string, ui.Color) {
	if reason := a.tasks.errs[task.ID]; reason != "" {
		return "触发失败：" + reason, t.Danger
	}
	if at, fire := task.Due(now); !at.IsZero() && !fire {
		return "错过了 " + whenText(at, now), t.Warning
	}
	return "", t.TextMuted
}

// ruleText 用一句话说规则。
func ruleText(r schedule.Rule, now time.Time) string {
	switch r.Kind {
	case schedule.Interval:
		if r.Every%60 == 0 {
			return "每隔 " + strconv.Itoa(r.Every/60) + " 小时"
		}
		return "每隔 " + strconv.Itoa(r.Every) + " 分钟"
	case schedule.Daily:
		return "每天 " + r.At
	case schedule.Weekly:
		return "每周" + dayNames[r.Day] + " " + r.At
	case schedule.Once:
		return "仅一次 " + whenText(r.Time, now)
	}
	return ""
}

// whenText 把时刻说成今天、明天，或月日加时分。
func whenText(at, now time.Time) string {
	clock := at.Format("15:04")
	switch {
	case sameDay(at, now):
		return "今天 " + clock
	case sameDay(at, now.AddDate(0, 0, 1)):
		return "明天 " + clock
	}
	return at.Format("01-02 ") + clock
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// taskDialogs 画定时任务视图的确认框：删除任务。
func (a *App) taskDialogs(c *ui.Context) {
	p := &a.tasks
	const message = "删除后无法恢复，它的任务会话也会一起删除。"
	if ui.AlertDialog(c, &p.deleteOpen, "删除这个定时任务？", message, "取消", "删除") == 1 {
		a.removeTask(p.deleteID)
	}
}
