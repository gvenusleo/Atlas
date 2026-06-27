package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

// Runtime 描述 WebSocket 通道需要调用的 Atlas 核心能力。
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error)
	ModelOptions(context.Context) (runtime.ModelOptions, error)
	SkillSummaries(context.Context, string) ([]runtime.SkillSummary, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
	ListSessionsPage(context.Context, string, int) (session.ListPage, error)
	ListSessionsForCWDPage(context.Context, string, string, int) (session.ListPage, error)
	DeleteSessionIfExists(context.Context, string) error
	RunMemoryWorker(context.Context) error
}

// ServerOptions 描述 WebSocket 服务参数。
type ServerOptions struct {
	Runtime Runtime
	Host    string
	Port    int
	Output  interface{ Write([]byte) (int, error) }
}

// Server 监听 WebSocket 连接并转发给 Atlas runtime。
type Server struct {
	rt     Runtime
	host   string
	port   int
	output interface{ Write([]byte) (int, error) }
}

// NewServer 创建 WebSocket 服务。
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	host := opts.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := opts.Port
	if port == 0 {
		port = 8765
	}
	return &Server{
		rt:     opts.Runtime,
		host:   host,
		port:   port,
		output: opts.Output,
	}, nil
}

// Run 启动 HTTP 服务并接受 WebSocket 连接。
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := &http.Server{Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	s.logf("ws serving on %s", ln.Addr().String())

	// 启动 memory worker，与 ACP / WeChat 通道一致
	go func() {
		_ = s.rt.RunMemoryWorker(ctx)
	}()

	return srv.Serve(ln)
}

// handleWebSocket 处理单个 WebSocket 连接。
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 允许任意 Origin，局域网使用
	})
	if err != nil {
		return
	}
	// 默认 32KB 限制太小，手机截图/照片 base64 会超限。设为 16MB。
	conn.SetReadLimit(16 << 20)
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancelConn := context.WithCancel(r.Context())
	defer cancelConn()

	state := &connState{}

	// turnRunning 控制同一时间只有一个 turn 在执行。
	var turnRunning atomic.Bool

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		msg, err := ParseClientMessage(data)
		if err != nil {
			s.sendError(ctx, conn, err.Error())
			continue
		}

		// cancel 消息通过取消当前 turn 的 context 实现
		if msg.Type == MsgCancel {
			state.cancelTurn()
			continue
		}

		// prompt 消息在 goroutine 中执行，不阻塞读取循环
		if msg.Type == MsgPrompt {
			if !turnRunning.CompareAndSwap(false, true) {
				s.sendError(ctx, conn, "a turn is already running, send cancel first")
				continue
			}
			go func() {
				// panic 兜底：确保 handlePrompt 即使 panic 也能释放 turn 锁，
				// 避免客户端因锁泄漏而无法再发 prompt。
				defer func() {
					turnRunning.Store(false)
				}()
				s.handlePrompt(ctx, conn, msg, state, &turnRunning)
			}()
			continue
		}

		// 非 prompt 消息：如果 turn 正在运行，排队等待（简化：直接处理，不阻塞）
		s.handleMessage(ctx, conn, msg, state)
	}
}

// connState 维护单个 WebSocket 连接的状态。
type connState struct {
	mu         sync.Mutex
	cwd        string
	sessionID  string
	model      string
	turnMu     sync.Mutex
	turnCtx    context.Context
	turnCancel context.CancelFunc
}

func (s *connState) setCWD(cwd string) {
	s.mu.Lock()
	s.cwd = cwd
	s.mu.Unlock()
}

func (s *connState) setSession(id string) {
	s.mu.Lock()
	s.sessionID = id
	s.mu.Unlock()
}

func (s *connState) setModel(model string) {
	s.mu.Lock()
	s.model = model
	s.mu.Unlock()
}

func (s *connState) getCWD() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

func (s *connState) getSession() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func (s *connState) getModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// startTurn 为当前 turn 创建可取消的 context，并记录到 state。
func (s *connState) startTurn(parent context.Context) context.Context {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	s.turnCtx = ctx
	s.turnCancel = cancel
	return ctx
}

