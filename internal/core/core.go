// Package core 是 Atlas 的核心：实现 protocol.Service，管理本进程加载的会话，是会话运行时
// 数据的唯一权威。GUI 和以后的远程入口都通过 Connect 拿到的 protocol.Service 使用它。
//
// 历史只在一条消息定稿时写库；会话的上下文在写库之后用 agent.Fold 推进，所以始终等于库里的
// 历史折叠出来的结果。会话的控制状态只有它是否忙（session.busy）。一轮归会话所有，与发起它的
// 连接无关。多个进程可以打开同一个库，但同一会话的并发写入是不支持的用法：写入时由序号检查
// 发现，后写入的一方得到 ErrConflict，会话在本进程内失效。
//
// 包内分三块：会话运行（session.go、turn.go）、更新转换（updates.go，事件与消息到更新的
// 纯函数）、模型解析（model.go）；persist.go 负责消息与记录之间的转换。
package core

import (
	"context"
	"sync"
	"sync/atomic"

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
	working  sync.WaitGroup // 进行中的请求与一轮，Close 等它们结束再关库
	versions atomic.Uint64  // 运行和队列的标识与版本，重新加载会话时也单调递增
	opening  map[string]*sessionGate
}

// sessionGate 协调同一会话的生命周期，等待者也占用一个引用。
type sessionGate struct {
	ready chan struct{}
	refs  int
}

// New 用打开的数据库建核心。
func New(db *store.DB) *Core {
	ctx, cancel := context.WithCancel(context.Background())
	return &Core{db: db, ctx: ctx, cancel: cancel, sessions: make(map[string]*session),
		opening: make(map[string]*sessionGate)}
}

// Connect 为一个客户端建连接：它订阅的会话的更新都推给 sub。
func (c *Core) Connect(sub protocol.Subscriber) protocol.Service {
	return &conn{core: c, sub: sub}
}

// Close 不再接收请求，取消所有正在运行的一轮，等请求和一轮都结束后关库。ctx 到期时不再等，
// 返回它的错误，数据库留着不关。
func (c *Core) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closing = true
	c.mu.Unlock()
	c.cancel()
	done := make(chan struct{})
	go func() {
		c.working.Wait()
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
	c.working.Add(1)
	return c.working.Done, nil
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

// lockSession 串行同一个会话的打开、关闭、删除和卸载。
func (c *Core) lockSession(ctx context.Context, id string) (func(), error) {
	c.mu.Lock()
	gate := c.opening[id]
	if gate == nil {
		gate = &sessionGate{ready: make(chan struct{}, 1)}
		gate.ready <- struct{}{}
		c.opening[id] = gate
	}
	gate.refs++
	c.mu.Unlock()
	release := func() {
		c.mu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(c.opening, id)
		}
		c.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	case <-gate.ready:
		unlock := func() {
			gate.ready <- struct{}{}
			release()
		}
		if err := ctx.Err(); err != nil {
			unlock()
			return nil, err
		}
		return unlock, nil
	}
}

// open 加载会话，并在订阅时取得当前状态和历史边界。
func (c *Core) open(
	ctx context.Context, id string, sub protocol.Subscriber,
) (protocol.OpenSessionResponse, error) {
	var response protocol.OpenSessionResponse
	done, err := c.lockSession(ctx, id)
	if err != nil {
		return response, err
	}
	defer done()
	info, err := c.db.Get(ctx, id)
	if err != nil {
		return response, mapStoreError(err)
	}
	s, err := c.lookup(id)
	if err != nil {
		if s, err = c.load(ctx, id); err != nil {
			return response, err
		}
		c.register(s)
	}
	options, err := s.options()
	if err != nil {
		if s.idle() {
			c.forget(s)
		}
		return response, err
	}
	state, seq, err := s.subscribe(sub, info.Title)
	return protocol.OpenSessionResponse{ConfigOptions: options, State: state, LastSeq: seq}, err
}

// adopt 登记新建的会话，并让 sub 订阅它。
func (c *Core) adopt(s *session, sub protocol.Subscriber) protocol.SessionState {
	// 新会话的标识尚未公布；先登记订阅再将它写进会话表。
	s.subscribers = []protocol.Subscriber{sub}
	c.register(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state("")
}

// release 取消订阅。运行结束、队列清空或配置更新后，会再次检查是否可以卸载。
func (c *Core) release(ctx context.Context, id string, sub protocol.Subscriber) error {
	done, err := c.lockSession(ctx, id)
	if err != nil {
		return err
	}
	defer done()
	if s, err := c.lookup(id); err == nil {
		s.unsubscribe(sub)
		if s.idle() {
			c.forget(s)
		}
	}
	return nil
}

// unloadIfIdle 仅在会话没有订阅者、没有进行中的操作且队列为空时卸载。
func (c *Core) unloadIfIdle(s *session) {
	done, _ := c.lockSession(context.Background(), s.id)
	defer done()
	if s.idle() {
		c.forget(s)
	}
}

// register 登记一个加载了的会话。
func (c *Core) register(s *session) {
	version := c.versions.Add(1)
	s.epoch, s.runState.Version, s.queue.Version = version, version, version
	c.mu.Lock()
	c.sessions[s.id] = s
	c.mu.Unlock()
}

// forget 卸载会话，仅当登记的还是它时。
func (c *Core) forget(s *session) {
	c.mu.Lock()
	if c.sessions[s.id] == s {
		delete(c.sessions, s.id)
	}
	c.mu.Unlock()
}
