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

// Runtime describes the Atlas core capabilities needed by the WebSocket channel.
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

// ServerOptions describes the WebSocket server parameters.
type ServerOptions struct {
	Runtime Runtime
	Host    string
	Port    int
	Output  interface{ Write([]byte) (int, error) }
}

// Server listens for WebSocket connections and forwards them to the Atlas runtime.
type Server struct {
	rt     Runtime
	host   string
	port   int
	output interface{ Write([]byte) (int, error) }
}

// NewServer creates a WebSocket server.
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

// Run starts the HTTP server and accepts WebSocket connections.
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

	// Start memory worker, consistent with ACP / WeChat channels
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		_ = s.rt.RunMemoryWorker(ctx)
	}()

	err = srv.Serve(ln)
	<-workerDone
	return err
}

// handleWebSocket handles a single WebSocket connection.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any Origin, for LAN use
	})
	if err != nil {
		return
	}
	// Default 32KB limit is too small; mobile screenshots/photos base64 will exceed it. Set to 16MB.
	conn.SetReadLimit(16 << 20)
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancelConn := context.WithCancel(r.Context())
	defer cancelConn()

	state := newConnState()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		msg, err := ParseClientMessage(data)
		if err != nil {
			s.sendError(ctx, conn, "", err.Error())
			continue
		}

		// cancel message cancels the specified session's turn context
		if msg.Type == MsgCancel {
			if msg.SessionID == "" {
				s.sendError(ctx, conn, "", "session_id is required for cancel")
				continue
			}
			state.cancelTurn(msg.SessionID)
			continue
		}

		// prompt message executes in a goroutine without blocking the read loop
		if msg.Type == MsgPrompt {
			s.handlePrompt(ctx, conn, msg, state)
			continue
		}

		// Non-prompt message: synchronous handling, non-blocking
		s.handleMessage(ctx, conn, msg, state)
	}
}

// sessionState maintains the state of a single session.
type sessionState struct {
	mu         sync.Mutex
	cwd        string
	model      string
	turnMu     sync.Mutex
	turnCtx    context.Context
	turnCancel context.CancelFunc
	// running ensures only one turn executes at a time per session.
	running atomic.Bool
}

func (ss *sessionState) setCWD(cwd string) {
	ss.mu.Lock()
	ss.cwd = cwd
	ss.mu.Unlock()
}

func (ss *sessionState) getCWD() string {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.cwd
}

func (ss *sessionState) setModel(model string) {
	ss.mu.Lock()
	ss.model = model
	ss.mu.Unlock()
}

func (ss *sessionState) getModel() string {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.model
}

// connState maintains all session states for a single WebSocket connection.
type connState struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
}

func newConnState() *connState {
	return &connState{sessions: make(map[string]*sessionState)}
}

// getOrCreateSession returns the state for the specified session, creating it if it does not exist.
func (c *connState) getOrCreateSession(id string) *sessionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	ss, ok := c.sessions[id]
	if !ok {
		ss = &sessionState{}
		c.sessions[id] = ss
	}
	return ss
}

// getSession returns the state for the specified session, or nil if not found.
func (c *connState) getSession(id string) *sessionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[id]
}

// cancelTurn cancels the current turn for the specified session.
func (c *connState) cancelTurn(sessionID string) {
	ss := c.getSession(sessionID)
	if ss == nil {
		return
	}
	ss.turnMu.Lock()
	defer ss.turnMu.Unlock()
	if ss.turnCancel != nil {
		ss.turnCancel()
	}
}

// handleMessage dispatches non-prompt client messages to their handlers.
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
		s.sendError(ctx, conn, msg.SessionID, fmt.Sprintf("unknown message type: %s", msg.Type))
	}
}

// handlePrompt handles a user conversation request. RunTurn executes in a goroutine without blocking the read loop.
// Prompts for different sessions can execute concurrently; the second prompt to the same session is rejected.
// running is released before sending the final event, ensuring the client can immediately send the next prompt after receiving turn_finished.
func (s *Server) handlePrompt(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	sessionID := msg.SessionID

	// New session: when sessionID is empty, runtime generates it and returns via turn_finished.
	// Create a temporary sessionState first; running CAS executes inside the goroutine.
	var ss *sessionState
	if sessionID != "" {
		ss = state.getOrCreateSession(sessionID)
		if !ss.running.CompareAndSwap(false, true) {
			s.sendError(ctx, conn, sessionID, "a turn is already running in this session, send cancel first")
			return
		}
	}

	go func() {
		// Panic recovery: ensures handlePrompt releases the turn lock even on panic.
		if ss != nil {
			defer ss.running.Store(false)
		}
		s.runPrompt(ctx, conn, msg, state, sessionID, ss)
	}()
}