// cancelTurn 取消当前正在运行的 turn。
func (s *connState) cancelTurn() {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turnCancel != nil {
		s.turnCancel()
	}
}

// clearTurn 清除 turn context 引用。
func (s *connState) clearTurn() {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	s.turnCtx = nil
	s.turnCancel = nil
}

// handleMessage 分发非 prompt 客户端消息到对应的处理函数。
func (s *Server) handleMessage(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	switch msg.Type {
	case MsgListSessions:
		s.handleListSessions(ctx, conn, msg)
	case MsgShowSession:
		s.handleShowSession(ctx, conn, msg)
	case MsgDeleteSession:
		s.handleDeleteSession(ctx, conn, msg)
	case MsgCompactSession:
		s.handleCompactSession(ctx, conn, msg, state)
	case MsgModelOptions:
		s.handleModelOptions(ctx, conn)
	case MsgSetModel:
		s.handleSetModel(ctx, conn, msg, state)
	case MsgSkillSummaries:
		s.handleSkillSummaries(ctx, conn, msg)
	default:
		s.sendError(ctx, conn, fmt.Sprintf("unknown message type: %s", msg.Type))
	}
}

// handlePrompt 处理用户对话请求。RunTurn 在 goroutine 中执行，不阻塞读取循环。
// turnRunning 在发送最终事件前释放，确保客户端收到 turn_finished 后可以立即发送下一条 prompt。
func (s *Server) handlePrompt(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState, turnRunning *atomic.Bool) {
	cwd := msg.CWD
	if cwd == "" {
		cwd = state.getCWD()
	}
	if cwd != "" {
		state.setCWD(cwd)
	}

	sessionID := msg.SessionID
	if sessionID == "" {
		sessionID = state.getSession()
	}
	if sessionID != "" {
		state.setSession(sessionID)
	}

	selectedModel := state.getModel()

	// 构建 content parts
	var parts []model.ContentPart
	if len(msg.Parts) > 0 {
		for _, p := range msg.Parts {
			part := model.ContentPart{Type: model.ContentPartType(p.Type)}
			switch model.ContentPartType(p.Type) {
			case model.ContentPartText:
				part.Text = p.Text
			case model.ContentPartImage:
				if p.MimeType == "" || p.Data == "" {
					continue
				}
				part.MimeType = p.MimeType
				// 客户端发送裸 base64，服务端拼接为完整 data URL
				part.DataURL = "data:" + p.MimeType + ";base64," + p.Data
			}
			parts = append(parts, part)
		}
	} else if msg.Content != "" {
		parts = []model.ContentPart{{Type: model.ContentPartText, Text: msg.Content}}
	}

	// 为当前 turn 创建可取消的 context
	turnCtx := state.startTurn(ctx)
	defer state.clearTurn()

	// 创建 observer 推送事件
	observer := s.makeObserver(turnCtx, conn)

	opts := runtime.TurnOptions{
		SessionID: sessionID,
		CWD:       cwd,
		Model:     selectedModel,
		Observer:  observer,
		Parts:     parts,
	}

	result, err := s.rt.RunTurn(turnCtx, opts)
	if err != nil {
		// 释放 turn 锁，确保客户端收到最终事件后可以立即发送下一条 prompt
		turnRunning.Store(false)
		if turnCtx.Err() != nil {
			// turn 被取消：发 turn_finished 作为终态
			s.sendEvent(ctx, conn, EventTurnFinished, 0, "", nil, "", "cancelled", false, sessionID)
		} else {
			// turn 出错：发 error 事件作为终态（不发 turn_finished）。
			// 客户端必须同时监听 turn_finished 和 error 作为终态信号。
			s.sendEvent(ctx, conn, EventError, 0, "", nil, "", err.Error(), true, sessionID)
		}
		return
	}

	// 更新 session ID
	if result.SessionID != "" {
		sessionID = result.SessionID
		state.setSession(sessionID)
	}

	// 释放 turn 锁后再发送 turn_finished
	turnRunning.Store(false)
	s.sendEvent(ctx, conn, EventTurnFinished, 0, "", nil, "", "", false, sessionID)
}

