package gui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/schedule"
)

// 这个文件是定时任务在界面里的状态与逻辑：存取、到点触发、在后台建任务会话并发出用户消息、
// 任务会话的列表。判定哪一刻该触发在 schedule 里，视图的绘制在 tasksview.go。

// checkEvery 是检查有没有任务到点的间隔；触发最多晚这么久。
const checkEvery = 30 * time.Second

// taskRun 是任务的一次触发在 chat 上留下的记录：触发时任务的样子、触发点，以及标题改过没有。
type taskRun struct {
	task   schedule.Task
	at     time.Time // 触发点；立即运行时是现在
	titled bool
}

// historyStep 是历史标签里每次多列出的条数。
const historyStep = 20

// tasksPage 是定时任务视图的状态。
type tasksPage struct {
	list []schedule.Task
	form *taskForm // 正在新建或编辑的任务；没有时不弹对话框
	tab  int       // 视图选中的标签：0 是任务，1 是历史

	deleteID   string
	deleteOpen bool

	// launched 是各任务最近一次触发的会话，用来判断上一次是否还在运行；
	// errs 是各任务最近一次触发失败的原因，只在内存里。
	launched map[string]*chat
	errs     map[string]string

	// detail 是任务面板里打开的任务，空表示在任务列表。
	detail string

	// history 是已读进来的任务会话，按更新时间倒序：historyOwner 不为空时只有这个任务的，
	// 否则是全部任务的。historyNext 是接着读的游标，空表示读完了；historyShown 是列出的条数；
	// historyGen 让过时的读取结果作废。
	history      []protocol.SessionInfo
	historyOwner string
	historyNext  string
	historyShown int
	historyGen   int
}

// loadTasks 读取配置目录下的定时任务；读不了或文件损坏时没有任务，原因弹出提示。
func (a *App) loadTasks() {
	a.tasks = tasksPage{
		launched: map[string]*chat{}, errs: map[string]string{}, historyShown: historyStep,
	}
	dir, err := config.Dir()
	if err == nil {
		a.tasks.list, err = schedule.Load(dir)
	}
	if err != nil {
		a.toast = err.Error()
	}
}

// saveTasks 保存定时任务；失败只提示，任务在本次运行里仍有效。文件很小，在主线程同步写。
func (a *App) saveTasks() {
	dir, err := config.Dir()
	if err == nil {
		err = schedule.Save(dir, a.tasks.list)
	}
	if err != nil {
		a.toast = "保存定时任务失败：" + err.Error()
	}
}

// openTasks 把主区换成定时任务视图，先看到列表。
func (a *App) openTasks() {
	a.showTasks, a.tasks.form, a.tasks.detail = true, nil, ""
	a.scopeHistory("")
}

// openTask 打开任务的面板，历史换成这个任务的；closeTask 回到任务列表，历史换回全部任务的。
func (a *App) openTask(id string) {
	a.tasks.detail = id
	a.scopeHistory(id)
}

func (a *App) closeTask() {
	a.tasks.detail = ""
	a.scopeHistory("")
}

// scopeHistory 换一个范围重读历史：owner 是任务的标识，为空时是全部任务。
func (a *App) scopeHistory(owner string) {
	p := &a.tasks
	p.historyOwner, p.history, p.historyNext, p.historyShown = owner, nil, "", historyStep
	a.refreshHistory()
}

// tasksNeedLook 报告有没有任务会话运行结束而还没看过（出错的也算）：侧栏的入口据此画状态点。
func (a *App) tasksNeedLook() bool {
	for _, ch := range a.chats {
		if ch.job != nil && ch.unread {
			return true
		}
	}
	return false
}

// refreshHistory 重新从头读取任务会话的第一页，历史据此更新；已经读进来的更早的几页丢掉，
// 需要时再点“显示更多”。
func (a *App) refreshHistory() {
	a.readHistory("", func(resp protocol.ListSessionsResponse) {
		a.tasks.history = resp.Sessions
		a.tasks.historyNext = resp.NextCursor
	})
}

