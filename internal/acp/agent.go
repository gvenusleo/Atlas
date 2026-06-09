package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/transcript"
)

const (
	defaultSessionListLimit = 100
)

// Runtime 是 ACP 适配层需要的 Atlas 执行入口。
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
	ListSessions(context.Context, int) ([]session.Session, error)
	ListSessionsForCWD(context.Context, string, int) ([]session.Session, error)
	DeleteSessionIfExists(context.Context, string) error
}

// Options 描述启动 ACP 标准输入输出服务所需参数。
type Options struct {
	Runtime Runtime
	Input   io.Reader
	Output  io.Writer
	Logger  *slog.Logger
}

// Run 启动 ACP agent 连接，并阻塞直到客户端断开。
func Run(ctx context.Context, opts Options) error {
	if opts.Runtime == nil {
		return fmt.Errorf("acp runtime is required")
	}
	if opts.Input == nil {
		return fmt.Errorf("acp input is required")
	}
	if opts.Output == nil {
		return fmt.Errorf("acp output is required")
	}
	a := NewAgent(opts.Runtime)
	conn := acpsdk.NewAgentSideConnection(a, opts.Output, opts.Input)
	if opts.Logger != nil {
		conn.SetLogger(opts.Logger)
	}
	a.SetAgentConnection(conn)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.Done():
		return nil
	}
}

type sessionState struct {
	cwd    string
	cancel context.CancelFunc
	turn   int
}

// Agent 将 Atlas runtime 适配为 ACP agent 方法。
type Agent struct {
	rt         Runtime
	sendUpdate func(context.Context, acpsdk.SessionNotification) error

	mu       sync.Mutex
	sessions map[string]sessionState
}

// NewAgent 创建由 Atlas runtime 驱动的 ACP agent。
func NewAgent(rt Runtime) *Agent {
	return &Agent{
		rt:       rt,
		sessions: make(map[string]sessionState),
	}
}

// SetAgentConnection 绑定用于发送 session/update 通知的 SDK 连接。
func (a *Agent) SetAgentConnection(conn *acpsdk.AgentSideConnection) {
	if conn == nil {
		a.sendUpdate = nil
		return
	}
	a.sendUpdate = conn.SessionUpdate
}

// Initialize 返回 Atlas 支持的 ACP v1 能力。
func (a *Agent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	title := "Atlas"
	return acpsdk.InitializeResponse{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession: false,
			SessionCapabilities: acpsdk.SessionCapabilities{
				Close:  &acpsdk.SessionCloseCapabilities{},
				Delete: &acpsdk.SessionDeleteCapabilities{},
				List:   &acpsdk.SessionListCapabilities{},
				Resume: &acpsdk.SessionResumeCapabilities{},
			},
		},
		AgentInfo: &acpsdk.Implementation{
			Name:    "atlas",
			Title:   &title,
			Version: "0.0.0",
		},
		AuthMethods: []acpsdk.AuthMethod{},
	}, nil
}

// NewSession 创建绑定到 cwd 的活动 ACP session。
func (a *Agent) NewSession(_ context.Context, params acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	if err := requireAbsoluteCWD(params.Cwd); err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	sessionID, err := session.NewID(time.Now())
	if err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	a.setSession(sessionID, params.Cwd)
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(sessionID)}, nil
}

