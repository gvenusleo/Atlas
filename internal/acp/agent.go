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
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/transcript"
	"github.com/liuyuxin/atlas/internal/version"
)

const (
	defaultSessionListLimit    = 100
	modelConfigID              = "model"
	reasoningEffortConfigID    = "reasoning_effort"
	defaultReasoningEffortName = "Default"
)

// Runtime 是 ACP 适配层需要的 Atlas 执行入口。
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	ModelOptions(context.Context) (runtime.ModelOptions, error)
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
	cwd             string
	model           string
	reasoningEffort string
	cancel          context.CancelFunc
	turn            int
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
			Version: version.Current,
		},
		AuthMethods: []acpsdk.AuthMethod{},
	}, nil
}

// NewSession 创建绑定到 cwd 的活动 ACP session。
func (a *Agent) NewSession(ctx context.Context, params acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	if err := requireAbsoluteCWD(params.Cwd); err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	models, err := a.rt.ModelOptions(ctx)
	if err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	sessionID, err := session.NewID(time.Now())
	if err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	a.setSession(sessionID, params.Cwd, models.Default, models.ReasoningEffort)
	return acpsdk.NewSessionResponse{
		SessionId:     acpsdk.SessionId(sessionID),
		ConfigOptions: sessionConfigOptions(models, models.Default, models.ReasoningEffort),
	}, nil
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
		SessionID:          string(params.SessionId),
		Prompt:             promptText,
		Model:              state.model,
		ReasoningEffort:    state.reasoningEffort,
		ReasoningEffortSet: true,
		CWD:                state.cwd,
		Observer:           a.observe(turnCtx, params.SessionId),
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
	models, err := a.rt.ModelOptions(ctx)
	if err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	a.setSession(string(params.SessionId), params.Cwd, models.Default, models.ReasoningEffort)
	return acpsdk.ResumeSessionResponse{
		ConfigOptions: sessionConfigOptions(models, models.Default, models.ReasoningEffort),
	}, nil
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

// SetSessionConfigOption 更新 ACP session 级配置。
func (a *Agent) SetSessionConfigOption(ctx context.Context, params acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	if params.ValueId == nil {
		return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("session config requires a value id")
	}
	req := params.ValueId
	if _, ok := a.getSession(string(req.SessionId)); !ok {
		return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("session %q not found", req.SessionId)
	}
	models, err := a.rt.ModelOptions(ctx)
	if err != nil {
		return acpsdk.SetSessionConfigOptionResponse{}, err
	}
	state, ok := a.getSession(string(req.SessionId))
	if !ok {
		return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("session %q not found", req.SessionId)
	}
	switch req.ConfigId {
	case acpsdk.SessionConfigId(modelConfigID):
		if !hasModelValue(models, string(req.Value)) {
			return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("provider model %q is not configured", req.Value)
		}
		state.model = string(req.Value)
	case acpsdk.SessionConfigId(reasoningEffortConfigID):
		if !hasReasoningEffortValue(string(req.Value)) {
			return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("reasoning effort %q is not supported", req.Value)
		}
		state.reasoningEffort = string(req.Value)
	default:
		return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("unsupported session config option: %s", req.ConfigId)
	}
	a.setSessionState(string(req.SessionId), state)
	return acpsdk.SetSessionConfigOptionResponse{
		ConfigOptions: sessionConfigOptions(models, state.model, state.reasoningEffort),
	}, nil
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
		case agent.EventModelReasoningDelta:
			if event.Content == "" {
				return
			}
			update = acpsdk.UpdateAgentThoughtText(event.Content)
		case agent.EventToolStarted:
			update = acpsdk.StartToolCall(
				toolCallID(event),
				toolTitle(event.ToolCall),
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

func (a *Agent) setSession(id, cwd, model, reasoningEffort string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.sessions[id]
	state.cwd = cwd
	state.model = model
	state.reasoningEffort = reasoningEffort
	a.sessions[id] = state
}

func (a *Agent) setSessionState(id string, state sessionState) {
	a.mu.Lock()
	defer a.mu.Unlock()
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
	case "edit_file", "write_file":
		return acpsdk.ToolKindEdit
	case "list_files", "search_text", "web_search":
		return acpsdk.ToolKindSearch
	case "web_fetch":
		return acpsdk.ToolKindFetch
	case "run_shell":
		return acpsdk.ToolKindExecute
	default:
		return acpsdk.ToolKindOther
	}
}

// toolTitle 返回客户端折叠工具调用时优先展示的可读标题。
func toolTitle(call model.ToolCall) string {
	if title := primaryToolTitle(call); title != "" {
		return title
	}
	if call.Name == "" {
		return "Tool"
	}
	return "Tool: " + call.Name
}

// primaryToolTitle 返回内置工具最能说明本次调用意图的动作和参数。
func primaryToolTitle(call model.ToolCall) string {
	prefix, key := "", ""
	switch call.Name {
	case "read_file":
		prefix, key = "Read: ", "path"
	case "write_file":
		prefix, key = "Write: ", "path"
	case "edit_file":
		prefix, key = "Edit: ", "path"
	case "list_files":
		prefix, key = "List: ", "path"
	case "search_text":
		prefix, key = "Search: ", "query"
	case "web_search":
		prefix, key = "WebSearch: ", "query"
	case "web_fetch":
		prefix, key = "WebFetch: ", "url"
	case "run_shell":
		prefix, key = "Run: ", "command"
	default:
		return ""
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return ""
	}
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
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

func sessionConfigOptions(options runtime.ModelOptions, currentModel, currentReasoningEffort string) []acpsdk.SessionConfigOption {
	return []acpsdk.SessionConfigOption{
		modelConfigOption(options, currentModel),
		reasoningEffortConfigOption(currentReasoningEffort),
	}
}

func modelConfigOption(options runtime.ModelOptions, current string) acpsdk.SessionConfigOption {
	category := acpsdk.SessionConfigOptionCategoryModel
	ungrouped := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(options.Models))
	for _, model := range options.Models {
		option := acpsdk.SessionConfigSelectOption{
			Name:  model.Name,
			Value: acpsdk.SessionConfigValueId(model.Value),
		}
		if model.Description != "" {
			description := model.Description
			option.Description = &description
		}
		ungrouped = append(ungrouped, option)
	}
	return acpsdk.SessionConfigOption{
		Select: &acpsdk.SessionConfigOptionSelect{
			Category:     &category,
			CurrentValue: acpsdk.SessionConfigValueId(current),
			Id:           acpsdk.SessionConfigId(modelConfigID),
			Name:         "Model",
			Options: acpsdk.SessionConfigSelectOptions{
				Ungrouped: &ungrouped,
			},
		},
	}
}

func reasoningEffortConfigOption(current string) acpsdk.SessionConfigOption {
	category := acpsdk.SessionConfigOptionCategoryThoughtLevel
	description := "Controls model reasoning depth when supported by the provider."
	ungrouped := acpsdk.SessionConfigSelectOptionsUngrouped{
		{Name: defaultReasoningEffortName, Value: ""},
		{Name: "High", Value: "high"},
		{Name: "Max", Value: "max"},
	}
	return acpsdk.SessionConfigOption{
		Select: &acpsdk.SessionConfigOptionSelect{
			Category:     &category,
			CurrentValue: acpsdk.SessionConfigValueId(current),
			Description:  &description,
			Id:           acpsdk.SessionConfigId(reasoningEffortConfigID),
			Name:         "Reasoning effort",
			Options: acpsdk.SessionConfigSelectOptions{
				Ungrouped: &ungrouped,
			},
		},
	}
}

func hasModelValue(options runtime.ModelOptions, value string) bool {
	for _, model := range options.Models {
		if model.Value == value {
			return true
		}
	}
	return false
}

func hasReasoningEffortValue(value string) bool {
	switch value {
	case "", "high", "max":
		return true
	default:
		return false
	}
}
