// Package acp exposes Atlas via the Agent Client Protocol.
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
	"sync/atomic"
	"time"
	"unicode"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
	"github.com/liuyuxin/atlas/internal/version"
)

const (
	defaultSessionListLimit   = 100
	modelConfigID             = "model"
	reasoningEffortConfigID   = "reasoning_effort"
	compactCommandName        = "compact"
	postResponseUpdateDelay   = 50 * time.Millisecond
	postResponseUpdateTimeout = 2 * time.Second
)

var errClientTerminalUnavailable = errors.New("client terminal unavailable")

// Runtime is the Atlas execution entry point needed by the ACP adapter layer.
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error)
	ModelOptions(context.Context) (runtime.ModelOptions, error)
	SkillSummaries(context.Context, string) ([]runtime.SkillSummary, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
	ListSessionsPage(context.Context, string, int) (session.ListPage, error)
	ListSessionsForCWDPage(context.Context, string, string, int) (session.ListPage, error)
	SaveSessionRoots(context.Context, string, []string) error
	DeleteSessionIfExists(context.Context, string) error
	RunMemoryWorker(context.Context) error
}

type terminalClient interface {
	CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error)
	KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error)
	TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error)
	ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error)
	WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error)
}

// fileClient is the minimal interface for ACP client filesystem capabilities.
type fileClient interface {
	ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error)
	WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error)
}

// Options describes the parameters for starting the ACP stdio service.
type Options struct {
	Runtime Runtime
	Input   io.Reader
	Output  io.Writer
	Logger  *slog.Logger
}

// Run starts the ACP agent connection and blocks until the client disconnects.
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
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := opts.Runtime.RunMemoryWorker(workerCtx); err != nil && opts.Logger != nil {
			opts.Logger.Debug("memory worker stopped", "error", err)
		}
	}()

	// Cancel the worker before waiting for it to finish, ensuring Run returns
	// promptly regardless of whether ctx or conn triggered the exit.
	select {
	case <-ctx.Done():
		cancelWorker()
		<-workerDone
		return ctx.Err()
	case <-conn.Done():
		cancelWorker()
		<-workerDone
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
	// running prevents concurrent turns within the same session.
	running atomic.Bool
}

// Agent adapts the Atlas runtime to ACP agent methods.
type Agent struct {
	rt                 Runtime
	terminalClient     terminalClient
	fileClient         fileClient
	sendUpdate         func(context.Context, acpsdk.SessionNotification) error
	clientCapabilities acpsdk.ClientCapabilities

	mu                sync.Mutex
	sessions          map[string]*sessionState
	terminalToolCalls map[string]struct{}
}

// NewAgent creates an ACP agent driven by the Atlas runtime.
func NewAgent(rt Runtime) *Agent {
	return &Agent{
		rt:                rt,
		sessions:          make(map[string]*sessionState),
		terminalToolCalls: make(map[string]struct{}),
	}
}

// SetAgentConnection binds the SDK connection used for sending session/update notifications.
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

