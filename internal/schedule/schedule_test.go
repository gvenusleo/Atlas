package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var loc = time.FixedZone("UTC+8", 8*3600)

func at(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, loc)
}

func daily(created time.Time) Task {
	return Task{Rule: Rule{Kind: Daily, At: "09:00"}, Enabled: true, Created: created}
}

func weekly(created time.Time, day time.Weekday) Task {
	return Task{Rule: Rule{Kind: Weekly, At: "09:00", Day: day}, Enabled: true, Created: created}
}

func every(minutes int, created time.Time) Task {
	return Task{Rule: Rule{Kind: Interval, Every: minutes}, Enabled: true, Created: created}
}

func once(when, created time.Time) Task {
	return Task{Rule: Rule{Kind: Once, Time: when}, Enabled: true, Created: created}
}

// 最近与下一个触发点：跨日、跨周、跨月，创建之前的不算，间隔从创建时刻起算。
func TestLatestAndNext(t *testing.T) {
	var none time.Time
	tests := []struct {
		name         string
		task         Task
		now          time.Time
		latest, next time.Time // 零值表示没有
	}{
		{"每天，今天的还没到", daily(at(10, 1, 12, 0)), at(10, 27, 8, 0),
			at(10, 26, 9, 0), at(10, 27, 9, 0)},
		{"每天，今天的已过", daily(at(10, 1, 12, 0)), at(10, 27, 9, 30),
			at(10, 27, 9, 0), at(10, 28, 9, 0)},
		{"每天，创建之前的触发点不算", daily(at(10, 27, 10, 0)), at(10, 27, 11, 0),
			none, at(10, 28, 9, 0)},
		{"每周一，跨周", weekly(at(10, 1, 0, 0), time.Monday), at(10, 30, 12, 0),
			at(10, 26, 9, 0), at(11, 2, 9, 0)},
		{"每周日，跨月", weekly(at(10, 1, 0, 0), time.Sunday), at(11, 4, 12, 0),
			at(11, 1, 9, 0), at(11, 8, 9, 0)},
		{"间隔，从创建起算", every(45, at(10, 27, 10, 0)), at(10, 27, 12, 0),
			at(10, 27, 11, 30), at(10, 27, 12, 15)},
		{"间隔，第一步之前没有", every(45, at(10, 27, 10, 0)), at(10, 27, 10, 20),
			none, at(10, 27, 10, 45)},
		{"一次，之前", once(at(11, 1, 8, 0), at(10, 27, 10, 0)), at(10, 27, 12, 0),
			none, at(11, 1, 8, 0)},
		{"一次，之后", once(at(11, 1, 8, 0), at(10, 27, 10, 0)), at(11, 1, 9, 0),
			at(11, 1, 8, 0), none},
	}
	for _, tt := range tests {
		latest, ok := tt.task.Latest(tt.now)
		if ok != !tt.latest.IsZero() || !latest.Equal(tt.latest) && ok {
			t.Errorf("%s：Latest = %v, %v，期望 %v", tt.name, latest, ok, tt.latest)
		}
		next, ok := tt.task.Next(tt.now)
		if ok != !tt.next.IsZero() || !next.Equal(tt.next) && ok {
			t.Errorf("%s：Next = %v, %v，期望 %v", tt.name, next, ok, tt.next)
		}
	}
}

// 每天 9 点跨过夏令时的那一天，仍然是当地的 9 点，而不是前一个触发点之后的 24 小时。
func TestDailyAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	task := daily(time.Date(2026, 3, 1, 0, 0, 0, 0, ny))
	next, ok := task.Next(time.Date(2026, 3, 7, 10, 0, 0, 0, ny))
	if want := time.Date(2026, 3, 8, 9, 0, 0, 0, ny); !ok || !next.Equal(want) {
		t.Errorf("夏令时开始那天的 Next = %v，期望 %v", next, want)
	}
	// 夏令时开始的那天只有 23 小时，临近午夜时加 24 小时会跳过这一天。
	next, ok = task.Next(time.Date(2026, 3, 7, 23, 30, 0, 0, ny))
	if want := time.Date(2026, 3, 8, 9, 0, 0, 0, ny); !ok || !next.Equal(want) {
		t.Errorf("夏令时前一天深夜的 Next = %v，期望 %v", next, want)
	}
	latest, ok := task.Latest(time.Date(2026, 3, 9, 10, 0, 0, 0, ny))
	if want := time.Date(2026, 3, 9, 9, 0, 0, 0, ny); !ok || !latest.Equal(want) {
		t.Errorf("夏令时之后的 Latest = %v，期望 %v", latest, want)
	}
}

