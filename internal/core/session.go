package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
	"github.com/gvenusleo/atlas/internal/systemprompt"
	"github.com/gvenusleo/atlas/internal/tools"
)

// session 是一个加载了的会话。字段都由 mu 保护，锁只在读写字段时持有，从不跨 IO；
// 修改配置与写库只由占用会话（busy）的一方进行，所以同一时刻只有一个 goroutine 在修改它们。
type session struct {
	id   string
	core *Core

	mu           sync.Mutex
	cwd          string
	sessionType  protocol.SessionType
	ownerID      string // 会话的属主，见 protocol.NewSessionRequest
	saved        bool   // 已写进库；之前只在内存里
	seq          int64  // 本进程写入或读到的最后序号
	busy         *busy  // 占用会话的操作；nil 表示空闲。这是唯一的控制状态
	model        agent.Model
	thinking     agent.ThinkingLevel
	systemPrompt string
	tools        []agent.Tool
	skills       []skills.Skill
	messages     []agent.Message // 上下文 = Fold(历史)
	// seqs 是上下文里每条消息在历史中的序号，与 messages 一一对应；压缩时用它定出新的上下文起点。
	seqs        []int64
	subscribers []protocol.Subscriber // 订阅该会话的连接
	ids         int                   // 已分配的消息 ID 数
}

// busy 是占用会话的操作。一轮或压缩有 cancel，可以取消，会通知订阅者；
// 换模型、删除这类很快的操作没有。
type busy struct {
	cancel context.CancelFunc
	since  time.Time
}

// create 建一个新会话：选好模型，按工作目录组装系统提示词与工具，还只在内存里，也还没有登记。
func (c *Core) create(
	cwd string, sessionType protocol.SessionType, ownerID string,
) (*session, error) {
	if err := checkCwd(cwd); err != nil {
		return nil, err
	}
	sessionType, err := checkSessionType(sessionType, ownerID)
	if err != nil {
		return nil, err
	}
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	resources, err := systemprompt.Build(cwd)
	if err != nil {
		return nil, err
	}
	thinking, model := chooseModel(settings, "", "")
	return &session{id: uuid.NewV7().String(), core: c, cwd: cwd, sessionType: sessionType,
		ownerID: ownerID, model: model, thinking: thinking, systemPrompt: resources.SystemPrompt,
		tools: toolset(cwd), skills: resources.Skills}, nil
}

// checkSessionType 检查建会话请求里的类型与属主：类型缺省是普通会话；定时任务的会话必须有
// 属主，普通会话没有。
func checkSessionType(
	sessionType protocol.SessionType, ownerID string,
) (protocol.SessionType, error) {
	sessionType = cmp.Or(sessionType, protocol.SessionInteractive)
	switch {
	case sessionType != protocol.SessionInteractive && sessionType != protocol.SessionScheduled:
		return "", fmt.Errorf("不认识的会话类型 %q", sessionType)
	case sessionType == protocol.SessionScheduled && ownerID == "":
		return "", errors.New("定时任务的会话必须有属主")
	case sessionType == protocol.SessionInteractive && ownerID != "":
		return "", errors.New("普通会话没有属主")
	}
	return sessionType, nil
}

// load 从库里加载会话：读出上下文起点之后的记录，折叠得到上下文；上次的模型失效了就退到别的
// 并存下来；系统提示词与最后一份快照不同时追加一份新快照。
func (c *Core) load(ctx context.Context, id string) (*session, error) {
	info, records, err := c.db.Context(ctx, id)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if err := checkCwd(info.Cwd); err != nil {
		return nil, err
	}
	s := &session{id: id, core: c, cwd: info.Cwd, saved: true, seq: info.LastSeq,
		sessionType: protocol.SessionType(info.Type), ownerID: info.OwnerID}
	for _, r := range records {
		m, err := c.decodeWithImages(ctx, id, r)
		if err != nil {
			return nil, err
		}
		s.fold(m, r.Seq)
	}
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	thinking, model := chooseModel(settings, info.Model, agent.ThinkingLevel(info.Thinking))
	if model.Ref != info.Model || string(thinking) != info.Thinking {
		if err := c.db.SetModel(ctx, id, model.Ref, string(thinking)); err != nil {
			return nil, mapStoreError(err)
		}
	}
	resources, err := systemprompt.Build(info.Cwd)
	if err != nil {
		return nil, err
	}
	last, err := c.lastSystemPrompt(ctx, id)
	if err != nil {
		return nil, err
	}
	if resources.SystemPrompt != last {
		entry, err := encode(systemPromptSnapshot(resources.SystemPrompt))
		if err != nil {
			return nil, err
		}
		if s.seq, err = c.db.Append(ctx, id, s.seq, entry); err != nil {
			return nil, mapStoreError(err)
		}
	}
	s.model, s.thinking = model, thinking
	s.systemPrompt, s.tools, s.skills = resources.SystemPrompt, toolset(info.Cwd), resources.Skills
	return s, nil
}