// Initialize returns the ACP v1 capabilities supported by Atlas.
func (a *Agent) Initialize(ctx context.Context, req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	a.clientCapabilities = req.ClientCapabilities
	title := "Atlas"
	imageInput := false
	if models, err := a.rt.ModelOptions(ctx); err == nil {
		imageInput = modelOptionsSupportInput(models, config.ModelInputFormatImage)
	}
	return acpsdk.InitializeResponse{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: acpsdk.PromptCapabilities{
				EmbeddedContext: true,
				Image:           imageInput,
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

// NewSession creates an active ACP session bound to the specified cwd.
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
	now := time.Now()
	defaultReasoning := modelInitialReasoningEffort(models, models.Default)
	a.setSession(sessionID, params.Cwd, models.Default, defaultReasoning, params.AdditionalDirectories)
	a.sendSessionMetadataLater(acpsdk.SessionId(sessionID), "", now, 0, 0)
	return acpsdk.NewSessionResponse{
		SessionId:     acpsdk.SessionId(sessionID),
		ConfigOptions: sessionConfigOptions(models, models.Default, defaultReasoning),
	}, nil
}

// Prompt executes an Atlas turn for the specified ACP session.
func (a *Agent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	state, ok := a.getSession(string(params.SessionId))
	if !ok {
		return acpsdk.PromptResponse{}, fmt.Errorf("session %q not found", params.SessionId)
	}
	promptParts, err := promptToParts(params.Prompt)
	if err != nil {
		return acpsdk.PromptResponse{}, err
	}
	promptText := strings.TrimSpace(model.TextFromParts(promptParts))
	if instruction, ok := compactCommandInstruction(promptText); ok {
		if promptHasImage(promptParts) {
			return acpsdk.PromptResponse{}, fmt.Errorf("slash commands do not support images")
		}
		return a.runCommandPrompt(ctx, params.SessionId, state, instruction)
	}
	selectedSkills := skillNames(promptText, a.skillCommands(ctx, state.cwd))
	// Reject if a turn is already running in this session, matching WS/WeChat behavior.
	if !state.running.CompareAndSwap(false, true) {
		return acpsdk.PromptResponse{}, fmt.Errorf("a turn is already running in session %q, send cancel first", params.SessionId)
	}
	defer state.running.Store(false)
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
		Parts:                    promptParts,
		Skills:                   selectedSkills,
		Model:                    state.model,
		ReasoningEffort:          state.reasoningEffort,
		ReasoningEffortSet:       true,
		AdditionalDirectories:    state.additionalDirectories,
		AdditionalDirectoriesSet: true,
		CWD:                      state.cwd,
		Observer:                 a.observe(turnCtx, params.SessionId, state.cwd),
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
	a.sendStoredSessionMetadata(ctx, params.SessionId, usageUsed(result.Usage), result.ContextWindow)
	response := acpsdk.PromptResponse{
		StopReason: acpsdk.StopReasonEndTurn,
		Usage:      acpUsage(result.Usage),
	}
	if params.MessageId != nil {
		response.UserMessageId = params.MessageId
	}
	return response, nil
}

// LoadSession restores an existing Atlas session and replays history via session/update.
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
	defaultReasoning := modelInitialReasoningEffort(models, models.Default)
	a.setSession(string(params.SessionId), params.Cwd, models.Default, defaultReasoning, params.AdditionalDirectories)
	if err := a.replayTranscript(ctx, params.SessionId, params.Cwd, trans); err != nil {
		a.deleteSessionState(string(params.SessionId))
		return acpsdk.LoadSessionResponse{}, err
	}
	a.sendSessionMetadataLater(params.SessionId, sess.Title, sess.UpdatedAt, sess.LastTotalTokens, modelContextWindow(models, models.Default))
	return acpsdk.LoadSessionResponse{
		ConfigOptions: sessionConfigOptions(models, models.Default, defaultReasoning),
	}, nil
}

// Cancel stops the currently running prompt in the specified session.
func (a *Agent) Cancel(_ context.Context, params acpsdk.CancelNotification) error {
	a.cancelSession(string(params.SessionId))
	return nil
}

// ResumeSession marks an existing Atlas session as active without replaying history.
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
	defaultReasoning := modelInitialReasoningEffort(models, models.Default)
	a.setSession(string(params.SessionId), params.Cwd, models.Default, defaultReasoning, params.AdditionalDirectories)
	a.sendSessionMetadataLater(params.SessionId, sess.Title, sess.UpdatedAt, sess.LastTotalTokens, modelContextWindow(models, models.Default))
	return acpsdk.ResumeSessionResponse{
		ConfigOptions: sessionConfigOptions(models, models.Default, defaultReasoning),
	}, nil
}

// ListSessions returns Atlas local SQLite session history.
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

