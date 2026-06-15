package acp

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
	"github.com/liuyuxin/atlas/internal/version"
)

const (
	defaultSessionListLimit    = 100
	modelConfigID              = "model"
	reasoningEffortConfigID    = "reasoning_effort"
	defaultReasoningEffortName = "Default"
	compactCommandName         = "compact"
)

var errClientTerminalUnavailable = errors.New("client terminal unavailable")

// Runtime 是 ACP 适配层需要的 Atlas 执行入口。
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error)
	ModelOptions(context.Context) (runtime.ModelOptions, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
	ListSessionsPage(context.Context, string, int) (session.ListPage, error)
	ListSessionsForCWDPage(context.Context, string, string, int) (session.ListPage, error)
	SaveSessionRoots(context.Context, string, []string) error
	DeleteSessionIfExists(context.Context, string) error
}

type terminalClient interface {
	CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error)
	KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error)
	TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error)
	ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error)
	WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error)
}

// fileClient 是 ACP 客户端文件系统能力的最小接口。
type fileClient interface {
	ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error)
	WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error)
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
	cwd                   string
	model                 string
	reasoningEffort       string
	additionalDirectories []string
	cancel                context.CancelFunc
	turn                  int
}

// Agent 将 Atlas runtime 适配为 ACP agent 方法。
type Agent struct {
	rt                 Runtime
	terminalClient     terminalClient
	fileClient         fileClient
	sendUpdate         func(context.Context, acpsdk.SessionNotification) error
	clientCapabilities acpsdk.ClientCapabilities

	mu                sync.Mutex
	sessions          map[string]sessionState
	terminalToolCalls map[string]struct{}
}

// NewAgent 创建由 Atlas runtime 驱动的 ACP agent。
func NewAgent(rt Runtime) *Agent {
	return &Agent{
		rt:                rt,
		sessions:          make(map[string]sessionState),
		terminalToolCalls: make(map[string]struct{}),
	}
}

// SetAgentConnection 绑定用于发送 session/update 通知的 SDK 连接。
func (a *Agent) SetAgentConnection(conn *acpsdk.AgentSideConnection) {
	if conn == nil {
		a.sendUpdate = nil
		a.terminalClient = nil
		a.fileClient = nil
		return
	}
	a.sendUpdate = conn.SessionUpdate
	a.terminalClient = conn
	a.fileClient = conn
}