// makeObserver 创建将 agent 事件推送给 WebSocket 客户端的 observer。
func (s *Server) makeObserver(ctx context.Context, conn *websocket.Conn) agent.Observer {
	return func(event agent.Event) {
		switch event.Type {
		case agent.EventTurnStarted:
			s.sendEvent(ctx, conn, EventTurnStarted, event.Step, "", nil, "", "", false, "")
		case agent.EventModelDelta:
			if event.Content != "" {
				s.sendEvent(ctx, conn, EventModelDelta, event.Step, event.Content, nil, "", "", false, "")
			}
		case agent.EventModelReasoningDelta:
			if event.Content != "" {
				s.sendEvent(ctx, conn, EventModelReasoningDelta, event.Step, event.Content, nil, "", "", false, "")
			}
		case agent.EventModelResponse:
			s.sendEvent(ctx, conn, EventModelResponse, event.Step, "", nil, "", "", false, "")
		case agent.EventToolStarted:
			tc := &ToolCallInfo{
				ID:        toolCallID(event),
				Name:      event.ToolCall.Name,
				Title:     tool.DisplayTitle(event.ToolCall, ""),
				Arguments: event.ToolCall.Arguments,
			}
			s.sendEvent(ctx, conn, EventToolStarted, event.Step, "", tc, "", "", false, "")
		case agent.EventToolFinished:
			tc := &ToolCallInfo{
				ID:   toolCallID(event),
				Name: event.ToolCall.Name,
			}
			errFlag := event.ToolError
			errMsg := ""
			if errFlag {
				errMsg = event.ToolResult
			}
			s.sendEvent(ctx, conn, EventToolFinished, event.Step, "", tc, event.ToolResult, errMsg, errFlag, "")
		}
	}
}

// handleListSessions 处理会话列表请求。
func (s *Server) handleListSessions(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	limit := msg.Limit
	if limit <= 0 {
		limit = 50
	}

	var page session.ListPage
	var err error
	if msg.CWD != "" {
		page, err = s.rt.ListSessionsForCWDPage(ctx, msg.CWD, msg.Cursor, limit)
	} else {
		page, err = s.rt.ListSessionsPage(ctx, msg.Cursor, limit)
	}
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}

	infos := make([]SessionInfo, 0, len(page.Sessions))
	for _, sess := range page.Sessions {
		infos = append(infos, toSessionInfo(sess))
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSessions, Sessions: infos, NextCursor: page.NextCursor})
}

// handleShowSession 处理会话详情请求。
func (s *Server) handleShowSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	sess, trans, err := s.rt.ShowSession(ctx, msg.SessionID)
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}

	detail := &SessionDetail{
		ID:              sess.ID,
		Title:           sess.Title,
		CWD:             sess.CWD,
		UpdatedAt:       sess.UpdatedAt.Format(time.RFC3339),
		LastTotalTokens: sess.LastTotalTokens,
	}

	var msgInfos []MessageInfo
	for _, m := range trans.Messages() {
		role := string(m.Role)
		content := model.TextFromParts(model.MessageParts(m))
		if content != "" {
			msgInfos = append(msgInfos, MessageInfo{Role: role, Content: content})
		}
	}

	s.send(ctx, conn, ServerMessage{
		Type:     MsgSessionDetail,
		Session:  detail,
		Messages: msgInfos,
	})
}

// handleDeleteSession 处理会话删除请求。
func (s *Server) handleDeleteSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	if err := s.rt.DeleteSessionIfExists(ctx, msg.SessionID); err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSessionDeleted, SessionID: msg.SessionID})
}

// handleCompactSession 处理会话压缩请求。
func (s *Server) handleCompactSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	result, err := s.rt.CompactSession(ctx, runtime.CompactOptions{
		SessionID: msg.SessionID,
		Model:     state.getModel(),
	})
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}
	s.send(ctx, conn, ServerMessage{
		Type:          MsgSessionCompacted,
		SessionID:     result.SessionID,
		ContextWindow: result.ContextWindow,
	})
}

