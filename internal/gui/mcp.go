package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/mcp"
)

// startMCP 在后台连接配置里的 MCP 服务器，连完（成败都算）后刷新界面，显示它们的状态。
func (a *App) startMCP() {
	go func() {
		err := a.host.StartMCP()
		if err == nil {
			a.host.WaitMCP(context.Background())
		}
		a.win.Update(func() {
			if err != nil {
				a.err = err.Error()
			}
		})
	}()
}

// mcpStatus 在状态行显示 MCP 服务器的连接情况（已连接数/启用数），悬停时列出各个服务器；
// 没有启用的服务器时不显示，有连接失败的用警示色。
func (a *App) mcpStatus(c *ui.Context, t *ui.Theme) {
	if a.host == nil {
		return
	}
	var lines []string
	enabled, connected, failed := 0, 0, false
	for _, s := range a.host.MCPStatus() {
		lines = append(lines, mcpLine(s))
		if s.State != mcp.StateDisabled {
			enabled++
		}
		if s.State == mcp.StateConnected {
			connected++
		}
		failed = failed || s.State == mcp.StateFailed
	}
	if enabled == 0 {
		return
	}
	color := t.TextMuted
	if failed {
		color = t.Danger
	}
	ui.Text(c, fmt.Sprintf("MCP %d/%d", connected, enabled)).FontSize(smallSize).TextColor(color).
		SingleLine().Padding(3, 6).Tooltip(strings.Join(lines, "\n"))
}

// mcpLine 是一个服务器在提示里的一行。
func mcpLine(s mcp.Status) string {
	switch s.State {
	case mcp.StateConnected:
		return fmt.Sprintf("%s：已连接，%d 个工具", s.Name, s.Tools)
	case mcp.StateConnecting:
		return s.Name + "：连接中…"
	case mcp.StateDisconnected:
		return s.Name + "：已断开，下次调用时重连"
	case mcp.StateDisabled:
		return s.Name + "：已停用"
	}
	return s.Name + "：失败：" + truncate(s.Err, 300)
}
