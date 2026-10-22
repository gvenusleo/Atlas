package core

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/prompt"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
	"github.com/gvenusleo/atlas/internal/tools"
)

// session 是一个加载了的会话。字段都由 mu 保护，锁只在读写字段时持有，从不跨 IO；
// 修改配置与写库只在持有 turn 时进行，所以同一时刻只有一个 goroutine 在改它们。
type session struct {
	id   string
	core *Core

	mu       sync.Mutex
	cwd      string
	saved    bool  // 已写进库；之前只在内存里
	seq      int64 // 本进程写入或读到的最后序号
	turn     *turn // 正在进行的操作；nil 表示空闲。这是唯一的控制状态
	model    agent.Model
	thinking agent.ThinkingLevel
	prompt   string // 系统提示词
	tools    []agent.Tool
	skills   []skills.Skill
	messages []agent.Message   // 上下文 = Fold(历史)
	clients  []protocol.Client // 打开着这个会话的连接
	ids      int               // 已分配的消息 ID 数
}

// turn 是会话上正在进行的操作。一轮对话或压缩有 cancel，可以取消，会通知客户端；
// 换模型、删除这类很快的操作没有。
type turn struct {
	cancel context.CancelFunc
	since  time.Time
}

// create 建一个新会话：选好模型，按工作目录组装提示词与工具，还只在内存里。
func (c *Core) create(cwd string) (*session, error) {
	if err := checkWorkdir(cwd); err != nil {
		return nil, err
	}
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	resources, err := prompt.Build(cwd)
	if err != nil {
		return nil, err
	}
	thinking, model := chooseModel(settings, "", "")
	s := &session{id: uuid.NewV7().String(), core: c, cwd: cwd, model: model, thinking: thinking,
		prompt: resources.SystemPrompt, tools: toolset(cwd), skills: resources.Skills}
	c.mu.Lock()
	c.sessions[s.id] = s
	c.mu.Unlock()
	return s, nil
}

// load 从库里加载会话：重放历史得到上下文；上次的模型失效了就退到别的并存下来；
// 提示词与最后一份快照不同时追加一份新快照。
func (c *Core) load(ctx context.Context, id string) (*session, error) {
	info, records, err := c.db.Log(ctx, id)
	if err != nil {
		return nil, err
	}
	messages, lastPrompt := replay(records)
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	thinking, model := chooseModel(settings, info.Model, info.Thinking)
	if model.Ref != info.Model || thinking != info.Thinking {
		if err := c.db.SetModel(ctx, id, model.Ref, thinking); err != nil {
			return nil, err
		}
	}
	if err := checkWorkdir(info.Cwd); err != nil {
		return nil, err
	}
	resources, err := prompt.Build(info.Cwd)
	if err != nil {
		return nil, err
	}
	seq := info.LastSeq
	if resources.SystemPrompt != lastPrompt {
		if seq, err = c.db.Append(ctx, id, seq, snapshot(resources.SystemPrompt)); err != nil {
			return nil, err
		}
	}
	return &session{id: id, core: c, cwd: info.Cwd, saved: true, seq: seq, model: model,
		thinking: thinking, prompt: resources.SystemPrompt, tools: toolset(info.Cwd),
		skills: resources.Skills, messages: messages}, nil
}

// replay 依次把历史 Fold 成上下文，并找出最后一份提示词快照。
func replay(records []store.Record) (messages []agent.Message, lastPrompt string) {
	for _, r := range records {
		if p, ok := r.Message.(agent.SystemMessage); ok && p.Type == agent.SystemMessageTypePrompt {
			lastPrompt = p.Content
		}
		messages = agent.Fold(messages, r.Message)
	}
	return messages, lastPrompt
}

// snapshot 是一份提示词快照。
func snapshot(content string) agent.SystemMessage {
	return agent.SystemMessage{
		Type: agent.SystemMessageTypePrompt, Content: content, Timestamp: time.Now(),
	}
}