// handleModelOptions 处理模型列表请求。
func (s *Server) handleModelOptions(ctx context.Context, conn *websocket.Conn) {
	options, err := s.rt.ModelOptions(ctx)
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}

	models := make([]ModelInfo, 0, len(options.Models))
	for _, m := range options.Models {
		mi := ModelInfo{
			Value:         m.Value,
			Name:          m.Name,
			Description:   m.Description,
			ContextWindow: m.ContextWindow,
			MaxTokens:     m.MaxTokens,
			InputFormats:  m.InputFormats,
		}
		for _, re := range m.ReasoningEfforts {
			mi.ReasoningEfforts = append(mi.ReasoningEfforts, ReasoningEffort{
				Value:       re.Value,
				Name:        re.Name,
				Description: re.Description,
			})
		}
		models = append(models, mi)
	}

	s.send(ctx, conn, ServerMessage{
		Type:    MsgModelOptionsResp,
		Default: options.Default,
		Models:  models,
	})
}

// handleSetModel 校验并保存当前连接选择的模型。
func (s *Server) handleSetModel(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	modelValue := strings.TrimSpace(msg.Model)
	if modelValue == "" {
		s.sendError(ctx, conn, "model is required")
		return
	}

	options, err := s.rt.ModelOptions(ctx)
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}
	if !hasModel(options, modelValue) {
		s.sendError(ctx, conn, fmt.Sprintf("model %q is not configured", modelValue))
		return
	}

	state.setModel(modelValue)
	s.send(ctx, conn, ServerMessage{Type: MsgModelSet, SessionID: msg.SessionID, Model: modelValue})
}

func hasModel(options runtime.ModelOptions, value string) bool {
	for _, model := range options.Models {
		if model.Value == value {
			return true
		}
	}
	return false
}

// handleSkillSummaries 处理 skill 列表请求。
func (s *Server) handleSkillSummaries(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	skills, err := s.rt.SkillSummaries(ctx, msg.CWD)
	if err != nil {
		s.sendError(ctx, conn, err.Error())
		return
	}

	infos := make([]SkillInfo, 0, len(skills))
	for _, sk := range skills {
		infos = append(infos, SkillInfo{Name: sk.Name, Description: sk.Description})
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSkills, Skills: infos})
}

// send 发送一条 JSON 消息给客户端。
func (s *Server) send(ctx context.Context, conn *websocket.Conn, msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("ws marshal", "error", err)
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.Write(writeCtx, websocket.MessageText, data); err != nil {
		slog.Debug("ws write", "error", err)
	}
}

// sendError 发送一条错误消息。
func (s *Server) sendError(ctx context.Context, conn *websocket.Conn, errMsg string) {
	s.send(ctx, conn, ServerMessage{Type: MsgEvent, Event: EventError, Error: errMsg, HasError: true})
}

// sendEvent 发送一条事件消息。
func (s *Server) sendEvent(ctx context.Context, conn *websocket.Conn, event string, step int, content string, tc *ToolCallInfo, result string, errMsg string, errFlag bool, sessionID string) {
	s.send(ctx, conn, ServerMessage{
		Type:      MsgEvent,
		Event:     event,
		Step:      step,
		Content:   content,
		ToolCall:  tc,
		Result:    result,
		Error:     errMsg,
		HasError:  errFlag,
		SessionID: sessionID,
	})
}

// toSessionInfo 将 session.Session 转换为 SessionInfo。
func toSessionInfo(sess session.Session) SessionInfo {
	return SessionInfo{
		ID:              sess.ID,
		Title:           sess.Title,
		CWD:             sess.CWD,
		UpdatedAt:       sess.UpdatedAt.Format(time.RFC3339),
		LastTotalTokens: sess.LastTotalTokens,
	}
}

// toolCallID 生成工具调用的唯一标识。
func toolCallID(event agent.Event) string {
	return fmt.Sprintf("step%d-%s", event.Step, event.ToolCall.ID)
}

// logf 输出日志到配置的 output。
func (s *Server) logf(format string, args ...any) {
	if s.output != nil {
		fmt.Fprintf(s.output, format+"\n", args...)
	}
}
