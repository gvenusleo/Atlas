package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/gvenusleo/atlas/internal/session"
)

var (
	ErrBusy        = errors.New("会话正在执行操作")
	ErrNeedsReload = errors.New("会话状态不可用，需要重新初始化")
	ErrClosed      = errors.New("runtime 已关闭")
	ErrDeleted     = errors.New("会话已删除")
	ErrNotFound    = session.ErrNotFound
	ErrConflict    = session.ErrConflict
)

// Manager 拥有数据库与所有运行实例；一个数据库在进程内只使用一个 Manager。
type Manager struct {
	store      *session.Store
	mu         sync.Mutex
	sessions   map[string]*Session
	ctx        context.Context
	cancel     context.CancelFunc
	operations sync.WaitGroup
	closing    bool
	closed     chan struct{}
	closeErr   error
}

// Open 打开宿主；空路径使用 ~/.atlas/atlas.db。此处不加载凭据或调用模型。
func Open(ctx context.Context, databasePath string) (*Manager, error) {
	store, err := session.Open(ctx, databasePath)
	if err != nil {
		return nil, err
	}
	hostCtx, cancel := context.WithCancel(context.Background())
	return &Manager{store: store, sessions: make(map[string]*Session), ctx: hostCtx, cancel: cancel, closed: make(chan struct{})}, nil
}

// begin 登记操作。登记与关闭门禁共用锁，避免 Wait 开始后再增加操作。
func (m *Manager) begin(ctx context.Context) (context.Context, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return nil, nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	m.operations.Add(1)
	opCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	var once sync.Once
	return opCtx, func() { once.Do(func() { stop(); cancel(); m.operations.Done() }) }, nil
}

// Close 停止接收操作，取消并等待所有操作收尾后关闭数据库。
// 等待超时不终止后台收尾；后续 Close 可继续等待并读取相同结果。
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closing {
		m.closing = true
		m.cancel()
		go func() {
			m.operations.Wait()
			err := m.store.Close()
			m.mu.Lock()
			m.closeErr = err
			close(m.closed)
			m.mu.Unlock()
		}()
	}
	m.mu.Unlock()
	select {
	case <-m.closed:
		return m.closeErr
	default:
	}
	select {
	case <-m.closed:
		return m.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