// Initialize 返回 Atlas 支持的 ACP v1 能力。
func (a *Agent) Initialize(_ context.Context, req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	a.clientCapabilities = req.ClientCapabilities
	title := "Atlas"
	return acpsdk.InitializeResponse{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: acpsdk.PromptCapabilities{
				EmbeddedContext: true,
			},
			SessionCapabilities: acpsdk.SessionCapabilities{
				AdditionalDirectories: &acpsdk.SessionAdditionalDirectoriesCapabilities{},
				Close:                 &acpsdk.SessionCloseCapabilities{},
				Delete:                &acpsdk.SessionDeleteCapabilities{},
				List:                  &acpsdk.SessionListCapabilities{},
				Resume:                &acpsdk.SessionResumeCapabilities{},
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
	if err := requireAbsoluteDirectories(params.AdditionalDirectories); err != nil {
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
	a.setSession(sessionID, params.Cwd, models.Default, models.ReasoningEffort, params.AdditionalDirectories)
	if err := a.sendAvailableCommands(ctx, acpsdk.SessionId(sessionID)); err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
	if err := a.sendSessionInfoUpdate(ctx, acpsdk.SessionId(sessionID), "", time.Now()); err != nil {
		return acpsdk.NewSessionResponse{}, err
	}
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
	if instruction, ok := compactCommandInstruction(promptText); ok {
		return a.runCommandPrompt(ctx, params.SessionId, state, instruction)
	}
	a.cancelSession(string(params.SessionId))
	turnCtx, cancel := context.WithCancel(ctx)
	turn, ok := a.setSessionCancel(string(params.SessionId), cancel)
	if !ok {
		cancel()
		return acpsdk.PromptResponse{}, fmt.Errorf("session %q not found", params.SessionId)
	}
	defer a.clearSessionCancel(string(params.SessionId), turn)

	result, err := a.rt.RunTurn(turnCtx, runtime.TurnOptions{
		SessionID:                string(params.SessionId),
		Prompt:                   promptText,
		Model:                    state.model,
		ReasoningEffort:          state.reasoningEffort,
		ReasoningEffortSet:       true,
		AdditionalDirectories:    state.additionalDirectories,
		AdditionalDirectoriesSet: true,
		CWD:                      state.cwd,
		Observer:                 a.observe(turnCtx, params.SessionId),
		ToolRunner:               a.toolRunner(params.SessionId, state.cwd),
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
	if err := a.sendStoredSessionInfoUpdate(ctx, params.SessionId); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if err := a.sendUsageUpdate(ctx, params.SessionId, usageUsed(result.Usage), result.ContextWindow); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	response := acpsdk.PromptResponse{
		StopReason: acpsdk.StopReasonEndTurn,
		Usage:      acpUsage(result.Usage),
	}
	if params.MessageId != nil {
		response.UserMessageId = params.MessageId
	}
	return response, nil
}

// LoadSession 恢复已有 Atlas session，并通过 session/update 回放历史消息。
func (a *Agent) LoadSession(ctx context.Context, params acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	if err := requireAbsoluteCWD(params.Cwd); err != nil {
		return acpsdk.LoadSessionResponse{}, err
	}
	if err := requireAbsoluteDirectories(params.AdditionalDirectories); err != nil {
		return acpsdk.LoadSessionResponse{}, err
	}
	sess, trans, err := a.rt.ShowSession(ctx, string(params.SessionId))
	if err != nil {
		return acpsdk.LoadSessionResponse{}, err
	}
	if sess.CWD != params.Cwd {
		return acpsdk.LoadSessionResponse{}, fmt.Errorf("session %q cwd mismatch: %s", params.SessionId, params.Cwd)
	}
	models, err := a.rt.ModelOptions(ctx)
	if err != nil {
		return acpsdk.LoadSessionResponse{}, err
	}
	if err := a.rt.SaveSessionRoots(ctx, string(params.SessionId), params.AdditionalDirectories); err != nil {
		return acpsdk.LoadSessionResponse{}, err
	}
	sess.AdditionalDirectories = append([]string(nil), params.AdditionalDirectories...)
	a.setSession(string(params.SessionId), params.Cwd, models.Default, models.ReasoningEffort, params.AdditionalDirectories)
	if err := a.replayTranscript(ctx, params.SessionId, trans); err != nil {
		a.deleteSessionState(string(params.SessionId))
		return acpsdk.LoadSessionResponse{}, err
	}
	if err := a.sendAvailableCommands(ctx, params.SessionId); err != nil {
		a.deleteSessionState(string(params.SessionId))
		return acpsdk.LoadSessionResponse{}, err
	}
	if err := a.sendSessionInfoUpdate(ctx, params.SessionId, sess.Title, sess.UpdatedAt); err != nil {
		a.deleteSessionState(string(params.SessionId))
		return acpsdk.LoadSessionResponse{}, err
	}
	if err := a.sendUsageUpdate(ctx, params.SessionId, sess.LastTotalTokens, modelContextWindow(models, models.Default)); err != nil {
		a.deleteSessionState(string(params.SessionId))
		return acpsdk.LoadSessionResponse{}, err
	}
	return acpsdk.LoadSessionResponse{
		ConfigOptions: sessionConfigOptions(models, models.Default, models.ReasoningEffort),
	}, nil
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
	if err := requireAbsoluteDirectories(params.AdditionalDirectories); err != nil {
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
	if err := a.rt.SaveSessionRoots(ctx, string(params.SessionId), params.AdditionalDirectories); err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	sess.AdditionalDirectories = append([]string(nil), params.AdditionalDirectories...)
	a.setSession(string(params.SessionId), params.Cwd, models.Default, models.ReasoningEffort, params.AdditionalDirectories)
	if err := a.sendAvailableCommands(ctx, params.SessionId); err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	if err := a.sendSessionInfoUpdate(ctx, params.SessionId, sess.Title, sess.UpdatedAt); err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	if err := a.sendUsageUpdate(ctx, params.SessionId, sess.LastTotalTokens, modelContextWindow(models, models.Default)); err != nil {
		return acpsdk.ResumeSessionResponse{}, err
	}
	return acpsdk.ResumeSessionResponse{
		ConfigOptions: sessionConfigOptions(models, models.Default, models.ReasoningEffort),
	}, nil
}

// ListSessions 返回 Atlas 本地 SQLite session 历史。
func (a *Agent) ListSessions(ctx context.Context, params acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	var (
		page session.ListPage
		err  error
	)
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	if params.Cwd != nil && *params.Cwd != "" {
		if err := requireAbsoluteCWD(*params.Cwd); err != nil {
			return acpsdk.ListSessionsResponse{}, err
		}
		page, err = a.rt.ListSessionsForCWDPage(ctx, *params.Cwd, cursor, defaultSessionListLimit)
	} else {
		page, err = a.rt.ListSessionsPage(ctx, cursor, defaultSessionListLimit)
	}
	if err != nil {
		return acpsdk.ListSessionsResponse{}, err
	}

	infos := make([]acpsdk.SessionInfo, 0, len(page.Sessions))
	for _, sess := range page.Sessions {
		infos = append(infos, sessionInfo(sess))
	}
	resp := acpsdk.ListSessionsResponse{Sessions: infos}
	if page.NextCursor != "" {
		resp.NextCursor = &page.NextCursor
	}
	return resp, nil
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
				tool.DisplayTitle(event.ToolCall),
				acpsdk.WithStartKind(toolKind(event.ToolCall.Name)),
				acpsdk.WithStartStatus(acpsdk.ToolCallStatusInProgress),
				acpsdk.WithStartRawInput(rawToolInput(event.ToolCall.Arguments)),
			)
		case agent.EventToolFinished:
			status := acpsdk.ToolCallStatusCompleted
			if event.ToolError {
				status = acpsdk.ToolCallStatusFailed
			}
			includeContent := !a.takeTerminalToolCall(sessionID, toolCallID(event))
			opts := toolCallUpdateOptions(status, event.ToolResult, event.ToolMetadata, includeContent)
			update = acpsdk.UpdateToolCall(
				toolCallID(event),
				opts...,
			)
		default:
			return
		}
		_ = a.sendSessionUpdate(ctx, sessionID, update)
	}
}

func (a *Agent) toolRunner(sessionID acpsdk.SessionId, cwd string) runtime.ToolRunner {
	return func(ctx context.Context, call model.ToolCall, fallback tool.RunFunc) (tool.RunResult, error) {
		if call.Name == "run_shell" && a.clientCapabilities.Terminal && a.terminalClient != nil {
			result, err := a.runShellInClientTerminal(ctx, sessionID, cwd, call)
			if errors.Is(err, errClientTerminalUnavailable) {
				return fallback(ctx, call)
			}
			return result, err
		}
		if isFileTool(call.Name) {
			return a.runFileTool(ctx, sessionID, cwd, call, fallback)
		}
		return fallback(ctx, call)
	}
}

// runShellInClientTerminal 使用 ACP terminal 能力执行 run_shell 并返回最终输出。
func (a *Agent) runShellInClientTerminal(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall) (tool.RunResult, error) {
	args, err := tool.ParseShellArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	spec := tool.DefaultShell()
	terminalArgs := append([]string(nil), spec.Args...)
	terminalArgs = append(terminalArgs, args.Command)
	workdir := args.Workdir
	if workdir == "" {
		workdir = cwd
	}
	limit := 128 * 1024
	createReq := acpsdk.CreateTerminalRequest{
		SessionId:       sessionID,
		Command:         spec.Command,
		Args:            terminalArgs,
		OutputByteLimit: &limit,
	}
	if workdir != "" {
		createReq.Cwd = &workdir
	}
	terminal, err := a.terminalClient.CreateTerminal(ctx, createReq)
	if err != nil {
		return tool.RunResult{}, fmt.Errorf("%w: %v", errClientTerminalUnavailable, err)
	}
	defer func() {
		_, _ = a.terminalClient.ReleaseTerminal(context.Background(), acpsdk.ReleaseTerminalRequest{
			SessionId:  sessionID,
			TerminalId: terminal.TerminalId,
		})
	}()
	if err := a.sendTerminalToolCallContent(ctx, sessionID, call, terminal.TerminalId); err != nil {
		a.killClientTerminal(sessionID, terminal.TerminalId)
		return tool.RunResult{}, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, tool.ShellTimeout(args.TimeoutSeconds))
	defer cancel()
	exit, err := a.terminalClient.WaitForTerminalExit(waitCtx, acpsdk.WaitForTerminalExitRequest{
		SessionId:  sessionID,
		TerminalId: terminal.TerminalId,
	})
	if waitErr := waitCtx.Err(); waitErr != nil {
		a.killClientTerminal(sessionID, terminal.TerminalId)
		status := "command cancelled"
		if waitErr == context.DeadlineExceeded {
			status = fmt.Sprintf("command timed out after %s", tool.ShellTimeout(args.TimeoutSeconds))
		}
		output, outputErr := a.terminalOutputWithStatus(context.Background(), sessionID, terminal.TerminalId, status)
		if waitErr == context.DeadlineExceeded && outputErr != nil {
			return tool.RunResult{Content: output}, outputErr
		}
		return tool.RunResult{Content: output}, waitErr
	}
	if err != nil {
		a.killClientTerminal(sessionID, terminal.TerminalId)
		return tool.RunResult{}, err
	}
	status := terminalExitStatus(exit)
	output, err := a.terminalOutputWithStatus(ctx, sessionID, terminal.TerminalId, status)
	return tool.RunResult{Content: output}, err
}

func (a *Agent) killClientTerminal(sessionID acpsdk.SessionId, terminalID string) {
	_, _ = a.terminalClient.KillTerminal(context.Background(), acpsdk.KillTerminalRequest{
		SessionId:  sessionID,
		TerminalId: terminalID,
	})
}

func (a *Agent) sendTerminalToolCallContent(ctx context.Context, sessionID acpsdk.SessionId, call model.ToolCall, terminalID string) error {
	if a.sendUpdate == nil {
		return nil
	}
	status := acpsdk.ToolCallStatusInProgress
	if err := a.sendSessionUpdate(ctx, sessionID, acpsdk.UpdateToolCall(
		acpsdk.ToolCallId(call.ID),
		acpsdk.WithUpdateStatus(status),
		acpsdk.WithUpdateContent([]acpsdk.ToolCallContent{acpsdk.ToolTerminalRef(terminalID)}),
	)); err != nil {
		return err
	}
	a.markTerminalToolCall(sessionID, acpsdk.ToolCallId(call.ID))
	return nil
}

func (a *Agent) terminalOutputWithStatus(ctx context.Context, sessionID acpsdk.SessionId, terminalID, status string) (string, error) {
	output, err := a.terminalClient.TerminalOutput(ctx, acpsdk.TerminalOutputRequest{
		SessionId:  sessionID,
		TerminalId: terminalID,
	})
	if err != nil {
		return "", err
	}
	content := output.Output
	if output.Truncated {
		content = "[output truncated]\n" + content
	}
	if status != "" {
		content = appendToolStatus(content, status)
		return content, fmt.Errorf("%s", status)
	}
	return content, nil
}

func terminalExitStatus(exit acpsdk.WaitForTerminalExitResponse) string {
	if exit.Signal != nil && *exit.Signal != "" {
		return "command terminated by signal " + *exit.Signal
	}
	if exit.ExitCode != nil && *exit.ExitCode != 0 {
		return fmt.Sprintf("command exited with code %d", *exit.ExitCode)
	}
	return ""
}

func appendToolStatus(output, status string) string {
	status = "[" + status + "]"
	if output == "" {
		return status
	}
	if strings.HasSuffix(output, "\n") {
		return output + status
	}
	return output + "\n" + status
}

func (a *Agent) markTerminalToolCall(sessionID acpsdk.SessionId, toolCallID acpsdk.ToolCallId) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.terminalToolCalls[terminalToolCallKey(sessionID, toolCallID)] = struct{}{}
}

func (a *Agent) takeTerminalToolCall(sessionID acpsdk.SessionId, toolCallID acpsdk.ToolCallId) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := terminalToolCallKey(sessionID, toolCallID)
	_, ok := a.terminalToolCalls[key]
	delete(a.terminalToolCalls, key)
	return ok
}

func terminalToolCallKey(sessionID acpsdk.SessionId, toolCallID acpsdk.ToolCallId) string {
	return string(sessionID) + "\x00" + string(toolCallID)
}

func (a *Agent) replayTranscript(ctx context.Context, sessionID acpsdk.SessionId, trans *transcript.Transcript) error {
	if a.sendUpdate == nil || trans == nil {
		return nil
	}
	var pendingToolIDs []acpsdk.ToolCallId
	for messageIndex, msg := range trans.Messages() {
		switch msg.Role {
		case model.RoleUser:
			if msg.Content != "" {
				if err := a.sendSessionUpdate(ctx, sessionID, acpsdk.UpdateUserMessageText(msg.Content)); err != nil {
					return err
				}
			}
		case model.RoleAssistant:
			if msg.ReasoningContent != "" {
				if err := a.sendSessionUpdate(ctx, sessionID, acpsdk.UpdateAgentThoughtText(msg.ReasoningContent)); err != nil {
					return err
				}
			}
			if msg.Content != "" {
				if err := a.sendSessionUpdate(ctx, sessionID, acpsdk.UpdateAgentMessageText(msg.Content)); err != nil {
					return err
				}
			}
			for toolIndex, call := range msg.ToolCalls {
				toolID := replayToolCallID(messageIndex, toolIndex, call.ID)
				pendingToolIDs = append(pendingToolIDs, toolID)
				if err := a.sendSessionUpdate(ctx, sessionID, replayToolStart(toolID, call)); err != nil {
					return err
				}
			}
		case model.RoleTool:
			toolID := acpsdk.ToolCallId(msg.ToolCallID)
			if toolID == "" && len(pendingToolIDs) > 0 {
				toolID = pendingToolIDs[0]
			}
			if len(pendingToolIDs) > 0 {
				pendingToolIDs = pendingToolIDs[1:]
			}
			if toolID == "" {
				continue
			}
			if err := a.sendSessionUpdate(ctx, sessionID, replayToolResult(toolID, msg)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Agent) sendSessionUpdate(ctx context.Context, sessionID acpsdk.SessionId, update acpsdk.SessionUpdate) error {
	if a.sendUpdate == nil {
		return nil
	}
	return a.sendUpdate(ctx, acpsdk.SessionNotification{
		SessionId: sessionID,
		Update:    update,
	})
}

// sendAvailableCommands 通知客户端当前 session 可用的 slash command。
func (a *Agent) sendAvailableCommands(ctx context.Context, sessionID acpsdk.SessionId) error {
	return a.sendSessionUpdate(ctx, sessionID, acpsdk.SessionUpdate{
		AvailableCommandsUpdate: &acpsdk.SessionAvailableCommandsUpdate{
			AvailableCommands: []acpsdk.AvailableCommand{compactAvailableCommand()},
		},
	})
}

// sendStoredSessionInfoUpdate 从本地 session 记录发送最新标题和更新时间。
func (a *Agent) sendStoredSessionInfoUpdate(ctx context.Context, sessionID acpsdk.SessionId) error {
	info, _, err := a.rt.ShowSession(ctx, string(sessionID))
	if err != nil {
		return err
	}
	return a.sendSessionInfoUpdate(ctx, sessionID, info.Title, info.UpdatedAt)
}

// sendSessionInfoUpdate 通知客户端 session 标题或更新时间变化。
func (a *Agent) sendSessionInfoUpdate(ctx context.Context, sessionID acpsdk.SessionId, title string, updatedAt time.Time) error {
	update := &acpsdk.SessionSessionInfoUpdate{
		SessionUpdate: "session_info_update",
	}
	if title != "" {
		update.Title = &title
	}
	if !updatedAt.IsZero() {
		formatted := updatedAt.UTC().Format(time.RFC3339)
		update.UpdatedAt = &formatted
	}
	if update.Title == nil && update.UpdatedAt == nil {
		return nil
	}
	return a.sendSessionUpdate(ctx, sessionID, acpsdk.SessionUpdate{SessionInfoUpdate: update})
}

// sendUsageUpdate 通知客户端当前上下文窗口使用情况。
func (a *Agent) sendUsageUpdate(ctx context.Context, sessionID acpsdk.SessionId, used, size int) error {
	if used <= 0 || size <= 0 {
		return nil
	}
	return a.sendSessionUpdate(ctx, sessionID, acpsdk.SessionUpdate{
		UsageUpdate: &acpsdk.SessionUsageUpdate{
			SessionUpdate: "usage_update",
			Used:          used,
			Size:          size,
		},
	})
}

// compactAvailableCommand 返回 ACP 客户端展示的手动压缩命令定义。
func compactAvailableCommand() acpsdk.AvailableCommand {
	return acpsdk.AvailableCommand{
		Name:        compactCommandName,
		Description: "Compact earlier conversation context.",
		Input: &acpsdk.AvailableCommandInput{
			Unstructured: &acpsdk.UnstructuredCommandInput{
				Hint: "optional instruction",
			},
		},
	}
}

// runCommandPrompt 执行不进入模型 turn 的 ACP slash command。
func (a *Agent) runCommandPrompt(ctx context.Context, sessionID acpsdk.SessionId, state sessionState, instruction string) (acpsdk.PromptResponse, error) {
	a.cancelSession(string(sessionID))
	turnCtx, cancel := context.WithCancel(ctx)
	turn, ok := a.setSessionCancel(string(sessionID), cancel)
	if !ok {
		cancel()
		return acpsdk.PromptResponse{}, fmt.Errorf("session %q not found", sessionID)
	}
	defer a.clearSessionCancel(string(sessionID), turn)
	result, err := a.rt.CompactSession(turnCtx, runtime.CompactOptions{
		SessionID:          string(sessionID),
		Model:              state.model,
		ReasoningEffort:    state.reasoningEffort,
		ReasoningEffortSet: true,
		CWD:                state.cwd,
		Instruction:        instruction,
	})
	if err != nil {
		if turnCtx.Err() != nil {
			return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
		}
		return acpsdk.PromptResponse{}, err
	}
	message := "No safe context to compact."
	if result.Compacted {
		message = fmt.Sprintf("Context compacted. Kept %d recent messages.", result.KeepCount)
	}
	if err := a.sendSessionUpdate(ctx, sessionID, acpsdk.UpdateAgentMessageText(message)); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if err := a.sendStoredSessionInfoUpdate(ctx, sessionID); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	if err := a.sendUsageUpdate(ctx, sessionID, result.TokensAfter, result.ContextWindow); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *Agent) setSession(id, cwd, model, reasoningEffort string, additionalDirectories []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.sessions[id]
	state.cwd = cwd
	state.model = model
	state.reasoningEffort = reasoningEffort
	state.additionalDirectories = append([]string(nil), additionalDirectories...)
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

// requireAbsoluteDirectories 校验 ACP 额外工作目录根都是绝对路径。
func requireAbsoluteDirectories(dirs []string) error {
	for _, dir := range dirs {
		if dir == "" {
			return fmt.Errorf("additional directory is required")
		}
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("additional directory must be absolute: %s", dir)
		}
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
		case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
			parts = append(parts, embeddedTextResource(block.Resource.Resource.TextResourceContents))
		case block.Resource != nil && block.Resource.Resource.BlobResourceContents != nil:
			return "", fmt.Errorf("unsupported ACP embedded blob resource")
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

// embeddedTextResource 将 ACP 内嵌文本资源转成模型可读的文本片段。
func embeddedTextResource(resource *acpsdk.TextResourceContents) string {
	lines := []string{fmt.Sprintf("Resource: %s", resource.Uri)}
	if resource.MimeType != nil && *resource.MimeType != "" {
		lines = append(lines, fmt.Sprintf("MIME: %s", *resource.MimeType))
	}
	lines = append(lines, "", resource.Text)
	return strings.Join(lines, "\n")
}

// sessionInfo 将本地 session 元数据转换为 ACP session/list 结果。
func sessionInfo(sess session.Session) acpsdk.SessionInfo {
	info := acpsdk.SessionInfo{
		SessionId:             acpsdk.SessionId(sess.ID),
		Cwd:                   sess.CWD,
		AdditionalDirectories: append([]string(nil), sess.AdditionalDirectories...),
	}
	if sess.Title != "" {
		title := sess.Title
		info.Title = &title
	}
	if !sess.UpdatedAt.IsZero() {
		updatedAt := sess.UpdatedAt.UTC().Format(time.RFC3339)
		info.UpdatedAt = &updatedAt
	}
	return info
}

// acpUsage 将 Atlas 模型用量转换为 ACP prompt 响应用量。
func acpUsage(usage model.Usage) *acpsdk.Usage {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &acpsdk.Usage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

// usageUsed 返回 usage_update 中展示的上下文占用估计值。
func usageUsed(usage model.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens
}

// modelContextWindow 返回当前模型配置的上下文窗口大小。
func modelContextWindow(options runtime.ModelOptions, current string) int {
	for _, model := range options.Models {
		if model.Value == current {
			return model.ContextWindow
		}
	}
	return 0
}

// compactCommandInstruction 解析 `/compact` 命令及其可选指令。
func compactCommandInstruction(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "/"+compactCommandName {
		return "", true
	}
	prefix := "/" + compactCommandName
	if strings.HasPrefix(text, prefix+" ") || strings.HasPrefix(text, prefix+"\t") || strings.HasPrefix(text, prefix+"\n") {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
	}
	return "", false
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

func replayToolStart(toolID acpsdk.ToolCallId, call model.ToolCall) acpsdk.SessionUpdate {
	return acpsdk.StartToolCall(
		toolID,
		tool.DisplayTitle(call),
		acpsdk.WithStartKind(toolKind(call.Name)),
		acpsdk.WithStartStatus(acpsdk.ToolCallStatusInProgress),
		acpsdk.WithStartRawInput(rawToolInput(call.Arguments)),
	)
}

func replayToolResult(toolID acpsdk.ToolCallId, msg model.Message) acpsdk.SessionUpdate {
	return acpsdk.UpdateToolCall(
		toolID,
		toolCallUpdateOptions(acpsdk.ToolCallStatusCompleted, msg.Content, msg.ToolMetadata, true)...,
	)
}

// toolCallUpdateOptions 将 Atlas 工具结果映射成 ACP tool_call_update。
func toolCallUpdateOptions(status acpsdk.ToolCallStatus, result string, metadata model.ToolMetadata, includeContent bool) []acpsdk.ToolCallUpdateOpt {
	opts := []acpsdk.ToolCallUpdateOpt{
		acpsdk.WithUpdateStatus(status),
		acpsdk.WithUpdateRawOutput(result),
	}
	if locations := toolCallLocations(metadata); len(locations) > 0 {
		opts = append(opts, acpsdk.WithUpdateLocations(locations))
	}
	if includeContent {
		opts = append(opts, acpsdk.WithUpdateContent(toolCallContent(result, metadata)))
	}
	return opts
}

// toolCallContent 优先使用结构化 diff，否则回退为普通文本内容。
func toolCallContent(result string, metadata model.ToolMetadata) []acpsdk.ToolCallContent {
	if metadata.Diff != nil {
		if metadata.Diff.OldText == nil {
			return []acpsdk.ToolCallContent{
				acpsdk.ToolDiffContent(metadata.Diff.Path, metadata.Diff.NewText),
			}
		}
		return []acpsdk.ToolCallContent{
			acpsdk.ToolDiffContent(metadata.Diff.Path, metadata.Diff.NewText, *metadata.Diff.OldText),
		}
	}
	return []acpsdk.ToolCallContent{
		acpsdk.ToolContent(acpsdk.TextBlock(result)),
	}
}

// toolCallLocations 将持久化的文件位置转换为 ACP 可跳转位置。
func toolCallLocations(metadata model.ToolMetadata) []acpsdk.ToolCallLocation {
	if len(metadata.Locations) == 0 {
		return nil
	}
	locations := make([]acpsdk.ToolCallLocation, 0, len(metadata.Locations))
	for _, location := range metadata.Locations {
		item := acpsdk.ToolCallLocation{Path: location.Path}
		if location.Line > 0 {
			line := location.Line
			item.Line = &line
		}
		locations = append(locations, item)
	}
	return locations
}

func replayToolCallID(messageIndex, toolIndex int, id string) acpsdk.ToolCallId {
	if id != "" {
		return acpsdk.ToolCallId(id)
	}
	return acpsdk.ToolCallId(fmt.Sprintf("tool_%d_%d", messageIndex, toolIndex))
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