// loadMoreHistory 接着读更早的一页，接在已读进来的后面。
func (a *App) loadMoreHistory() {
	p := &a.tasks
	if p.historyNext == "" {
		return
	}
	a.readHistory(p.historyNext, func(resp protocol.ListSessionsResponse) {
		p.history = append(p.history, resp.Sessions...)
		p.historyNext = resp.NextCursor
	})
}

// readHistory 在后台读一页任务会话，成功时在主线程调用 apply；读的时候又有新的读取开始，
// 这次的结果就作废。
func (a *App) readHistory(cursor string, apply func(protocol.ListSessionsResponse)) {
	p := &a.tasks
	p.historyGen++
	gen := p.historyGen
	req := protocol.ListSessionsRequest{
		Type: protocol.SessionScheduled, OwnerID: p.historyOwner, Cursor: cursor}
	go func() {
		resp, err := a.service.ListSessions(context.Background(), req)
		a.win.Update(func() {
			switch {
			case gen != p.historyGen:
			case err != nil:
				a.toast = "读取任务会话失败：" + err.Error()
			default:
				apply(resp)
			}
		})
	}()
}

// showMoreHistory 多列出一些历史；已经读进来的不够时接着往前读。
func (a *App) showMoreHistory() {
	p := &a.tasks
	p.historyShown += historyStep
	if p.historyShown > len(p.history) {
		a.loadMoreHistory()
	}
}

// watchTasks 开始定期检查有没有任务到点，窗口创建之后调用一次；启动时先检查一次，
// 刚错过一会儿的触发点也能赶上。
func (a *App) watchTasks() {
	a.win.Update(a.checkTasks)
	go func() {
		for range time.Tick(checkEvery) {
			a.win.Update(a.checkTasks)
		}
	}()
}

// checkTasks 触发到点的任务。触发点先记为已处理再触发，同一个触发点不会处理两次。
func (a *App) checkTasks() {
	p, now := &a.tasks, time.Now()
	changed := false
	for i := range p.list {
		task := &p.list[i]
		if at, fire := task.Due(now); fire {
			task.Done(at)
			changed = true
			a.runTask(*task, at)
		}
	}
	if changed {
		a.saveTasks()
	}
}

// runTask 为任务的一次触发在后台建一个任务会话并发出用户消息，不切换当前会话。
// 上一次触发的会话还在运行时跳过这一次。
func (a *App) runTask(task schedule.Task, at time.Time) {
	p := &a.tasks
	if prev := p.launched[task.ID]; prev != nil && prev.inFlight() {
		p.errs[task.ID] = "上一次还在运行，已跳过这一次"
		return
	}
	delete(p.errs, task.ID)
	a.keys++
	ch := &chat{key: "chat-" + strconv.Itoa(a.keys), cwd: task.Cwd,
		job: &taskRun{task: task, at: at}}
	p.launched[task.ID] = ch
	a.start(ch)
}

// inFlight 报告会话还没结束：正在运行，或者还在建。
func (ch *chat) inFlight() bool {
	return ch.running || ch.id == "" && ch.err == ""
}

// sendTask 在任务会话建好之后换上任务指定的模型与思考深度，再发出用户消息；任何一步失败，
// 这次触发作废。
func (a *App) sendTask(ch *chat) {
	task, id := ch.job.task, ch.id
	go func() {
		ctx := context.Background()
		err := a.configureTask(ctx, id, task)
		if err == nil {
			_, err = a.service.SendPrompt(ctx, protocol.SendPromptRequest{
				SessionID: id, Prompt: []protocol.ContentBlock{protocol.Text(task.Prompt)}})
		}
		if err != nil {
			a.win.Update(func() {
				a.failTask(ch, err)
				a.discard(ch)
			})
		}
	}()
}

