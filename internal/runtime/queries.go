package runtime

import (
	"context"
	"errors"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/session"
)

func infoOf(info session.Info) Info {
	return Info{ID: info.ID, Name: info.Name, Cwd: info.Cwd, ModelRef: info.ModelRef,
		ThinkingLevel: info.ThinkingLevel, CreatedAt: info.CreatedAt, UpdatedAt: info.UpdatedAt, LastSeq: info.LastSeq}
}

// List 查询持久化会话，无需加载执行器或凭据。limit 为 0 时使用默认 50 条。
func (m *Manager) List(ctx context.Context, cwd string, before *ListCursor, limit int) ([]Info, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	var cursor *session.ListCursor
	if before != nil {
		cursor = &session.ListCursor{UpdatedAt: before.UpdatedAt, ID: before.ID}
	}
	infos, err := m.store.List(ctx, cwd, cursor, limit)
	if err != nil {
		return nil, err
	}
	result := make([]Info, len(infos))
	for i, info := range infos {
		result[i] = infoOf(info)
	}
	return result, nil
}

// Get 查询持久化元信息；运行中的状态使用 Session.State。
func (m *Manager) Get(ctx context.Context, id string) (Info, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return Info{}, err
	}
	defer end()
	info, err := m.store.Get(ctx, id)
	if err != nil {
		return Info{}, err
	}
	return infoOf(info), nil
}

// History 返回完整记录，包括提示词快照；beforeSeq 为 0 时取最新一页。
func (m *Manager) History(ctx context.Context, id string, beforeSeq int64, limit int) ([]Record, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	records, err := m.store.Messages(ctx, id, beforeSeq, limit)
	if err != nil {
		return nil, err
	}
	result := make([]Record, len(records))
	for i, record := range records {
		result[i] = Record{Seq: record.Seq, Message: record.Message, ModelRef: record.ModelRef, ThinkingLevel: record.ThinkingLevel}
	}
	return result, nil
}

// Usage 是会话累计 token 消耗，不是当前上下文长度。
func (m *Manager) Usage(ctx context.Context, id string) (agent.Usage, error) {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return agent.Usage{}, err
	}
	defer end()
	return m.store.Usage(ctx, id)
}

// lockMutation 用占位实例排除未加载会话的恢复与其他修改，不持有全局锁执行 I/O。
func (m *Manager) lockMutation(id string) (*Session, func(), error) {
	s, created, err := m.hold(id)
	if err != nil {
		return nil, nil, err
	}
	return s, func() {
		if created {
			m.unregister(s)
		}
		s.finish()
	}, nil
}

// Rename 修改空闲会话名称；空名称清除名称，不要求模型配置可用。
func (m *Manager) Rename(ctx context.Context, id, name string) error {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	s, unlock, err := m.lockMutation(id)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.store.Rename(ctx, id, name); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			m.forget(s)
		}
		return err
	}
	s.mu.Lock()
	s.state.Name = name
	s.mu.Unlock()
	return nil
}

// Delete 删除空闲会话；成功后，已经返回的 Session 引用也不能继续执行。
func (m *Manager) Delete(ctx context.Context, id string) error {
	ctx, end, err := m.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	s, unlock, err := m.lockMutation(id)
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.store.Delete(ctx, id); err != nil {
		// 记录已不存在时结果确定；其他错误无法确认是否已提交，要求重新初始化。
		if errors.Is(err, session.ErrNotFound) {
			m.forget(s)
		} else {
			s.invalidate()
		}
		return err
	}
	m.forget(s)
	return nil
}

// forget 将实例标记为已删除并移出注册表；已返回的引用此后不能再执行。
func (m *Manager) forget(s *Session) {
	s.mu.Lock()
	s.state.Deleted = true
	s.mu.Unlock()
	m.unregister(s)
}
