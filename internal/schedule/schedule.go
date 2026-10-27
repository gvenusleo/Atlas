// Package schedule 是定时任务的数据与触发判定：任务的类型、规则、最近与下一次触发的时刻，
// 以及 schedule.json 的存取（见 file.go）。它不读时钟，也不知道会话与核心，到点之后做什么
// 由调用方决定。
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Grace 是触发点之后仍然照常触发的时间：休眠唤醒、应用刚启动与定时器都会有误差，超过它的
// 触发点算错过，不补跑。
const Grace = 5 * time.Minute

// Kind 是规则的种类。
type Kind string

const (
	Interval Kind = "interval" // 每隔 Every 分钟，以任务的创建时刻为起点
	Daily    Kind = "daily"    // 每天的 At
	Weekly   Kind = "weekly"   // 每周 Day 的 At
	Once     Kind = "once"     // 仅在 Time 触发一次
)

// 每隔多少分钟的取值范围；更长的间隔用每天或每周表达。
const (
	MinEvery = 1
	MaxEvery = 24 * 60
)

// Rule 是任务什么时候触发，时间一律用调用方给的时区（本机时区）。
type Rule struct {
	Kind  Kind         `json:"kind"`
	Every int          `json:"every,omitzero"` // Interval：间隔的分钟数
	At    string       `json:"at,omitzero"`    // Daily、Weekly：时刻，如 "09:30"
	Day   time.Weekday `json:"day,omitzero"`   // Weekly：星期几，周日为 0（省略时也是周日）
	Time  time.Time    `json:"time,omitzero"`  // Once：触发的时刻
}

// Task 是一个定时任务：到点在 Cwd 新建一个会话，发出 Prompt。
type Task struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Prompt   string    `json:"prompt"`
	Cwd      string    `json:"cwd"`
	Model    string    `json:"model,omitzero"`    // 空表示新会话的默认选择
	Thinking string    `json:"thinking,omitzero"` // 同上
	Rule     Rule      `json:"rule"`
	Enabled  bool      `json:"enabled,omitzero"`
	Created  time.Time `json:"created"`
	// LastRun 是已处理的最近一个触发点（触发了，或因上一次还在运行而跳过），不是触发发生的
	// 时刻；它让同一个触发点不会处理两次。
	LastRun time.Time `json:"last_run,omitzero"`
}

// Validate 检查任务能不能保存：名称、用户消息与工作目录非空，规则完整。
func (t Task) Validate() error {
	switch {
	case strings.TrimSpace(t.Name) == "":
		return errors.New("名称不能为空")
	case strings.TrimSpace(t.Prompt) == "":
		return errors.New("用户消息不能为空")
	case t.Cwd == "":
		return errors.New("请选择工作目录")
	}
	return t.Rule.validate()
}

func (r Rule) validate() error {
	switch r.Kind {
	case Interval:
		if r.Every < MinEvery || r.Every > MaxEvery {
			return fmt.Errorf("间隔要在 %d 到 %d 分钟之间", MinEvery, MaxEvery)
		}
	case Daily, Weekly:
		if _, _, err := clock(r.At); err != nil {
			return err
		}
		if r.Kind == Weekly && (r.Day < time.Sunday || r.Day > time.Saturday) {
			return errors.New("星期几要在周日到周六之间")
		}
	case Once:
		if r.Time.IsZero() {
			return errors.New("请选择触发的时间")
		}
	default:
		return fmt.Errorf("不认识的规则 %q", r.Kind)
	}
	return nil
}

// clock 解析 "15:04" 形式的时刻。
func clock(s string) (hour, minute int, err error) {
	tm, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("时刻 %q 不是 时:分 的形式", s)
	}
	return tm.Hour(), tm.Minute(), nil
}

// Due 判断 now 时刻有没有要处理的触发点：at 是任务最近一个还没处理的触发点，没有时为零值；
// fire 为真表示它还在 Grace 之内，应当触发，否则它已经错过。停用的任务没有。
func (t Task) Due(now time.Time) (at time.Time, fire bool) {
	if !t.Enabled {
		return time.Time{}, false
	}
	at, ok := t.Latest(now)
	if !ok || !at.After(t.LastRun) {
		return time.Time{}, false
	}
	return at, now.Sub(at) <= Grace
}

// Done 记下触发点 at 已经处理；一次性的任务随之停用。
func (t *Task) Done(at time.Time) {
	t.LastRun = at
	if t.Rule.Kind == Once {
		t.Enabled = false
	}
}

// Latest 是不晚于 now 的最近一个触发点，要晚于任务的创建时刻；没有时 ok 为假。
func (t Task) Latest(now time.Time) (at time.Time, ok bool) {
	r := t.Rule
	switch r.Kind {
	case Interval:
		if r.Every <= 0 {
			return time.Time{}, false
		}
		step := time.Duration(r.Every) * time.Minute
		at = t.Created.Add(now.Sub(t.Created) / step * step)
	case Daily, Weekly:
		at, ok = r.latestAt(now)
		if !ok {
			return time.Time{}, false
		}
	case Once:
		at = r.Time
	default:
		return time.Time{}, false
	}
	return at, at.After(t.Created) && !at.After(now)
}

// Next 是晚于 now 的下一个触发点；没有时（停用的任务不在这里判断）ok 为假。
func (t Task) Next(now time.Time) (at time.Time, ok bool) {
	r := t.Rule
	switch r.Kind {
	case Interval:
		if r.Every <= 0 {
			return time.Time{}, false
		}
		step := time.Duration(r.Every) * time.Minute
		// 第一个触发点是创建之后的第一步，now 早于创建时刻时也如此。
		return t.Created.Add(max(now.Sub(t.Created)/step+1, 1) * step), true
	case Daily, Weekly:
		return r.nextAt(now)
	case Once:
		return r.Time, r.Time.After(now)
	}
	return time.Time{}, false
}

// latestAt 在每天或每周的规则里找不晚于 now 的最近一天的触发点。
func (r Rule) latestAt(now time.Time) (time.Time, bool) {
	hour, minute, err := clock(r.At)
	if err != nil {
		return time.Time{}, false
	}
	for back := range 8 {
		day := now.AddDate(0, 0, -back)
		at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
		if r.onDay(at) && !at.After(now) {
			return at, true
		}
	}
	return time.Time{}, false
}

// nextAt 在每天或每周的规则里找晚于 now 的最近一天的触发点。
func (r Rule) nextAt(now time.Time) (time.Time, bool) {
	hour, minute, err := clock(r.At)
	if err != nil {
		return time.Time{}, false
	}
	for ahead := range 8 {
		day := now.AddDate(0, 0, ahead)
		at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
		if r.onDay(at) && at.After(now) {
			return at, true
		}
	}
	return time.Time{}, false
}

// onDay 报告规则在 at 所在的这一天是否触发：每天的总是，每周的看星期几。
func (r Rule) onDay(at time.Time) bool {
	return r.Kind == Daily || at.Weekday() == r.Day
}
