package host

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/prompt"
	"github.com/gvenusleo/atlas/internal/session"
	"github.com/gvenusleo/atlas/internal/tools"
)

// Session 的互斥由 state.Busy 表示：mu 只保护状态与取消信号，不跨操作持有。
// Busy 期间，持有者独占 loop 与 seq；seq 是本实例已知的最后一条消息序号。
type Session struct {
	manager *Manager
	id      string
	mu      sync.Mutex
	loop    agent.LoopContext
	seq     int64
	state   State
	done    chan struct{}
	cancel  context.CancelFunc
}

// ID 返回持久化会话标识，与 Manager 的查询和恢复接口使用同一值。
func (s *Session) ID() string { return s.id }

// State 返回运行期间也可读取的独立快照。
func (s *Session) State() State {
	s.mu.Lock()
	state := s.state
	state.Skills = slices.Clone(state.Skills)
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

// Create 加载配置与资源，保存首个提示词快照并发布会话实例。
func (m *Manager) Create(ctx context.Context, cwd string) (*Session, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	if settings.Model == "" {
		return nil, fmt.Errorf("请在 ~/.atlas/settings.json 中配置 model")
	}
	_, _, metadata, err := settings.ModelRef(settings.Model)
	if err != nil {
		return nil, err
	}
	thinking := defaultThinkingLevel(metadata)
	modelID, stream, err := modelConfig(settings, settings.Model, thinking)
	if err != nil {
		return nil, err
	}
	resources, err := prompt.Build(cwd)
	if err != nil {
		return nil, err
	}
	id := uuid.NewV7().String()
	s := &Session{
		manager: m,
		id:      id,
		done:    make(chan struct{}),
		state:   State{ID: id, Busy: true, NeedsReload: true},
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	defer func() {
		if s.State().NeedsReload {
			m.unregister(s)
		}
		s.finish()
	}()
	info, err := m.store.Create(ctx, session.CreateOptions{
		ID:            id,
		Cwd:           cwd,
		ModelRef:      settings.Model,
		ThinkingLevel: thinking,
		SystemPrompt:  resources.SystemPrompt,
	})
	if err != nil {
		return nil, err
	}
	s.bind(session.Loaded{Info: info}, modelID, stream, resources)
	return s, nil
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
	loaded, err := m.store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	modelID, stream, err := modelConfig(settings, loaded.Info.ModelRef, loaded.Info.ThinkingLevel)
	if err != nil {
		return nil, err
	}
	if err := checkWorkdir(loaded.Info.Cwd); err != nil {
		return nil, err
	}
	resources, err := prompt.Build(loaded.Info.Cwd)
	if err != nil {
		return nil, err
	}
	if resources.SystemPrompt != loaded.SystemPrompt.Content {
		message := agent.SystemMessage{
			Type:      agent.SystemMessageTypePrompt,
			Content:   resources.SystemPrompt,
			Timestamp: time.Now(),
		}
		seq, err := m.store.AppendMessage(ctx, id, loaded.Info.LastSeq, message)
		if err != nil {
			return nil, errors.Join(ErrNeedsReload, err)
		}
		loaded.Info.LastSeq = seq
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.bind(loaded, modelID, stream, resources)
	return s, nil
}

// bind 发布前或在操作锁内更新配置；控制状态最后一次性发布。
func (s *Session) bind(
	loaded session.Loaded, modelID string, stream agent.StreamFn, resources prompt.Resources,
) {
	cwd := loaded.Info.Cwd
	info := loaded.Info
	toolset := []agent.Tool{tools.Read(cwd), tools.Write(cwd), tools.Edit(cwd), tools.Shell(cwd)}
	s.loop = agent.LoopContext{
		SystemPrompt:  resources.SystemPrompt,
		Messages:      loaded.Messages,
		Tools:         toolset,
		Skills:        resources.Skills,
		Model:         modelID,
		Stream:        stream,
		ThinkingLevel: info.ThinkingLevel,
	}
	s.seq = info.LastSeq
	s.mu.Lock()
	s.state.ID, s.state.Cwd, s.state.Name = s.id, cwd, info.Name
	s.state.ModelRef, s.state.ThinkingLevel = info.ModelRef, info.ThinkingLevel
	s.state.SystemPrompt, s.state.Skills = resources.SystemPrompt, slices.Clone(resources.Skills)
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
	modelID, stream, err := modelConfig(settings, modelRef, thinking)
	if err != nil {
		return err
	}
	if err := s.manager.store.UpdateSettings(ctx, s.id, modelRef, thinking); err != nil {
		s.invalidate()
		return errors.Join(ErrNeedsReload, err)
	}
	s.loop.Model, s.loop.Stream, s.loop.ThinkingLevel = modelID, stream, thinking
	s.mu.Lock()
	s.state.ModelRef, s.state.ThinkingLevel = modelRef, thinking
	s.mu.Unlock()
	return nil
}