// toolset 是会话的工具，都以工作目录 cwd 为根。
func toolset(cwd string) []agent.Tool {
	return []agent.Tool{tools.Read(cwd), tools.Write(cwd), tools.Edit(cwd), tools.Shell(cwd)}
}

// subscribe 让 client 收到这个会话的更新。
func (s *session) subscribe(client protocol.Client) {
	s.mu.Lock()
	if !slices.Contains(s.clients, client) {
		s.clients = append(s.clients, client)
	}
	s.mu.Unlock()
}

// unsubscribe 让 client 不再收到更新；没有人看、也不在运行的会话卸载掉。
func (s *session) unsubscribe(client protocol.Client) {
	s.mu.Lock()
	s.clients = slices.DeleteFunc(s.clients, func(c protocol.Client) bool { return c == client })
	idle := len(s.clients) == 0 && s.turn == nil
	s.mu.Unlock()
	if idle {
		s.core.forget(s)
	}
}

// broadcast 把更新推给每个打开着这个会话的连接。
func (s *session) broadcast(updates ...protocol.SessionUpdate) {
	s.mu.Lock()
	clients := slices.Clone(s.clients)
	s.mu.Unlock()
	for _, u := range updates {
		for _, c := range clients {
			c.SessionUpdate(protocol.SessionNotification{SessionID: s.id, Update: u})
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
	if s.turn != nil && s.turn.cancel != nil {
		state.Running, state.Since = true, s.turn.since
	}
	return state
}

// usage 是上下文的占用；调用方持有 mu。
func (s *session) usage() protocol.Usage {
	return protocol.Usage{Used: agent.Tokens(s.messages), Size: s.model.Window}
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

// begin 开始一个独占的操作；会话不空闲时返回 ErrBusy。cancel 不为空的是一轮对话或压缩。
// 成功后调用方必须调用 end。
func (s *session) begin(cancel context.CancelFunc) (end func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turn != nil {
		return nil, protocol.ErrBusy
	}
	s.turn = &turn{cancel: cancel, since: time.Now()}
	return func() {
		s.mu.Lock()
		s.turn = nil
		s.mu.Unlock()
	}, nil
}

// cancelTurn 取消正在运行的一轮；没有时什么也不做。
func (s *session) cancelTurn() {
	s.mu.Lock()
	if s.turn != nil && s.turn.cancel != nil {
		s.turn.cancel()
	}
	s.mu.Unlock()
}

// setConfig 换模型或思考深度：配好新模型，保存，再采用；返回完整的配置项，并推给其他连接。
func (s *session) setConfig(ctx context.Context, id, value string) ([]protocol.ConfigOption, error) {
	end, err := s.begin(nil)
	if err != nil {
		return nil, err
	}
	defer end()
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	ref, thinking, saved := s.model.Ref, s.thinking, s.saved
	s.mu.Unlock()
	switch id {
	case protocol.ConfigModel:
		ref = value
		models := listModels(settings)
		if i := slices.IndexFunc(models, func(m modelInfo) bool { return m.Ref == ref }); i >= 0 {
			thinking = fitLevel(models[i], thinking)
		}
	case protocol.ConfigThinking:
		thinking = agent.ThinkingLevel(value)
	default:
		return nil, errors.New("未知的配置项 " + id)
	}
	model, err := modelConfig(settings, ref, thinking)
	if err != nil {
		return nil, err
	}
	if saved {
		if err := s.core.db.SetModel(ctx, s.id, ref, thinking); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.model, s.thinking = model, thinking
	usage := s.usage()
	s.mu.Unlock()
	options := configOptions(settings, ref, thinking)
	s.broadcast(protocol.ConfigOptionUpdate{ConfigOptions: options}, protocol.UsageUpdate{Usage: usage})
	return options, nil
}

// options 读配置，给出会话现在的配置项。
func (s *session) options() ([]protocol.ConfigOption, error) {
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return configOptions(settings, s.model.Ref, s.thinking), nil
}