// 触发判定：容忍内触发，超出算错过；处理过的触发点不再出现；停用的没有；一次性的触发后停用。
func TestDue(t *testing.T) {
	task := daily(at(10, 1, 12, 0))
	trigger := at(10, 27, 9, 0)

	if got, fire := task.Due(trigger.Add(Grace)); !got.Equal(trigger) || !fire {
		t.Errorf("容忍的边界上 = %v, %v，期望触发 %v", got, fire, trigger)
	}
	if got, fire := task.Due(trigger.Add(Grace + time.Second)); !got.Equal(trigger) || fire {
		t.Errorf("超出容忍 = %v, %v，期望错过 %v", got, fire, trigger)
	}

	task.Done(trigger)
	if got, _ := task.Due(trigger.Add(time.Minute)); !got.IsZero() {
		t.Errorf("处理过的触发点又出现了：%v", got)
	}

	task.Enabled = false
	if got, _ := task.Due(at(10, 28, 9, 1)); !got.IsZero() {
		t.Errorf("停用的任务有触发点：%v", got)
	}

	one := once(at(11, 1, 8, 0), at(10, 27, 10, 0))
	one.Done(at(11, 1, 8, 0))
	if one.Enabled {
		t.Error("一次性任务触发后应当停用")
	}
}

func TestFile(t *testing.T) {
	dir := t.TempDir()
	if tasks, err := Load(dir); tasks != nil || err != nil {
		t.Fatalf("文件不存在时 Load = %v, %v，期望没有任务也没有错误", tasks, err)
	}

	saved := []Task{{
		ID: "a", Name: "整理", Prompt: "整理 TODO", Cwd: "/work", Model: "p/m", Thinking: "high",
		Rule:    Rule{Kind: Weekly, At: "09:30", Day: time.Friday},
		Enabled: true, Created: at(10, 27, 8, 0), LastRun: at(10, 26, 9, 30),
	}, {
		// 周日是零值，写文件时省略，读回来仍是周日。
		ID: "b", Name: "周日", Prompt: "p", Cwd: "/work",
		Rule: Rule{Kind: Weekly, At: "09:00", Day: time.Sunday}, Created: at(10, 27, 8, 0),
	}}
	if err := Save(dir, saved); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || len(got) != 2 {
		t.Fatalf("Load = %v, %v", got, err)
	}
	g, s := got[0], saved[0]
	if g.ID != s.ID || g.Name != s.Name || g.Prompt != s.Prompt || g.Cwd != s.Cwd ||
		g.Model != s.Model || g.Thinking != s.Thinking || !g.Enabled ||
		g.Rule.Kind != s.Rule.Kind || g.Rule.At != s.Rule.At || g.Rule.Day != time.Friday ||
		!g.Created.Equal(s.Created) || !g.LastRun.Equal(s.LastRun) {
		t.Errorf("往返之后 = %+v，期望 %+v", g, s)
	}
	if got[1].Rule.Day != time.Sunday {
		t.Errorf("周日的任务读回来是 %v", got[1].Rule.Day)
	}
}

// 读不了的文件（权限、临时不可读）不是损坏：原样留在原处，不改名，返回读取的错误。
func TestUnreadableFileIsNotRenamed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, File)
	if err := Save(dir, []Task{{ID: "a", Name: "n", Prompt: "p", Cwd: "/"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	tasks, err := Load(dir)
	if tasks != nil || err == nil {
		t.Fatalf("Load = %v, %v，期望没有任务并给出错误", tasks, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("读不了的文件不应被改名：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, backup)); err == nil {
		t.Error("读不了的文件不应出现 .bad")
	}
}

// 损坏的文件改名保留，不能被之后的保存覆盖。
func TestCorruptFileIsKept(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte("{坏的"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := Load(dir)
	if tasks != nil || err == nil {
		t.Fatalf("Load = %v, %v，期望没有任务并给出错误", tasks, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, backup)); err != nil || string(data) != "{坏的" {
		t.Errorf("备份 = %q, %v，期望保留原内容", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, File)); err == nil {
		t.Error("损坏的文件还在原处")
	}
}