// CloseSession cancels the running turn and clears the local active session state.
func (a *Agent) CloseSession(_ context.Context, params acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	a.deleteSessionState(string(params.SessionId))
	return acpsdk.CloseSessionResponse{}, nil
}

// UnstableDeleteSession implements the SDK's current session/delete hook.
func (a *Agent) UnstableDeleteSession(ctx context.Context, params acpsdk.UnstableDeleteSessionRequest) (acpsdk.UnstableDeleteSessionResponse, error) {
	a.deleteSessionState(string(params.SessionId))
	if err := a.rt.DeleteSessionIfExists(ctx, string(params.SessionId)); err != nil {
		return acpsdk.UnstableDeleteSessionResponse{}, err
	}
	return acpsdk.UnstableDeleteSessionResponse{}, nil
}

// Authenticate is not supported because Atlas has not yet implemented ACP auth.
func (a *Agent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodAuthenticate)
}

// Logout is not supported because Atlas has not yet implemented ACP auth.
func (a *Agent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodLogout)
}

// SetSessionConfigOption updates the ACP session-level configuration.
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
		if !hasReasoningEffortValue(models, state.model, state.reasoningEffort) {
			state.reasoningEffort = modelInitialReasoningEffort(models, state.model)
		}
	case acpsdk.SessionConfigId(reasoningEffortConfigID):
		if !hasReasoningEffortValue(models, state.model, string(req.Value)) {
			return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("reasoning effort %q is not supported by model %q", req.Value, state.model)
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

// SetSessionMode is not supported because Atlas does not have ACP session modes.
func (a *Agent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, acpsdk.NewMethodNotFound(acpsdk.AgentMethodSessionSetMode)
}

func (a *Agent) observe(ctx context.Context, sessionID acpsdk.SessionId, cwd string) agent.Observer {
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
				tool.DisplayTitle(event.ToolCall, cwd),
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
			// Send plan update after todo_write tool execution.
			if len(event.ToolMetadata.Todos) > 0 {
				entries := make([]acpsdk.PlanEntry, len(event.ToolMetadata.Todos))
				for i, todo := range event.ToolMetadata.Todos {
					entries[i] = acpsdk.PlanEntry{
						Content:  todo.Content,
						Priority: acpsdk.PlanEntryPriorityMedium,
						Status:   acpsdk.PlanEntryStatus(todo.Status),
					}
				}
				planUpdate := acpsdk.UpdatePlan(entries...)
				_ = a.sendSessionUpdate(ctx, sessionID, planUpdate)
			}
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

// runShellInClientTerminal executes run_shell using ACP terminal capabilities and returns the final output.
func (a *Agent) runShellInClientTerminal(ctx context.Context, sessionID acpsdk.SessionId, cwd string, call model.ToolCall) (tool.RunResult, error) {
	args, err := tool.ParseShellArgs(call.Arguments)
	if err != nil {
		return tool.RunResult{}, err
	}
	spec := tool.DefaultShell()
	terminalArgs := append([]string(nil), spec.Args...)
	terminalArgs = append(terminalArgs, args.Command)
	terminalCWD := args.CWD
	if terminalCWD == "" {
		terminalCWD = cwd
	}
	limit := 128 * 1024
	createReq := acpsdk.CreateTerminalRequest{
		SessionId:       sessionID,
		Command:         spec.Command,
		Args:            terminalArgs,
		OutputByteLimit: &limit,
	}
	if terminalCWD != "" {
		createReq.Cwd = &terminalCWD
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
		output, outputErr := a.terminalOutputWithStatus(context.Background(), sessionID, terminal.TerminalId, status, true)
		if waitErr == context.DeadlineExceeded && outputErr != nil {
			return tool.RunResult{Content: output}, outputErr
		}
		return tool.RunResult{Content: output}, waitErr
	}
	if err != nil {
		a.killClientTerminal(sessionID, terminal.TerminalId)
		return tool.RunResult{}, err
	}
	status, failed := terminalExitStatus(exit, args.SuccessExitCodes)
	output, err := a.terminalOutputWithStatus(ctx, sessionID, terminal.TerminalId, status, failed)
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

func (a *Agent) terminalOutputWithStatus(ctx context.Context, sessionID acpsdk.SessionId, terminalID, status string, failed bool) (string, error) {
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
		if failed {
			return content, fmt.Errorf("%s", status)
		}
	}
	return content, nil
}

func terminalExitStatus(exit acpsdk.WaitForTerminalExitResponse, successExitCodes []int) (string, bool) {
	if exit.Signal != nil && *exit.Signal != "" {
		return "command terminated by signal " + *exit.Signal, true
	}
	if exit.ExitCode != nil && !tool.IsSuccessfulExitCode(successExitCodes, *exit.ExitCode) {
		return fmt.Sprintf("command exited with code %d", *exit.ExitCode), true
	}
	if exit.ExitCode != nil && *exit.ExitCode != 0 {
		return fmt.Sprintf("command exited with accepted code %d", *exit.ExitCode), false
	}
	return "", false
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

func (a *Agent) replayTranscript(ctx context.Context, sessionID acpsdk.SessionId, cwd string, trans *transcript.Transcript) error {
	if a.sendUpdate == nil || trans == nil {
		return nil
	}
	var pendingToolIDs []acpsdk.ToolCallId
	for messageIndex, msg := range trans.Messages() {
		switch msg.Role {
		case model.RoleUser:
			for _, part := range model.MessageParts(msg) {
				update, ok := userMessageUpdate(part)
				if !ok {
					continue
				}
				if err := a.sendSessionUpdate(ctx, sessionID, update); err != nil {
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
				if err := a.sendSessionUpdate(ctx, sessionID, replayToolStart(toolID, call, cwd)); err != nil {
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

func userMessageUpdate(part model.ContentPart) (acpsdk.SessionUpdate, bool) {
	switch part.Type {
	case model.ContentPartImage:
		data, ok := base64FromDataURL(part.DataURL)
		if !ok || part.MimeType == "" {
			return acpsdk.SessionUpdate{}, false
		}
		block := acpsdk.ImageBlock(data, part.MimeType)
		if part.URI != "" {
			block.Image.Uri = acpsdk.Ptr(part.URI)
		}
		return acpsdk.UpdateUserMessage(block), true
	default:
		if part.Text == "" {
			return acpsdk.SessionUpdate{}, false
		}
		return acpsdk.UpdateUserMessageText(part.Text), true
	}
}

func base64FromDataURL(value string) (string, bool) {
	const marker = ";base64,"
	index := strings.Index(value, marker)
	if index < 0 {
		return "", false
	}
	return value[index+len(marker):], true
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

// sendAvailableCommands notifies the client of the slash commands available in the current session.
func (a *Agent) sendAvailableCommands(ctx context.Context, sessionID acpsdk.SessionId) error {
	state, ok := a.getSession(string(sessionID))
	if !ok {
		return nil
	}
	return a.sendSessionUpdate(ctx, sessionID, acpsdk.SessionUpdate{
		AvailableCommandsUpdate: &acpsdk.SessionAvailableCommandsUpdate{
			AvailableCommands: a.availableCommands(ctx, state.cwd),
		},
	})
}

// sendSessionMetadataLater sends non-critical session metadata after the response is returned, avoiding issues when the client has not yet registered the session.
func (a *Agent) sendSessionMetadataLater(sessionID acpsdk.SessionId, title string, updatedAt time.Time, used, size int) {
	if a.sendUpdate == nil {
		return
	}
	go func() {
		timer := time.NewTimer(postResponseUpdateDelay)
		defer timer.Stop()
		<-timer.C
		if _, ok := a.getSession(string(sessionID)); !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), postResponseUpdateTimeout)
		defer cancel()
		_ = a.sendAvailableCommands(ctx, sessionID)
		_ = a.sendSessionInfoUpdate(ctx, sessionID, title, updatedAt)
		_ = a.sendUsageUpdate(ctx, sessionID, used, size)
	}()
}

// sendStoredSessionMetadata best-effort sends the title and usage after a turn ends.
func (a *Agent) sendStoredSessionMetadata(ctx context.Context, sessionID acpsdk.SessionId, used, size int) {
	info, _, err := a.rt.ShowSession(ctx, string(sessionID))
	if err != nil {
		return
	}
	_ = a.sendSessionInfoUpdate(ctx, sessionID, info.Title, info.UpdatedAt)
	_ = a.sendUsageUpdate(ctx, sessionID, used, size)
}

// sendSessionInfoUpdate notifies the client of session title or update time changes.
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

// sendUsageUpdate notifies the client of the current context window usage.
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

// compactAvailableCommand returns the manual compaction command definition for ACP client display.
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

// skillAvailableCommand returns the skill slash command definition for ACP client display.
func skillAvailableCommand(summary runtime.SkillSummary) acpsdk.AvailableCommand {
	return acpsdk.AvailableCommand{
		Name:        summary.Name,
		Description: summary.Description,
		Input: &acpsdk.AvailableCommandInput{
			Unstructured: &acpsdk.UnstructuredCommandInput{
				Hint: "task",
			},
		},
	}
}

// availableCommands returns the slash commands displayable by the client in the current session working directory.
func (a *Agent) availableCommands(ctx context.Context, cwd string) []acpsdk.AvailableCommand {
	commands := []acpsdk.AvailableCommand{compactAvailableCommand()}
	for _, summary := range a.skillCommands(ctx, cwd) {
		commands = append(commands, skillAvailableCommand(summary))
	}
	return commands
}

// skillCommands returns skill summaries that can be exposed as ACP slash commands.
func (a *Agent) skillCommands(ctx context.Context, cwd string) []runtime.SkillSummary {
	summaries, err := a.rt.SkillSummaries(ctx, cwd)
	if err != nil {
		return nil
	}
	commands := make([]runtime.SkillSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Name == "" || summary.Name == compactCommandName || !validSlashCommandName(summary.Name) {
			continue
		}
		commands = append(commands, summary)
	}
	return commands
}

// runCommandPrompt executes an ACP slash command without entering a model turn.
func (a *Agent) runCommandPrompt(ctx context.Context, sessionID acpsdk.SessionId, state *sessionState, instruction string) (acpsdk.PromptResponse, error) {
	if !state.running.CompareAndSwap(false, true) {
		return acpsdk.PromptResponse{}, fmt.Errorf("a turn is already running in session %q, send cancel first", sessionID)
	}
	defer state.running.Store(false)
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
	a.sendStoredSessionMetadata(ctx, sessionID, result.TokensAfter, result.ContextWindow)
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *Agent) setSession(id, cwd, model, reasoningEffort string, additionalDirectories []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[id]
	if !ok {
		state = &sessionState{}
		a.sessions[id] = state
	}
	state.cwd = cwd
	state.model = model
	state.reasoningEffort = reasoningEffort
	state.additionalDirectories = append([]string(nil), additionalDirectories...)
}

func (a *Agent) setSessionState(id string, state *sessionState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions[id] = state
}

func (a *Agent) getSession(id string) (*sessionState, bool) {
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
	if state != nil && state.cancel != nil {
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
}

func (a *Agent) cancelSession(id string) {
	a.mu.Lock()
	state := a.sessions[id]
	a.mu.Unlock()
	if state != nil && state.cancel != nil {
		state.cancel()
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

// requireAbsoluteDirectories validates that ACP additional working directory roots are all absolute paths.
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

func promptToParts(blocks []acpsdk.ContentBlock) ([]model.ContentPart, error) {
	var parts []model.ContentPart
	for _, block := range blocks {
		switch {
		case block.Text != nil:
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: block.Text.Text})
		case block.Image != nil:
			part, err := imageBlockPart(block.Image)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		case block.ResourceLink != nil:
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: fmt.Sprintf("Resource: %s (%s)", block.ResourceLink.Name, block.ResourceLink.Uri)})
		case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: embeddedTextResource(block.Resource.Resource.TextResourceContents)})
		case block.Resource != nil && block.Resource.Resource.BlobResourceContents != nil:
			part, err := embeddedBlobResourcePart(block.Resource.Resource.BlobResourceContents)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		default:
			return nil, fmt.Errorf("unsupported ACP prompt content block")
		}
	}
	if len(parts) == 0 || strings.TrimSpace(model.TextFromParts(parts)) == "" && !promptHasImage(parts) {
		return nil, fmt.Errorf("prompt is required")
	}
	return parts, nil
}

func imageBlockPart(image *acpsdk.ContentBlockImage) (model.ContentPart, error) {
	if image == nil || strings.TrimSpace(image.Data) == "" || strings.TrimSpace(image.MimeType) == "" {
		return model.ContentPart{}, fmt.Errorf("ACP image block is incomplete")
	}
	return model.ContentPart{
		Type:     model.ContentPartImage,
		MimeType: image.MimeType,
		DataURL:  dataURL(image.MimeType, image.Data),
		URI:      derefString(image.Uri),
		Detail:   model.ImageDetailAuto,
	}, nil
}

func embeddedBlobResourcePart(resource *acpsdk.BlobResourceContents) (model.ContentPart, error) {
	mimeType := ""
	if resource.MimeType != nil {
		mimeType = *resource.MimeType
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return model.ContentPart{}, fmt.Errorf("unsupported ACP embedded blob resource")
	}
	if strings.TrimSpace(resource.Blob) == "" {
		return model.ContentPart{}, fmt.Errorf("ACP embedded image resource is empty")
	}
	return model.ContentPart{
		Type:     model.ContentPartImage,
		MimeType: mimeType,
		DataURL:  dataURL(mimeType, resource.Blob),
		URI:      resource.Uri,
		Detail:   model.ImageDetailAuto,
	}, nil
}

func dataURL(mimeType, base64Data string) string {
	return "data:" + mimeType + ";base64," + base64Data
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func promptHasImage(parts []model.ContentPart) bool {
	for _, part := range parts {
		if part.Type == model.ContentPartImage {
			return true
		}
	}
	return false
}

// embeddedTextResource converts an ACP embedded text resource into model-readable text segments.
func embeddedTextResource(resource *acpsdk.TextResourceContents) string {
	lines := []string{fmt.Sprintf("Resource: %s", resource.Uri)}
	if resource.MimeType != nil && *resource.MimeType != "" {
		lines = append(lines, fmt.Sprintf("MIME: %s", *resource.MimeType))
	}
	lines = append(lines, "", resource.Text)
	return strings.Join(lines, "\n")
}

// sessionInfo converts local session metadata to ACP session/list results.
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

// acpUsage converts Atlas model usage to ACP prompt response usage.
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

// usageUsed returns the estimated context usage displayed in usage_update.
func usageUsed(usage model.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens
}

// modelContextWindow returns the configured context window size for the current model.
func modelContextWindow(options runtime.ModelOptions, current string) int {
	for _, model := range options.Models {
		if model.Value == current {
			return model.ContextWindow
		}
	}
	return 0
}

// compactCommandInstruction parses the `/compact` command and its optional instruction.
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

// skillNames scans text for whitespace-separated /name tokens and returns
// matched skill names in order of first appearance, deduplicated.
func skillNames(text string, skills []runtime.SkillSummary) []string {
	skillSet := make(map[string]bool, len(skills))
	for _, s := range skills {
		skillSet[s.Name] = true
	}
	var matched []string
	seen := make(map[string]bool)
	for _, field := range strings.Fields(text) {
		name, ok := slashCommandName(field)
		if !ok || !skillSet[name] || seen[name] {
			continue
		}
		matched = append(matched, name)
		seen[name] = true
	}
	return matched
}

// slashCommandName parses the name from a `/name optional text` slash command.
func slashCommandName(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", false
	}
	withoutSlash := strings.TrimPrefix(text, "/")
	if withoutSlash == "" {
		return "", false
	}
	index := strings.IndexFunc(withoutSlash, unicode.IsSpace)
	name := withoutSlash
	if index >= 0 {
		name = withoutSlash[:index]
	}
	if name == "" {
		return "", false
	}
	if !validSlashCommandName(name) {
		return "", false
	}
	return name, true
}

// validSlashCommandName determines whether a skill name can be safely mapped to an ACP slash command.
func validSlashCommandName(name string) bool {
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return name != ""
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

func replayToolStart(toolID acpsdk.ToolCallId, call model.ToolCall, cwd string) acpsdk.SessionUpdate {
	return acpsdk.StartToolCall(
		toolID,
		tool.DisplayTitle(call, cwd),
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

// toolCallUpdateOptions maps Atlas tool results to ACP tool_call_update.
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

// toolCallContent prioritizes structured diff, falling back to plain text content.
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

// toolCallLocations converts persisted file locations to ACP navigable locations.
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
	case "edit_file", "write_file", "apply_patch":
		return acpsdk.ToolKindEdit
	case "glob", "grep", "web_search":
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
	result := []acpsdk.SessionConfigOption{
		modelConfigOption(options, currentModel),
	}
	if option, ok := reasoningEffortConfigOption(options, currentModel, currentReasoningEffort); ok {
		result = append(result, option)
	}
	return result
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

func reasoningEffortConfigOption(options runtime.ModelOptions, currentModel, current string) (acpsdk.SessionConfigOption, bool) {
	modelOption, ok := findModelOption(options, currentModel)
	if !ok || len(modelOption.ReasoningEfforts) == 0 {
		return acpsdk.SessionConfigOption{}, false
	}
	category := acpsdk.SessionConfigOptionCategoryThoughtLevel
	description := "Controls model reasoning depth when supported by the provider."
	ungrouped := make(acpsdk.SessionConfigSelectOptionsUngrouped, 0, len(modelOption.ReasoningEfforts))
	for _, effort := range modelOption.ReasoningEfforts {
		option := acpsdk.SessionConfigSelectOption{
			Name:  effort.Name,
			Value: acpsdk.SessionConfigValueId(effort.Value),
		}
		if effort.Description != "" {
			effortDescription := effort.Description
			option.Description = &effortDescription
		}
		ungrouped = append(ungrouped, option)
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
	}, true
}

func hasModelValue(options runtime.ModelOptions, value string) bool {
	_, ok := findModelOption(options, value)
	return ok
}

func hasReasoningEffortValue(options runtime.ModelOptions, modelValue string, value string) bool {
	modelOption, ok := findModelOption(options, modelValue)
	if !ok {
		return false
	}
	for _, effort := range modelOption.ReasoningEfforts {
		if effort.Value == value {
			return true
		}
	}
	return false
}

func modelInitialReasoningEffort(options runtime.ModelOptions, modelValue string) string {
	modelOption, ok := findModelOption(options, modelValue)
	if !ok || len(modelOption.ReasoningEfforts) == 0 {
		return ""
	}
	return modelOption.ReasoningEfforts[0].Value
}

func modelOptionsSupportInput(options runtime.ModelOptions, inputFormat string) bool {
	for _, modelOption := range options.Models {
		for _, format := range modelOption.InputFormats {
			if format == inputFormat {
				return true
			}
		}
	}
	return false
}

func findModelOption(options runtime.ModelOptions, value string) (runtime.ModelOption, bool) {
	for _, model := range options.Models {
		if model.Value == value {
			return model, true
		}
	}
	return runtime.ModelOption{}, false
}