// configureTask 把会话的模型与思考深度换成任务指定的；任务没指定的保持默认。
func (a *App) configureTask(ctx context.Context, id string, task schedule.Task) error {
	for _, option := range []struct{ id, name, value string }{
		{protocol.ConfigModel, "模型", task.Model},
		{protocol.ConfigThinking, "思考深度", task.Thinking},
	} {
		if option.value == "" {
			continue
		}
		_, err := a.service.SetConfigOption(ctx, protocol.SetConfigOptionRequest{
			SessionID: id, ConfigID: option.id, Value: option.value})
		if err != nil {
			return fmt.Errorf("设置%s失败：%w", option.name, err)
		}
	}
	return nil
}

// failTask 记下这个会话对应的触发失败的原因，显示在任务上。
func (a *App) failTask(ch *chat, err error) {
	a.tasks.errs[ch.job.task.ID] = err.Error()
}

// titleTask 在任务会话保存了第一条消息（标题随之更新）之后，把标题改成触发的日期时间：
// 每次触发的用户消息一样，自动取的标题分不出来；任务名由视图里的分组给出。只改一次。
func (a *App) titleTask(ch *chat) {
	job := ch.job
	if job == nil || job.titled {
		return
	}
	job.titled = true
	id, title := ch.id, job.at.Format("01-02 15:04")
	go func() {
		err := a.service.RenameSession(context.Background(),
			protocol.RenameSessionRequest{SessionID: id, Title: title})
		a.win.Update(func() {
			if err != nil {
				a.toast = "任务会话改标题失败：" + err.Error()
			}
			a.refreshHistory()
		})
	}()
}

// setEnabled 启用或停用任务；重新启用的任务从现在起算，停用期间的触发点不算错过。
func (a *App) setEnabled(id string, on bool) {
	i := slices.IndexFunc(a.tasks.list, func(t schedule.Task) bool { return t.ID == id })
	if i < 0 {
		return
	}
	task := &a.tasks.list[i]
	task.Enabled = on
	if on {
		task.Created, task.LastRun = time.Now(), time.Time{}
	}
	a.saveTasks()
}

// removeTask 删除任务和它的全部任务会话：先删会话，都删掉了才把任务从清单里去掉，这样会话
// 删不掉时（核心拒绝删正在运行的，比如用户正在里面聊）任务还在，会话不会变成找不到主人的孤儿。
// 任务最近一次触发的会话还在运行时直接不删。
func (a *App) removeTask(id string) {
	p := &a.tasks
	if prev := p.launched[id]; prev != nil && prev.inFlight() {
		a.toast = "任务正在运行，等这一轮结束再删除"
		return
	}
	go func() {
		ids, err := a.deleteRuns(context.Background(), id)
		a.win.Update(func() {
			for _, id := range ids {
				a.dropChat(id)
			}
			defer a.refreshHistory()
			if err != nil {
				a.toast = "没有删除任务，它的任务会话没删干净：" + err.Error()
				return
			}
			p.list = slices.DeleteFunc(p.list, func(t schedule.Task) bool { return t.ID == id })
			if p.detail == id {
				a.closeTask() // 删的是面板里打开的任务
			}
			delete(p.launched, id)
			delete(p.errs, id)
			a.saveTasks()
		})
	}()
}

// deleteRuns 逐页读出任务的全部任务会话并删除，返回已删除的会话；出错时停下。taskID 不能为空：
// 列表里空的属主不筛选，会读出所有任务的会话。
func (a *App) deleteRuns(ctx context.Context, taskID string) (deleted []string, err error) {
	if taskID == "" {
		return nil, errors.New("缺少任务标识")
	}
	req := protocol.ListSessionsRequest{Type: protocol.SessionScheduled, OwnerID: taskID}
	for {
		resp, err := a.service.ListSessions(ctx, req)
		if err != nil {
			return deleted, err
		}
		for _, info := range resp.Sessions {
			del := protocol.DeleteSessionRequest{SessionID: info.SessionID}
			if err := a.service.DeleteSession(ctx, del); err != nil {
				return deleted, err
			}
			deleted = append(deleted, info.SessionID)
		}
		// 已删的不会再出现在列表里，从头再读，直到读空。
		if len(resp.Sessions) == 0 {
			return deleted, nil
		}
	}
}