// runPrompt executes a turn and pushes events.
func (s *Server) runPrompt(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState, sessionID string, ss *sessionState) {
	// Determine cwd: prefer message value, then cached session state
	cwd := msg.CWD
	if cwd == "" && ss != nil {
		cwd = ss.getCWD()
	}
	if cwd != "" && ss != nil {
		ss.setCWD(cwd)
	}

	// Determine model
	selectedModel := ""
	if ss != nil {
		selectedModel = ss.getModel()
	}

	// Build content parts
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
				// client sends bare base64, server assembles full data URL
				part.DataURL = "data:" + p.MimeType + ";base64," + p.Data
			}
			parts = append(parts, part)
		}
	} else if msg.Content != "" {
		parts = []model.ContentPart{{Type: model.ContentPartText, Text: msg.Content}}
	}

	// Create a cancellable context for the current turn
	// New session scenario: ss is nil, derive from connection-level context
	var turnCtx context.Context
	var turnCancel context.CancelFunc
	if ss != nil {
		ss.turnMu.Lock()
		turnCtx, turnCancel = context.WithCancel(ctx)
		ss.turnCtx = turnCtx
		ss.turnCancel = turnCancel
		ss.turnMu.Unlock()
		defer func() {
			ss.turnMu.Lock()
			ss.turnCtx = nil
			ss.turnCancel = nil
			ss.turnMu.Unlock()
		}()
	} else {
		turnCtx, turnCancel = context.WithCancel(ctx)
		defer turnCancel()
	}

	// Create observer to push events, all events carry sessionID
	observer := s.makeObserver(turnCtx, conn, sessionID)

	opts := runtime.TurnOptions{
		SessionID: sessionID,
		CWD:       cwd,
		Model:     selectedModel,
		Observer:  observer,
		Parts:     parts,
	}

	result, err := s.rt.RunTurn(turnCtx, opts)
	if err != nil {
		// Update sessionID (new session scenario)
		if result.SessionID != "" {
			sessionID = result.SessionID
		}
		// New session scenario: register to connState
		if ss == nil && sessionID != "" {
			ss = state.getOrCreateSession(sessionID)
		}
		// Release turn lock
		if ss != nil {
			ss.running.Store(false)
		}
		if turnCtx.Err() != nil {
			// Turn cancelled: send turn_finished as terminal state
			s.sendEvent(ctx, conn, EventTurnFinished, 0, "", nil, "", "cancelled", false, sessionID)
		} else {
			// Turn error: send error event as terminal state (no turn_finished).
			// Clients must listen for both turn_finished and error as terminal signals.
			s.sendEvent(ctx, conn, EventError, 0, "", nil, "", err.Error(), true, sessionID)
		}
		return
	}

	// Update session ID (new session scenario)
	if result.SessionID != "" {
		sessionID = result.SessionID
		// Register new session to connState
		if ss == nil {
			ss = state.getOrCreateSession(sessionID)
			ss.setCWD(cwd)
			ss.setModel(selectedModel)
		}
	}

	// Release turn lock before sending turn_finished
	if ss != nil {
		ss.running.Store(false)
	}
	s.sendEvent(ctx, conn, EventTurnFinished, 0, "", nil, "", "", false, sessionID)
}

// makeObserver creates an observer that pushes agent events to the WebSocket client.
// All events carry sessionID; clients route events to the correct session accordingly.
func (s *Server) makeObserver(ctx context.Context, conn *websocket.Conn, sessionID string) agent.Observer {
	return func(event agent.Event) {
		switch event.Type {
		case agent.EventTurnStarted:
			s.sendEvent(ctx, conn, EventTurnStarted, event.Step, "", nil, "", "", false, sessionID)
		case agent.EventModelDelta:
			if event.Content != "" {
				s.sendEvent(ctx, conn, EventModelDelta, event.Step, event.Content, nil, "", "", false, sessionID)
			}
		case agent.EventModelReasoningDelta:
			if event.Content != "" {
				s.sendEvent(ctx, conn, EventModelReasoningDelta, event.Step, event.Content, nil, "", "", false, sessionID)
			}
		case agent.EventModelResponse:
			s.sendEvent(ctx, conn, EventModelResponse, event.Step, "", nil, "", "", false, sessionID)
		case agent.EventToolStarted:
			tc := &ToolCallInfo{
				ID:        toolCallID(event),
				Name:      event.ToolCall.Name,
				Title:     tool.DisplayTitle(event.ToolCall, ""),
				Arguments: event.ToolCall.Arguments,
			}
			s.sendEvent(ctx, conn, EventToolStarted, event.Step, "", tc, "", "", false, sessionID)
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
			s.sendEvent(ctx, conn, EventToolFinished, event.Step, "", tc, event.ToolResult, errMsg, errFlag, sessionID)
		}
	}
}

