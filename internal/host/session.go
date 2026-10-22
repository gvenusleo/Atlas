package host

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/prompt"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
	"github.com/gvenusleo/atlas/internal/tools"
)

// Session 的互斥由 state.Busy 表示：mu 只保护状态与取消信号，不跨操作持有。
// Busy 期间，持有者独占 loop 与 seq；seq 是本实例已知的最后一条消息序号。
type Session struct {
	manager *Manager
	id      string
	mu      sync.Mutex
	loop    agent.Input
	skills  []skills.Skill
	seq     int64
	state   State
	done    chan struct{}
	cancel  context.CancelFunc
}

// ID 返回持久化会话标识，与 Manager 的查询和恢复接口使用同一值。
func (s *Session) ID() string { return s.id }

// State 返回运行期间也可读取的独立快照。模型、思考深度、提示词与技能目录取自 loop 与 skills，
// 它们是会话配置的唯一来源，所以只在持有 mu 时修改。
func (s *Session) State() State {
	s.mu.Lock()
	state := s.state
	state.ModelRef, state.ThinkingLevel = s.loop.Model.Ref, s.loop.Thinking
	state.SystemPrompt, state.Skills = s.loop.Prompt, slices.Clone(s.skills)
	s.mu.Unlock()
	s.manager.mu.Lock()
	state.Closed = s.manager.closing
	s.manager.mu.Unlock()
	return state
}

// start 登记独占操作；成功后调用方必须调用 finish。
func (s *Session) start(allowReload bool, cancel context.CancelFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Busy {
		return ErrBusy
	}
	if s.state.Deleted {
		return ErrDeleted
	}
	if s.state.NeedsReload && !allowReload {
		return ErrNeedsReload
	}
	s.state.Busy = true
	s.state.Running = cancel != nil
	s.done = make(chan struct{})
	s.cancel = cancel
	return nil
}

func (s *Session) finish() {
	s.mu.Lock()
	s.state.Busy, s.state.Running = false, false
	s.cancel = nil
	close(s.done)
	s.done = nil
	s.mu.Unlock()
}

func (s *Session) invalidate() {
	s.mu.Lock()
	s.state.NeedsReload = true
	s.mu.Unlock()
}

// hold 取出已加载实例或注册占位实例并独占它；created 表示实例是新注册的。
func (m *Manager) hold(id string) (s *Session, created bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s = m.sessions[id]
	created = s == nil
	if created {
		s = &Session{manager: m, id: id, state: State{ID: id}}
	}
	if err := s.start(true, nil); err != nil {
		return nil, false, err
	}
	m.sessions[id] = s
	return s, created, nil
}

// unregister 仅当注册表中仍是该实例时移除它。
func (m *Manager) unregister(s *Session) {
	m.mu.Lock()
	if m.sessions[s.id] == s {
		delete(m.sessions, s.id)
	}
	m.mu.Unlock()
}

// New 建一个新会话：加载配置与资源，选好模型，但还只在内存里（State.Draft），不写库。
// 第一条消息保存时才把它连同提示词快照写进库（见 save），所以没发过消息的新会话不会在库里
// 留下空记录；之前可以用 SetModel 换模型、用 SetCwd 换工作目录。模型选不出或配不起来也照常
// 建好，见 chooseModel。
func (m *Manager) New(ctx context.Context, cwd string) (*Session, error) {
	_, end, err := m.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
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
	ref, thinking, model := chooseModel(settings, "", "")
	id := uuid.NewV7().String()
	s := &Session{manager: m, id: id, state: State{ID: id, Draft: true}}
	s.bind(store.Info{ID: id, Cwd: cwd, Model: ref, Thinking: thinking}, nil, model, resources)
	return s, nil
}

// SetCwd 给草稿会话换工作目录：工具、提示词与技能目录都按新目录重建。已经保存过的会话不能换。
func (s *Session) SetCwd(ctx context.Context, cwd string) error {
	_, end, err := s.manager.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if err := s.start(false, nil); err != nil {
		return err
	}
	defer s.finish()
	if !s.state.Draft {
		return ErrNotDraft
	}
	if err := checkWorkdir(cwd); err != nil {
		return err
	}
	resources, err := prompt.Build(cwd)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.loop.Prompt, s.skills = resources.SystemPrompt, resources.Skills
	s.loop.Tools = toolset(cwd)
	s.state.Cwd = cwd
	s.mu.Unlock()
	return nil
}

// persistDraft 在草稿会话保存第一条消息前把它写进库：会话表一行，加提示词快照，并登记实例。
func (s *Session) persistDraft(ctx context.Context) error {
	if !s.state.Draft {
		return nil
	}
	snapshot := agent.SystemMessage{
		Type: agent.SystemMessageTypePrompt, Content: s.loop.Prompt, Timestamp: time.Now(),
	}
	info, err := s.manager.store.Create(ctx, store.Info{
		ID: s.id, Cwd: s.state.Cwd, Model: s.loop.Model.Ref, Thinking: s.loop.Thinking,
	}, snapshot)
	if err != nil {
		return err
	}
	s.seq = info.LastSeq
	s.manager.mu.Lock()
	s.manager.sessions[s.id] = s
	s.manager.mu.Unlock()
	s.mu.Lock()
	s.state.Draft = false
	s.mu.Unlock()
	return nil
}