// ruleKinds 是规则选择的各项，次序与表单上的一致。
var ruleKinds = []struct {
	kind  schedule.Kind
	label string
}{
	{schedule.Interval, "间隔"}, {schedule.Daily, "每天"},
	{schedule.Weekly, "每周"}, {schedule.Once, "一次"},
}

// taskForm 是新建与编辑任务的表单。
type taskForm struct {
	id                string // 空表示新建
	name, prompt, cwd string
	kind              int          // ruleKinds 里选中的一项
	every             string       // 间隔的分钟数，保存时才解析
	at                time.Time    // 每天、每周的时刻；一次的日期与时刻
	weekday           time.Weekday // 每周的星期几，界面一次选一天
	model, thinking   string       // 空表示默认
	err               string
	dateOpen          bool // 日历是不是展开着
}

// newTaskForm 是新建任务的表单：每天早上 9 点，在默认的工作目录。
func newTaskForm(cwd string, now time.Time) *taskForm {
	return &taskForm{
		cwd: cwd, kind: 1, every: "60", weekday: time.Monday,
		at: time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location()),
	}
}

// formOf 是编辑 task 的表单。
func formOf(task schedule.Task, now time.Time) *taskForm {
	f := newTaskForm(task.Cwd, now)
	f.id, f.name, f.prompt = task.ID, task.Name, task.Prompt
	f.model, f.thinking = task.Model, task.Thinking
	for i, k := range ruleKinds {
		if k.kind == task.Rule.Kind {
			f.kind = i
		}
	}
	r := task.Rule
	switch r.Kind {
	case schedule.Interval:
		f.every = strconv.Itoa(r.Every)
	case schedule.Daily, schedule.Weekly:
		if tm, err := time.Parse("15:04", r.At); err == nil {
			f.at = time.Date(now.Year(), now.Month(), now.Day(), tm.Hour(), tm.Minute(), 0, 0,
				now.Location())
		}
		f.weekday = r.Day
	case schedule.Once:
		f.at = r.Time
	}
	return f
}

// task 由表单得到任务，不合法时返回原因。规则从保存的这一刻起算：创建时刻记为现在，
// 之前的触发点都不再算。
func (f *taskForm) task(now time.Time) (schedule.Task, error) {
	task := schedule.Task{
		ID: f.id, Name: strings.TrimSpace(f.name), Prompt: strings.TrimSpace(f.prompt),
		Cwd: f.cwd, Model: f.model, Thinking: f.thinking, Enabled: true, Created: now,
	}
	if task.ID == "" {
		task.ID = uuid.NewV7().String()
	}
	rule := schedule.Rule{Kind: ruleKinds[f.kind].kind}
	switch rule.Kind {
	case schedule.Interval:
		every, err := strconv.Atoi(strings.TrimSpace(f.every))
		if err != nil {
			return schedule.Task{}, errors.New("间隔要填整数的分钟数")
		}
		rule.Every = every
	case schedule.Daily:
		rule.At = f.at.Format("15:04")
	case schedule.Weekly:
		rule.At = f.at.Format("15:04")
		rule.Day = f.weekday
	case schedule.Once:
		rule.Time = f.at.Truncate(time.Minute)
		if !rule.Time.After(now) {
			return schedule.Task{}, errors.New("触发的时间要晚于现在")
		}
	}
	task.Rule = rule
	return task, task.Validate()
}

// saveForm 保存表单：新建的加到列表末尾，编辑的换掉原来的（启用与否不变）。
func (a *App) saveForm() {
	p := &a.tasks
	task, err := p.form.task(time.Now())
	if err != nil {
		p.form.err = err.Error()
		return
	}
	same := func(old schedule.Task) bool { return old.ID == task.ID }
	if i := slices.IndexFunc(p.list, same); i >= 0 {
		task.Enabled = p.list[i].Enabled
		p.list[i] = task
	} else {
		p.list = append(p.list, task)
	}
	delete(p.errs, task.ID)
	p.form = nil
	a.saveTasks()
}
