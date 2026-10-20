package host

import (
	"context"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/mcp"
)

// mcpWait 是创建或恢复会话时，最多等 MCP 服务器完成首次连接的时间；
// 之后连上的服务器，工具要到下一个会话才有。
const mcpWait = 10 * time.Second

// StartMCP 读取 settings.json，在后台连接其中的 MCP 服务器。只调用一次；设置里没有服务器
// 时什么也不做。设置的改动不会热更新，要重启才生效。
func (m *Manager) StartMCP() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closing && m.mcp == nil {
		m.mcp = mcp.Start(m.ctx, settings.MCPServers)
	}
	return nil
}

// MCPStatus 返回各个 MCP 服务器的状态，还没启动时为空。
func (m *Manager) MCPStatus() []mcp.Status {
	if mm := m.mcpManager(); mm != nil {
		return mm.Statuses()
	}
	return nil
}

// WaitMCP 等各个 MCP 服务器完成首次连接，成败都算；ctx 先结束就不再等。
func (m *Manager) WaitMCP(ctx context.Context) {
	if mm := m.mcpManager(); mm != nil {
		mm.Wait(ctx)
	}
}

// mcpTools 返回 MCP 服务器暴露的工具；首次连接还没结束时最多等 mcpWait。
func (m *Manager) mcpTools(ctx context.Context) []agent.Tool {
	mm := m.mcpManager()
	if mm == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, mcpWait)
	defer cancel()
	mm.Wait(ctx)
	return mm.Tools()
}

func (m *Manager) mcpManager() *mcp.Manager {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mcp
}