// lastSystemPrompt 取历史里最后一份系统提示词快照的内容，没有时为空。
func (c *Core) lastSystemPrompt(ctx context.Context, id string) (string, error) {
	r, err := c.db.Latest(ctx, id, kindSystemPrompt)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	m, err := decode(id, r)
	if err != nil {
		return "", err
	}
	return m.(agent.SystemMessage).Content, nil
}

// fold 把一条已保存的记录并进上下文，seq 是它的序号。调用方保证独占 s。
func (s *session) fold(m agent.Message, seq int64) {
	if sm, ok := m.(agent.SystemMessage); ok && sm.Type == agent.SystemMessageTypeCompaction {
		kept := min(sm.Kept, len(s.seqs))
		s.seqs = append([]int64{seq}, s.seqs[len(s.seqs)-kept:]...)
		s.messages = agent.Fold(s.messages, m)
		return
	}
	before := len(s.messages)
	s.messages = agent.Fold(s.messages, m)
	if len(s.messages) > before {
		s.seqs = append(s.seqs, seq)
	}
}

// contextFrom 给出压缩记录 m 追加之后的上下文起点：保留段里第一条消息的序号；
// 没有保留的消息时，是这条压缩记录自己的序号。调用方持有 mu。
func (s *session) contextFrom(m agent.SystemMessage) int64 {
	kept := min(m.Kept, len(s.seqs))
	if kept == 0 {
		return s.seq + 1
	}
	return s.seqs[len(s.seqs)-kept]
}

// systemPromptSnapshot 是一份系统提示词快照。
func systemPromptSnapshot(content string) agent.SystemMessage {
	return agent.SystemMessage{
		Type: agent.SystemMessageTypeSystemPrompt, Content: content, Timestamp: time.Now(),
	}
}

// toolset 是会话的工具，都以工作目录 cwd 为根。
func toolset(cwd string) []agent.Tool {
	return []agent.Tool{tools.Read(cwd), tools.Write(cwd), tools.Edit(cwd), tools.Shell(cwd)}
}

// subscribe 让 sub 收到这个会话的更新。
func (s *session) subscribe(sub protocol.Subscriber) {
	s.mu.Lock()
	if !slices.Contains(s.subscribers, sub) {
		s.subscribers = append(s.subscribers, sub)
	}
	s.mu.Unlock()
}

// unsubscribe 让 sub 不再收到更新。
func (s *session) unsubscribe(sub protocol.Subscriber) {
	s.mu.Lock()
	s.subscribers = slices.DeleteFunc(s.subscribers, func(c protocol.Subscriber) bool {
		return c == sub
	})
	s.mu.Unlock()
}

// subscribed 报告 sub 是否订阅着这个会话。
func (s *session) subscribed(sub protocol.Subscriber) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.subscribers, sub)
}

// idle 报告会话是否没有订阅者、也不忙。
func (s *session) idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subscribers) == 0 && s.busy == nil
}

// broadcast 把更新推送给每个订阅该会话的连接。
func (s *session) broadcast(updates ...protocol.Update) {
	s.mu.Lock()
	subs := slices.Clone(s.subscribers)
	s.mu.Unlock()
	for _, u := range updates {
		for _, sub := range subs {
			sub.Receive(protocol.Notification{SessionID: s.id, Update: u})
		}
	}
}

// newID 分配一个消息 ID。
func (s *session) newID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids++
	return "m" + strconv.Itoa(s.ids)
}

// state 是会话现在的状态，打开会话时随响应给出。
func (s *session) state(title string) protocol.SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := protocol.SessionState{Cwd: s.cwd, Title: title, Commands: commands(s.skills),
		Usage: s.usage()}
	if s.busy != nil && s.busy.cancel != nil {
		state.Running, state.Since = true, s.busy.since
	}
	return state
}

// usage 是上下文的占用；调用方持有 mu。
func (s *session) usage() protocol.Usage {
	return protocol.Usage{Used: agent.Tokens(s.messages), ContextWindow: s.model.ContextWindow}
}

// commands 是可用的斜杠命令：/compact，加上会话的技能。
func commands(list []skills.Skill) []protocol.Command {
	commands := []protocol.Command{{Name: "compact", Description: "把较早的对话压缩成摘要"}}
	for _, s := range list {
		commands = append(commands, protocol.Command{
			Name: s.Name, Description: s.Description, Input: &protocol.CommandInput{Hint: "参数"},
		})
	}
	return commands
}

// begin 开始一个独占的操作；会话不空闲时返回 ErrBusy。cancel 不为空的是一轮或压缩。
// 成功后调用方必须调用 end。
func (s *session) begin(cancel context.CancelFunc) (end func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy != nil {
		return nil, protocol.ErrBusy
	}
	s.busy = &busy{cancel: cancel, since: time.Now()}
	return func() {
		s.mu.Lock()
		s.busy = nil
		s.mu.Unlock()
	}, nil
}

// cancelTurn 取消正在运行的一轮；没有时什么也不做。
func (s *session) cancelTurn() {
	s.mu.Lock()
	if s.busy != nil && s.busy.cancel != nil {
		s.busy.cancel()
	}
	s.mu.Unlock()
}