// Prompt 为指定 ACP session 执行一次 Atlas turn。
func (a *Agent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	state, ok := a.getSession(string(params.SessionId))
	if !ok {
		return acpsdk.PromptResponse{}, fmt.Errorf("session %q not found", params.SessionId)
	}
	promptText, err := promptToText(params.Prompt)
	if err != nil {
		return acpsdk.PromptResponse{}, err
	}
	a.cancelSession(string(params.SessionId))
	turnCtx, cancel := context.WithCancel(ctx)
	turn, ok := a.setSessionCancel(string(params.SessionId), cancel)
	if !ok {
		cancel()
		return acpsdk.PromptResponse{}, fmt.Errorf("session %q not found", params.SessionId)
	}
	defer a.clearSessionCancel(string(params.SessionId), turn)

	_, err = a.rt.RunTurn(turnCtx, runtime.TurnOptions{
		SessionID: string(params.SessionId),
		Prompt:    promptText,
		CWD:       state.cwd,
		Observer:  a.observe(turnCtx, params.SessionId),
	})
	if err != nil {
		if turnCtx.Err() != nil {
			return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
		}
		return acpsdk.PromptResponse{}, err
	}
	if turnCtx.Err() != nil {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

// Cancel 停止指定 session 中正在运行的 prompt。
func (a *Agent) Cancel(_ context.Context, params acpsdk.CancelNotification) error {
	a.cancelSession(string(params.SessionId))
	return nil
}

// ResumeSession 将已有 Atlas session 标记为活动状态，不回放历史消息。
func (a *Agent) ResumeSession(ctx context.Context, params acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	if err := requireAbsoluteCWD(params.Cwd); err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	sess, _, err := a.rt.ShowSession(ctx, string(params.SessionId))
	if err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	if sess.CWD != params.Cwd {
		return acpsdk.ResumeSessionResponse{}, fmt.Errorf("session %q cwd mismatch: %s", params.SessionId, params.Cwd)
	}
	a.setSession(string(params.SessionId), params.Cwd)
	return acpsdk.ResumeSessionResponse{}, nil
}

// ListSessions 返回 Atlas 本地 SQLite session 历史。
func (a *Agent) ListSessions(ctx context.Context, params acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	if params.Cursor != nil && *params.Cursor != "" {
		return acpsdk.ListSessionsResponse{}, fmt.Errorf("session/list cursor is not supported")
	}
	var (
		sessions []session.Session
		err      error
	)
	if params.Cwd != nil && *params.Cwd != "" {
		if err := requireAbsoluteCWD(*params.Cwd); err != nil {
			return acpsdk.ListSessionsResponse{}, err
		}
		sessions, err = a.rt.ListSessionsForCWD(ctx, *params.Cwd, defaultSessionListLimit)
	} else {
		sessions, err = a.rt.ListSessions(ctx, defaultSessionListLimit)
	}
	if err != nil {
		return acpsdk.ListSessionsResponse{}, err
	}

	infos := make([]acpsdk.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		title := sess.Title
		updatedAt := sess.UpdatedAt.Format(time.RFC3339)
		infos = append(infos, acpsdk.SessionInfo{
			SessionId: acpsdk.SessionId(sess.ID),
			Cwd:       sess.CWD,
			Title:     &title,
			UpdatedAt: &updatedAt,
		})
	}
	return acpsdk.ListSessionsResponse{Sessions: infos}, nil
}

// CloseSession 取消正在运行的 turn，并清除本地活动 session 状态。
func (a *Agent) CloseSession(_ context.Context, params acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	a.deleteSessionState(string(params.SessionId))
	return acpsdk.CloseSessionResponse{}, nil
}

// UnstableDeleteSession 实现 SDK 当前的 session/delete 钩子。
func (a *Agent) UnstableDeleteSession(ctx context.Context, params acpsdk.UnstableDeleteSessionRequest) (acpsdk.UnstableDeleteSessionResponse, error) {
	a.deleteSessionState(string(params.SessionId))
	if err := a.rt.DeleteSessionIfExists(ctx, string(params.SessionId)); err != nil {
		return acpsdk.UnstableDeleteSessionResponse{}, err
	}
	return acpsdk.UnstableDeleteSessionResponse{}, nil
}

// Authenticate 不受支持，因为 Atlas 尚未实现 ACP auth。
func (a *Agent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodAuthenticate)
}

// Logout 不受支持，因为 Atlas 尚未实现 ACP auth。
func (a *Agent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodLogout)
}

// SetSessionConfigOption 不受支持，因为 Atlas 没有 ACP session 级配置。
func (a *Agent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionSetConfigOption)
}

// SetSessionMode 不受支持，因为 Atlas 没有 ACP session mode。
func (a *Agent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionSetMode)
}

