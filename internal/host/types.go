package host

import (
	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/session"
	"github.com/gvenusleo/atlas/internal/skills"
)

// Info 是持久化会话元信息，时间以 UTC 返回。
type Info = session.Info

// ListCursor 是上一页最后一项的位置；分页是实时视图，不提供跨页快照。
type ListCursor = session.ListCursor

// Record 保留完整历史，包括提示词快照；Seq 是消息分页游标。
type Record = session.Record

// State 是已加载实例的独立快照；历史通过 Manager.History 读取。
// Busy 覆盖初始化与修改；Running 仅表示一次 Run 尚未完成收尾。
type State struct {
	ID            string
	Name          string
	Cwd           string
	ModelRef      string
	ThinkingLevel agent.ThinkingLevel
	Busy          bool
	Running       bool
	NeedsReload   bool
	Deleted       bool
	Closed        bool
	SystemPrompt  string
	Skills        []skills.Skill
}