// handleListSessions handles a session list request.
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
		s.sendError(ctx, conn, msg.SessionID, err.Error())
		return
	}

	infos := make([]SessionInfo, 0, len(page.Sessions))
	for _, sess := range page.Sessions {
		infos = append(infos, toSessionInfo(sess))
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSessions, Sessions: infos, NextCursor: page.NextCursor})
}

// handleShowSession handles a session detail request.
func (s *Server) handleShowSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	sess, trans, err := s.rt.ShowSession(ctx, msg.SessionID)
	if err != nil {
		s.sendError(ctx, conn, msg.SessionID, err.Error())
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

// handleDeleteSession handles a session deletion request.
func (s *Server) handleDeleteSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	if err := s.rt.DeleteSessionIfExists(ctx, msg.SessionID); err != nil {
		s.sendError(ctx, conn, msg.SessionID, err.Error())
		return
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSessionDeleted, SessionID: msg.SessionID})
}

// handleCompactSession handles a session compaction request.
func (s *Server) handleCompactSession(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	ss := state.getSession(msg.SessionID)
	selectedModel := ""
	if ss != nil {
		selectedModel = ss.getModel()
	}
	result, err := s.rt.CompactSession(ctx, runtime.CompactOptions{
		SessionID: msg.SessionID,
		Model:     selectedModel,
	})
	if err != nil {
		s.sendError(ctx, conn, msg.SessionID, err.Error())
		return
	}
	s.send(ctx, conn, ServerMessage{
		Type:          MsgSessionCompacted,
		SessionID:     result.SessionID,
		ContextWindow: result.ContextWindow,
	})
}

// handleModelOptions handles a model list request.
func (s *Server) handleModelOptions(ctx context.Context, conn *websocket.Conn) {
	options, err := s.rt.ModelOptions(ctx)
	if err != nil {
		s.sendError(ctx, conn, "", err.Error())
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

// handleSetModel validates and saves the model selected for the specified session.
func (s *Server) handleSetModel(ctx context.Context, conn *websocket.Conn, msg ClientMessage, state *connState) {
	modelValue := strings.TrimSpace(msg.Model)
	if modelValue == "" {
		s.sendError(ctx, conn, msg.SessionID, "model is required")
		return
	}
	if msg.SessionID == "" {
		s.sendError(ctx, conn, "", "session_id is required for set_model")
		return
	}

	options, err := s.rt.ModelOptions(ctx)
	if err != nil {
		s.sendError(ctx, conn, msg.SessionID, err.Error())
		return
	}
	if !hasModel(options, modelValue) {
		s.sendError(ctx, conn, msg.SessionID, fmt.Sprintf("model %q is not configured", modelValue))
		return
	}

	ss := state.getOrCreateSession(msg.SessionID)
	ss.setModel(modelValue)
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

// handleSkillSummaries handles a skill list request.
func (s *Server) handleSkillSummaries(ctx context.Context, conn *websocket.Conn, msg ClientMessage) {
	skills, err := s.rt.SkillSummaries(ctx, msg.CWD)
	if err != nil {
		s.sendError(ctx, conn, msg.SessionID, err.Error())
		return
	}

	infos := make([]SkillInfo, 0, len(skills))
	for _, sk := range skills {
		infos = append(infos, SkillInfo{Name: sk.Name, Description: sk.Description})
	}
	s.send(ctx, conn, ServerMessage{Type: MsgSkills, Skills: infos})
}

// send sends a JSON message to the client.
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

// sendError sends an error message.
func (s *Server) sendError(ctx context.Context, conn *websocket.Conn, sessionID string, errMsg string) {
	s.send(ctx, conn, ServerMessage{Type: MsgEvent, Event: EventError, Error: errMsg, HasError: true, SessionID: sessionID})
}

// sendEvent sends an event message.
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

// toSessionInfo converts session.Session to SessionInfo.
func toSessionInfo(sess session.Session) SessionInfo {
	return SessionInfo{
		ID:              sess.ID,
		Title:           sess.Title,
		CWD:             sess.CWD,
		UpdatedAt:       sess.UpdatedAt.Format(time.RFC3339),
		LastTotalTokens: sess.LastTotalTokens,
	}
}

// toolCallID generates a unique identifier for a tool call.
func toolCallID(event agent.Event) string {
	return fmt.Sprintf("step%d-%s", event.Step, event.ToolCall.ID)
}

// logf writes logs to the configured output.
func (s *Server) logf(format string, args ...any) {
	if s.output != nil {
		fmt.Fprintf(s.output, format+"\n", args...)
	}
}