func (a *Agent) observe(ctx context.Context, sessionID acpsdk.SessionId) agent.Observer {
	return func(event agent.Event) {
		if a.sendUpdate == nil {
			return
		}
		var update acpsdk.SessionUpdate
		switch event.Type {
		case agent.EventModelDelta:
			if event.Content == "" {
				return
			}
			update = acpsdk.UpdateAgentMessageText(event.Content)
		case agent.EventToolStarted:
			update = acpsdk.StartToolCall(
				toolCallID(event),
				"Tool: "+event.ToolCall.Name,
				acpsdk.WithStartKind(toolKind(event.ToolCall.Name)),
				acpsdk.WithStartStatus(acpsdk.ToolCallStatusInProgress),
				acpsdk.WithStartRawInput(rawToolInput(event.ToolCall.Arguments)),
			)
		case agent.EventToolFinished:
			status := acpsdk.ToolCallStatusCompleted
			if event.ToolError {
				status = acpsdk.ToolCallStatusFailed
			}
			update = acpsdk.UpdateToolCall(
				toolCallID(event),
				acpsdk.WithUpdateStatus(status),
				acpsdk.WithUpdateContent([]acpsdk.ToolCallContent{
					acpsdk.ToolContent(acpsdk.TextBlock(event.ToolResult)),
				}),
				acpsdk.WithUpdateRawOutput(event.ToolResult),
			)
		default:
			return
		}
		_ = a.sendUpdate(ctx, acpsdk.SessionNotification{
			SessionId: sessionID,
			Update:    update,
		})
	}
}

func (a *Agent) setSession(id, cwd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.sessions[id]
	state.cwd = cwd
	a.sessions[id] = state
}

func (a *Agent) getSession(id string) (sessionState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[id]
	return state, ok
}

func (a *Agent) deleteSessionState(id string) {
	if id == "" {
		return
	}
	a.mu.Lock()
	state := a.sessions[id]
	delete(a.sessions, id)
	a.mu.Unlock()
	if state.cancel != nil {
		state.cancel()
	}
}

func (a *Agent) setSessionCancel(id string, cancel context.CancelFunc) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[id]
	if !ok {
		return 0, false
	}
	state.turn++
	state.cancel = cancel
	a.sessions[id] = state
	return state.turn, true
}

func (a *Agent) clearSessionCancel(id string, turn int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[id]
	if !ok || state.turn != turn {
		return
	}
	state.cancel = nil
	a.sessions[id] = state
}

func (a *Agent) cancelSession(id string) {
	a.mu.Lock()
	cancel := a.sessions[id].cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func requireAbsoluteCWD(cwd string) error {
	if cwd == "" {
		return fmt.Errorf("cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		return fmt.Errorf("cwd must be absolute: %s", cwd)
	}
	return nil
}

func promptToText(blocks []acpsdk.ContentBlock) (string, error) {
	var parts []string
	for _, block := range blocks {
		switch {
		case block.Text != nil:
			parts = append(parts, block.Text.Text)
		case block.ResourceLink != nil:
			parts = append(parts, fmt.Sprintf("Resource: %s (%s)", block.ResourceLink.Name, block.ResourceLink.Uri))
		default:
			return "", fmt.Errorf("unsupported ACP prompt content block")
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return text, nil
}

func toolCallID(event agent.Event) acpsdk.ToolCallId {
	if event.ToolCall.ID != "" {
		return acpsdk.ToolCallId(event.ToolCall.ID)
	}
	if event.ToolCall.Name != "" {
		return acpsdk.ToolCallId(fmt.Sprintf("tool_%d_%s", event.Step, event.ToolCall.Name))
	}
	return acpsdk.ToolCallId(fmt.Sprintf("tool_%d", event.Step))
}

func toolKind(name string) acpsdk.ToolKind {
	switch name {
	case "read_file":
		return acpsdk.ToolKindRead
	case "write_file":
		return acpsdk.ToolKindEdit
	case "list_files", "search_text":
		return acpsdk.ToolKindSearch
	case "run_shell":
		return acpsdk.ToolKindExecute
	default:
		return acpsdk.ToolKindOther
	}
}

func rawToolInput(arguments string) any {
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(arguments), &parsed); err == nil {
		return parsed
	}
	return arguments
}
