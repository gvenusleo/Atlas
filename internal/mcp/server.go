package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
)

const (
	defaultTimeout = 60 * time.Second
	// connectTimeout 限制连接加取工具清单的耗时；首次运行 npx、uvx 要下载包，比调用慢得多。
	connectTimeout = 2 * time.Minute
)

var errClosed = errors.New("MCP 已关闭")

// server 是一个服务器的连接：配置、状态与它暴露的工具。
type server struct {
	name    string
	cfg     config.MCPServer
	timeout time.Duration
	usable  bool          // 配置有效且启用；只有这样的服务器会连接
	ready   chan struct{} // 首次连接结束后关闭

	dial sync.Mutex // 串行化连接尝试

	mu      sync.Mutex // 保护以下字段
	state   State
	err     string
	session *sdk.ClientSession // 只在连着时非空
	cmd     *exec.Cmd          // stdio 子进程
	entries []entry
	closed  bool
}

// entry 是服务器的一个工具。
type entry struct {
	name string // 暴露给模型的名字；服务器给的名字是 tool.Name
	tool *sdk.Tool
}

// newServer 校验配置：无效或停用的服务器不会连接。
func newServer(name string, cfg config.MCPServer) *server {
	timeout := defaultTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	s := &server{name: name, cfg: cfg, timeout: timeout, ready: make(chan struct{})}
	switch err := validate(name, cfg); {
	case err != nil:
		s.state, s.err = StateFailed, err.Error()
	case cfg.Enabled != nil && !*cfg.Enabled:
		s.state = StateDisabled
	default:
		s.state, s.usable = StateConnecting, true
	}
	return s
}

// start 做首次连接；失败的原因已记在状态里。
func (s *server) start(ctx context.Context) {
	defer close(s.ready)
	if s.usable {
		_, _ = s.ensure(ctx)
	}
}

// ensure 返回可用的会话；没连上或已断开时先（重新）连接。
func (s *server) ensure(ctx context.Context) (*sdk.ClientSession, error) {
	s.dial.Lock()
	defer s.dial.Unlock()
	s.mu.Lock()
	cs, closed := s.session, s.closed
	s.mu.Unlock()
	switch {
	case closed:
		return nil, errClosed
	case cs != nil:
		return cs, nil
	}
	return s.connect(ctx)
}

// connect 建立连接并取得工具清单；失败时记下原因并清理子进程。
func (s *server) connect(ctx context.Context) (*sdk.ClientSession, error) {
	s.setState(StateConnecting, "")
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	c, err := s.open()
	if err != nil {
		return nil, s.fail(c, err)
	}
	// 连接超时或被取消时立刻终止进程组：SDK 自己关闭一个挂住的子进程，要多等十来秒。
	stop := context.AfterFunc(ctx, func() { killGroup(c.cmd) })
	defer stop()
	cs, err := s.client().Connect(ctx, c.transport, nil)
	if err != nil {
		return nil, s.fail(c, fmt.Errorf("连接失败: %w%s", err, authHint(err)))
	}
	entries, err := s.listTools(ctx, cs)
	if err != nil {
		_ = cs.Close()
		return nil, s.fail(c, err)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = cs.Close()
		killGroup(c.cmd)
		return nil, errClosed
	}
	s.session, s.cmd, s.entries, s.state, s.err = cs, c.cmd, entries, StateConnected, ""
	s.mu.Unlock()
	go s.watch(cs)
	return cs, nil
}

// client 建一个客户端：不声明 roots、采样等能力，Atlas 都不提供。
func (s *server) client() *sdk.Client {
	return sdk.NewClient(&sdk.Implementation{Name: "atlas", Version: "dev"}, &sdk.ClientOptions{
		Capabilities:           &sdk.ClientCapabilities{},
		ToolListChangedHandler: s.toolsChanged,
	})
}

// fail 记下失败的原因，附上子进程 stderr 的末尾，并终止子进程。
func (s *server) fail(c conn, err error) error {
	if tail := strings.TrimSpace(c.stderr.String()); tail != "" {
		err = fmt.Errorf("%w\n%s", err, tail)
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill() // 没有进程组的平台上，killGroup 什么也不做
	}
	killGroup(c.cmd)
	s.setState(StateFailed, err.Error())
	return err
}

// watch 等连接断开，随后把状态记为已断开，并清理子进程留下的进程组；下次调用时会重连。
// 进程组只在刚断开时清理：子进程退出得久了，它的进程号可能已被别的进程组借用。
func (s *server) watch(cs *sdk.ClientSession) {
	_ = cs.Wait()
	s.mu.Lock()
	var cmd *exec.Cmd
	if s.session == cs {
		s.session, s.state, s.err = nil, StateDisconnected, "连接已断开"
		cmd, s.cmd = s.cmd, nil
	}
	s.mu.Unlock()
	killGroup(cmd)
}

// toolsChanged 在服务器通知工具清单变了时重新取；取不到就保留旧的。新清单对之后创建或
// 恢复的会话生效。
func (s *server) toolsChanged(ctx context.Context, req *sdk.ToolListChangedRequest) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	entries, err := s.listTools(ctx, req.Session)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == req.Session {
		s.entries = entries
	}
}

// listTools 取服务器的全部工具，按配置筛选。
func (s *server) listTools(ctx context.Context, cs *sdk.ClientSession) ([]entry, error) {
	var tools []*sdk.Tool
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("取工具清单失败: %w", err)
		}
		if s.visible(tool.Name) {
			tools = append(tools, tool)
		}
	}
	return s.entriesOf(tools), nil
}

// call 调用服务器的一个工具；服务器断开了先重连。工具自己报告的失败是 IsError 的结果，
// 交给模型处理；连接与协议的失败作为错误返回。调用不重试：服务器可能已经执行了它。
// 超时同时管重连与调用，服务器挂住时，一次调用不会等到连接超时那么久。
func (s *server) call(
	ctx context.Context, tool string, params jsontext.Value,
) (agent.ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cs, err := s.ensure(ctx)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("MCP 服务器 %s 不可用: %w", s.name, err)
	}
	var args any = params
	if len(params) == 0 {
		args = map[string]any{}
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("MCP 服务器 %s 调用 %s 失败: %w", s.name, tool, err)
	}
	return toResult(res), nil
}

// close 断开连接并终止子进程，之后不再重连。
func (s *server) close() {
	s.mu.Lock()
	s.closed = true
	cs, cmd := s.session, s.cmd
	s.session, s.cmd = nil, nil
	s.mu.Unlock()
	if cs != nil {
		_ = cs.Close()
	}
	killGroup(cmd)
}

func (s *server) setState(state State, err string) {
	s.mu.Lock()
	s.state, s.err = state, err
	s.mu.Unlock()
}

func (s *server) status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Name: s.name, State: s.state, Tools: len(s.entries), Err: s.err}
}
