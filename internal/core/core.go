// Package core 是 Atlas 的核心，ACP 形态的提供端：管理本进程加载的会话，是会话运行时数据的
// 唯一权威。GUI、ACP、WebSocket 都通过 Connect 拿到的 protocol.Agent 使用它。
//
// 历史只在一条消息定稿时写库；会话的上下文在写库之后用 agent.Fold 推进，所以始终等于库里的历史
// 折叠出来的结果。会话的控制状态只有正在进行的操作（session.turn）。多个进程可以打开同一个库，
// 同一会话的并发写入由序号检查发现：这一轮停止，下一轮开始前从库里重新加载。
package core

import (
	"context"
	"sync"

	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

// Core 是一个进程里的核心，一个数据库只用一个。
type Core struct {
	db     *store.DB
	ctx    context.Context // Close 时取消，所有一轮都随之取消
	cancel context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*session // 本进程加载的会话
	closing  bool
	ops      sync.WaitGroup // 进行中的请求，Close 等它们结束再关库
	loading  sync.Mutex     // 加载串行进行，同一会话不会被加载两次
}

// New 用打开的数据库建核心。
func New(db *store.DB) *Core {
	ctx, cancel := context.WithCancel(context.Background())
	return &Core{db: db, ctx: ctx, cancel: cancel, sessions: make(map[string]*session)}
}

// Connect 为一个客户端建连接：它打开的会话的更新都推给 client。
func (c *Core) Connect(client protocol.Client) protocol.Agent {
	return &conn{core: c, client: client}
}

// Close 不再接收请求，取消所有正在运行的一轮，等请求都结束后关库。ctx 到期时不再等，
// 返回它的错误，数据库留着不关。
func (c *Core) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closing = true
	c.mu.Unlock()
	c.cancel()
	done := make(chan struct{})
	go func() {
		c.ops.Wait()
		close(done)
	}()
	select {
	case <-done:
		return c.db.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// begin 登记一个请求；关闭中返回 ErrClosed。成功后调用方必须调用 end。
func (c *Core) begin() (end func(), err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return nil, protocol.ErrClosed
	}
	c.ops.Add(1)
	return c.ops.Done, nil
}

// lookup 返回本进程加载的会话。
func (c *Core) lookup(id string) (*session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.sessions[id]; s != nil {
		return s, nil
	}
	return nil, protocol.ErrNotFound
}

// open 返回会话，没加载就从库里加载。
func (c *Core) open(ctx context.Context, id string) (*session, error) {
	c.loading.Lock()
	defer c.loading.Unlock()
	if s, err := c.lookup(id); err == nil {
		return s, nil
	}
	s, err := c.load(ctx, id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions[id] = s
	c.mu.Unlock()
	return s, nil
}

// forget 卸载会话，仅当登记的还是它时。
func (c *Core) forget(s *session) {
	c.mu.Lock()
	if c.sessions[s.id] == s {
		delete(c.sessions, s.id)
	}
	c.mu.Unlock()
}
