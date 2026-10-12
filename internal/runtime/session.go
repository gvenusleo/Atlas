// Package runtime 创建和恢复会话，将消息持久化接入 agent 循环。
package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/prompt"
	"github.com/gvenusleo/atlas/internal/session"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/tools"
)

var (
	ErrBusy        = errors.New("会话正在执行操作")
	ErrNeedsReload = errors.New("会话保存失败，需要重新加载")
	ErrInterrupted = errors.New("会话有未完成的工具调用，需要确认处理")
)

// Manager 为一个应用进程共享，确保同一会话只持有一份运行状态。
// 同一个 Store 只应创建一个 Manager；数据库由调用方在所有运行结束后关闭。
type Manager struct {
	store    *session.Store
	mu       sync.Mutex
	sessions map[string]*Session
}

// New 创建进程内会话管理器，调用方负责共享该实例和关闭数据库。
func New(store *session.Store) *Manager {
	return &Manager{store: store, sessions: make(map[string]*Session)}
}

// Session 的历史和配置保持私有；所有修改都经过同一个操作锁。
type Session struct {
	store       *session.Store
	id          string
	mu          sync.Mutex
	loop        agent.LoopContext
	pending     []agent.ToolCall
	needsReload bool
}

// ID 返回用于查询和恢复的持久化会话标识。
func (s *Session) ID() string { return s.id }

// Create 加载当前配置和提示词，保存新会话并注册工具。
func (m *Manager) Create(ctx context.Context, cwd string) (*Session, error) {
	settings, err := config.Load()
	if err != nil {
		return nil, err
	}
	modelID, stream, err := modelConfig(settings, settings.Model, "")
	if err != nil {
		return nil, err
	}
	resources, err := prompt.Build(cwd)
	if err != nil {
		return nil, err
	}
	info, err := m.store.Create(ctx, session.CreateOptions{Cwd: cwd, ModelRef: settings.Model, SystemPrompt: resources.SystemPrompt})
	if err != nil {
		return nil, err
	}
	s := m.bind(session.Loaded{Info: info, Messages: []agent.Message{
		agent.SystemMessage{Content: resources.SystemPrompt, Timestamp: info.CreatedAt},
	}}, modelID, stream, resources.Skills)
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s, nil
}