// Resume 每次在同一实例上重新初始化，慢操作不持有 Manager 全局锁。
func (m *Manager) Resume(ctx context.Context, id string) (*Session, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	s, created, err := m.hold(id)
	if err != nil {
		return nil, err
	}
	s.invalidate()
	defer func() {
		if created && s.State().NeedsReload {
			m.unregister(s)
		}
		s.finish()
	}()
	info, records, err := m.store.Log(ctx, id)
	if err != nil {
		return nil, err
	}
	var messages []agent.Message
	var lastPrompt string
	for _, r := range records {
		if p, ok := r.Message.(agent.SystemMessage); ok && p.Type == agent.SystemMessageTypePrompt {
			lastPrompt = p.Content
		}
		messages = agent.Fold(messages, r.Message)
	}
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	// 上次用的模型失效了就退到别的（见 chooseModel），变了就存进会话表；一个可用的都没有时
	// 保留原来的。
	ref, thinking, model := chooseModel(settings, info.Model, info.Thinking)
	switch {
	case ref == "":
		ref, thinking = info.Model, info.Thinking
	case ref != info.Model || thinking != info.Thinking:
		if err := m.store.SetModel(ctx, id, ref, thinking); err != nil {
			return nil, err
		}
	}
	info.Model, info.Thinking = ref, thinking
	if err := checkWorkdir(info.Cwd); err != nil {
		return nil, err
	}
	resources, err := prompt.Build(info.Cwd)
	if err != nil {
		return nil, err
	}
	if resources.SystemPrompt != lastPrompt {
		message := agent.SystemMessage{
			Type:      agent.SystemMessageTypePrompt,
			Content:   resources.SystemPrompt,
			Timestamp: time.Now(),
		}
		seq, err := m.store.Append(ctx, id, info.LastSeq, message)
		if err != nil {
			return nil, errors.Join(ErrNeedsReload, err)
		}
		info.LastSeq = seq
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.bind(info, messages, model, resources)
	return s, nil
}

// toolset 是会话的工具，都以工作目录 cwd 为根。
func toolset(cwd string) []agent.Tool {
	return []agent.Tool{tools.Read(cwd), tools.Write(cwd), tools.Edit(cwd), tools.Shell(cwd)}
}

// bind 发布前或在操作锁内更新配置；控制状态最后一次性发布。
func (s *Session) bind(
	info store.Info, messages []agent.Message, model agent.Model, resources prompt.Resources,
) {
	cwd := info.Cwd
	s.seq = info.LastSeq
	model.Ref = info.Model // 没有可用的模型时 model 是零值，引用仍记着原来的
	s.mu.Lock()
	s.loop = agent.Input{
		Model:    model,
		Thinking: info.Thinking,
		Prompt:   resources.SystemPrompt,
		Tools:    toolset(cwd),
		Messages: messages,
	}
	s.skills = resources.Skills
	s.state.ID, s.state.Cwd, s.state.Name = s.id, cwd, info.Name
	s.state.NeedsReload = false
	s.mu.Unlock()
}

// SetModel 在空闲时更新模型；保存成功后才发布新配置。
func (s *Session) SetModel(ctx context.Context, modelRef string, thinking agent.ThinkingLevel) error {
	ctx, end, err := s.manager.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if err := s.start(false, nil); err != nil {
		return err
	}
	defer s.finish()
	settings, err := config.Load()
	if err != nil {
		return err
	}
	model, err := modelConfig(settings, modelRef, thinking)
	if err != nil {
		return err
	}
	return s.setModel(ctx, modelRef, thinking, model)
}

// setModel 保存并采用已经配好的模型：先写会话表，保存成功后才改 loop。调用方须独占会话。
func (s *Session) setModel(
	ctx context.Context, modelRef string, thinking agent.ThinkingLevel, model agent.Model,
) error {
	if !s.state.Draft { // 草稿还不在库里，保存第一条消息时一并写入
		if err := s.manager.store.SetModel(ctx, s.id, modelRef, thinking); err != nil {
			s.invalidate()
			return errors.Join(ErrNeedsReload, err)
		}
	}
	s.mu.Lock()
	s.loop.Model, s.loop.Thinking = model, thinking
	s.mu.Unlock()
	return nil
}

// retryModel 在会话没有可用的模型时（恢复时选不出或配不起来）再试一次：配置可能已经改好了。
// 仍然不行就返回原因。调用方须独占会话。
func (s *Session) retryModel(ctx context.Context) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	ref, thinking, err := pickModel(settings, s.loop.Model.Ref, s.loop.Thinking)
	if err != nil {
		return err
	}
	model, err := modelConfig(settings, ref, thinking)
	if err != nil {
		return err
	}
	return s.setModel(ctx, ref, thinking, model)
}
