// Package mcp 连接 settings.json 里配置的 MCP 服务器，把它们的工具交给 agent 使用。
package mcp

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
)

// State 是一个服务器的连接状态。
type State string

const (
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateDisconnected State = "disconnected" // 连上之后断开，下次调用时重连
	StateFailed       State = "failed"       // 配置无效或连不上，原因见 Status.Err
	StateDisabled     State = "disabled"
)

// Status 是一个服务器当前的状态，供界面显示。
type Status struct {
	Name  string
	State State
	Tools int    // 暴露给模型的工具数
	Err   string // 失败或断开的原因
}

// Manager 持有全部服务器的连接。
type Manager struct {
	servers []*server // 按名称排序
}

// Start 在后台连接 servers 里每个启用的服务器，随即返回；ctx 取消时放弃还没连上的。
func Start(ctx context.Context, servers map[string]config.MCPServer) *Manager {
	m := &Manager{}
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		s := newServer(name, servers[name])
		m.servers = append(m.servers, s)
		go s.start(ctx)
	}
	return m
}

// Wait 等各服务器完成首次连接，成败都算；ctx 先结束就不再等。
func (m *Manager) Wait(ctx context.Context) {
	for _, s := range m.servers {
		select {
		case <-s.ready:
		case <-ctx.Done():
			return
		}
	}
}

// Tools 返回各服务器暴露的工具，按服务器与工具名排序。服务器在调用时才会重连，
// 所以断开的服务器的工具也在其中。
func (m *Manager) Tools() []agent.Tool {
	var tools []agent.Tool
	for _, s := range m.servers {
		tools = append(tools, s.agentTools()...)
	}
	return tools
}

// Statuses 返回各服务器当前的状态，按名称排序。
func (m *Manager) Statuses() []Status {
	statuses := make([]Status, len(m.servers))
	for i, s := range m.servers {
		statuses[i] = s.status()
	}
	return statuses
}

// Close 断开全部服务器：子进程先收到 stdin 的关闭，一直不退出才被终止。
func (m *Manager) Close() {
	var wg sync.WaitGroup
	for _, s := range m.servers {
		wg.Go(s.close)
	}
	wg.Wait()
}