// Resume 在全局锁外加载历史和资源；同一会话恢复期间返回 ErrBusy，其他会话可继续操作。
func (m *Manager) Resume(ctx context.Context, id string) (*Session, error) {
	m.mu.Lock()
	if existing := m.sessions[id]; existing != nil {
		if !existing.mu.TryLock() {
			m.mu.Unlock()
			return nil, ErrBusy
		}
		needsReload := existing.needsReload
		existing.mu.Unlock()
		if !needsReload {
			m.mu.Unlock()
			return existing, nil
		}
	}
	// 占位实例不对外返回；恢复完成前，同一 ID 不能再次恢复或删除。
	loading := &Session{store: m.store, id: id, needsReload: true}
	loading.mu.Lock()
	m.sessions[id] = loading
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if m.sessions[id] == loading {
			delete(m.sessions, id)
		}
		m.mu.Unlock()
		loading.mu.Unlock()
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
	discoveredSkills, err := skills.Discover(loaded.Info.Cwd)
	if err != nil {
		return nil, err
	}
	s := m.bind(loaded, modelID, stream, discoveredSkills)
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

// bind 将恢复的历史与待处理调用绑定到新运行状态，工具以会话目录为基准。
func (m *Manager) bind(loaded session.Loaded, modelID string, stream agent.StreamFn, discoveredSkills []skills.Skill) *Session {
	cwd := loaded.Info.Cwd
	return &Session{
		store:   m.store,
		id:      loaded.Info.ID,
		pending: loaded.PendingCalls,
		loop: agent.LoopContext{
			Messages: loaded.Messages,
			Tools:    []agent.Tool{tools.Read(cwd), tools.Write(cwd), tools.Edit(cwd), tools.Shell(cwd)},
			Skills:   discoveredSkills,
			Model:    modelID, Stream: stream, ThinkingLevel: loaded.Info.ThinkingLevel,
		},
	}
}

// Run 逐条保存 message_end；保存失败立即取消，随后事件不再尝试写入。
// emit 同步调用，不能在其中重新进入本会话的运行或修改方法。
func (s *Session) Run(ctx context.Context, message agent.UserMessage, emit func(agent.Event)) ([]agent.Message, error) {
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if s.needsReload {
		return nil, ErrNeedsReload
	}
	if len(s.pending) != 0 {
		return nil, ErrInterrupted
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}
	// 在进入循环前校验消息，避免无效输入污染内存历史。
	if err := agent.ValidateMessage(message); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var saveErr error
	messages := agent.Run(runCtx, []agent.Message{message}, &s.loop, func(event agent.Event) {
		if event.Type == agent.EventTypeMessageEnd && saveErr == nil {
			// 取消后仍保存已完成的消息，避免工具结果因 run context 失效而丢失。
			saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_, saveErr = s.store.AppendMessage(saveCtx, s.id, event.Message)
			stop()
			if saveErr != nil {
				s.needsReload = true
				cancel()
			}
		}
		if emit != nil {
			emit(event)
		}
	})
	if saveErr != nil {
		return messages, errors.Join(ErrNeedsReload, saveErr)
	}
	if err := ctx.Err(); err != nil {
		return messages, err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if last, ok := messages[i].(agent.AssistantMessage); ok {
			switch last.StopReason {
			case agent.StopReasonError:
				return messages, fmt.Errorf("模型调用失败: %s", last.ErrorMessage)
			case agent.StopReasonCanceled:
				return messages, context.Canceled
			}
			break
		}
	}
	return messages, nil
}

// PendingCalls 返回独立快照供 UI 展示，不允许修改内存中的调用参数。
func (s *Session) PendingCalls() ([]agent.ToolCall, error) {
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	calls := make([]agent.ToolCall, len(s.pending))
	for i, call := range s.pending {
		call.Arguments = bytes.Clone(call.Arguments)
		calls[i] = call
	}
	return calls, nil
}

// ResolveInterrupted 只在用户明确确认后调用；补充未知结果，不重放工具调用。
func (s *Session) ResolveInterrupted(ctx context.Context) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if s.needsReload {
		return ErrNeedsReload
	}
	for _, call := range s.pending {
		message := agent.ToolResultMessage{ToolCallID: call.ID, ToolName: call.Name,
			IsError: true, Timestamp: time.Now(), Content: []agent.Content{agent.TextContent{
				Text: "上次运行中断，工具是否执行及其结果未知。恢复过程没有重新执行该调用。",
			}}}
		if _, err := s.store.AppendMessage(ctx, s.id, message); err != nil {
			s.needsReload = true
			return errors.Join(ErrNeedsReload, err)
		}
		s.loop.Messages = append(s.loop.Messages, message)
	}
	s.pending = nil
	return nil
}

// SetModel 仅在会话空闲且工具结果完整时切换模型，保存成功后更新内存配置。
func (s *Session) SetModel(ctx context.Context, modelRef string, thinkingLevel agent.ThinkingLevel) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if s.needsReload {
		return ErrNeedsReload
	}
	if len(s.pending) != 0 {
		return ErrInterrupted
	}
	settings, err := config.Load()
	if err != nil {
		return err
	}
	modelID, stream, err := modelConfig(settings, modelRef, thinkingLevel)
	if err != nil {
		return err
	}
	if err := s.store.UpdateSettings(ctx, s.id, modelRef, thinkingLevel); err != nil {
		s.needsReload = true
		return errors.Join(ErrNeedsReload, err)
	}
	s.loop.Model = modelID
	s.loop.Stream = stream
	s.loop.ThinkingLevel = thinkingLevel
	return nil
}

// Delete 排除运行中的会话；删除后旧实例也不能再执行。
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s != nil {
		if !s.mu.TryLock() {
			return ErrBusy
		}
		defer s.mu.Unlock()
	}
	if err := m.store.Delete(ctx, id); err != nil {
		if s != nil {
			s.needsReload = true
		}
		return err
	}
	if s != nil {
		s.needsReload = true
	}
	delete(m.sessions, id)
	return nil
}
