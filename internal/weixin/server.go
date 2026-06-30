package weixin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

const (
	defaultSessionListLimit = 10
	maxWeixinTextLength     = 3500
)

// Runtime describes the Atlas core capabilities needed by the WeChat channel.
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error)
	ListSessions(context.Context, int) ([]session.Session, error)
	ListSessionsForCWD(context.Context, string, int) ([]session.Session, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
	RunMemoryWorker(context.Context) error
}

// ServerOptions describes the WeChat channel server parameters.
type ServerOptions struct {
	Runtime    Runtime
	Store      *Store
	Client     *Client
	Account    Account
	Output     io.Writer
	PollDelay  time.Duration
	CDNBaseURL string
}

// Server connects to the WeChat iLink Bot and forwards messages to the Atlas runtime.
type Server struct {
	rt         Runtime
	store      *Store
	client     *Client
	account    Account
	output     io.Writer
	pollDelay  time.Duration
	cdnBaseURL string

	stateMu sync.Mutex
	active  map[string]context.CancelFunc
}

// NewServer creates a WeChat channel server.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Runtime == nil {
		return nil, fmt.Errorf("weixin runtime is required")
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("weixin store is required")
	}
	if opts.Client == nil {
		return nil, fmt.Errorf("weixin client is required")
	}
	if opts.Account.ID == "" || opts.Account.UserID == "" {
		return nil, fmt.Errorf("weixin account is required")
	}
	if opts.Output == nil {
		opts.Output = io.Discard
	}
	if opts.PollDelay <= 0 {
		opts.PollDelay = 2 * time.Second
	}
	cdnBaseURL := strings.TrimRight(strings.TrimSpace(opts.CDNBaseURL), "/")
	if cdnBaseURL == "" {
		cdnBaseURL = defaultCDNBaseURL
	}
	return &Server{
		rt:         opts.Runtime,
		store:      opts.Store,
		client:     opts.Client,
		account:    opts.Account,
		output:     opts.Output,
		pollDelay:  opts.PollDelay,
		cdnBaseURL: cdnBaseURL,
		active:     map[string]context.CancelFunc{},
	}, nil
}

// Run starts long-polling WeChat messages until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		_ = s.rt.RunMemoryWorker(workerCtx)
	}()

	err := s.pollLoop(ctx)
	<-workerDone
	return err
}

// pollLoop runs the WeChat long-polling loop until ctx is cancelled.
func (s *Server) pollLoop(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		state, err := s.loadState()
		if err != nil {
			return err
		}
		resp, err := s.client.GetUpdates(ctx, state.GetUpdatesBuf, 35*time.Second)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			fmt.Fprintf(s.output, "weixin getupdates failed: %v\n", err)
			if !sleepContext(ctx, s.pollDelay) {
				return nil
			}
			continue
		}
		if resp.GetUpdatesBuf != "" && resp.GetUpdatesBuf != state.GetUpdatesBuf {
			if err := s.updateState(func(state *channelState) {
				state.GetUpdatesBuf = resp.GetUpdatesBuf
			}); err != nil {
				return err
			}
		}
		for _, msg := range resp.Messages {
			if err := s.HandleMessage(ctx, msg); err != nil {
				fmt.Fprintf(s.output, "weixin message failed: %v\n", err)
			}
		}
	}
}

// HandleMessage processes a single WeChat message.
func (s *Server) HandleMessage(ctx context.Context, msg WeixinMessage) error {
	if msg.MessageType != messageTypeUser {
		return nil
	}
	if msg.FromUserID != s.account.UserID {
		return nil
	}
	parts, err := s.extractParts(ctx, msg.Items)
	if err != nil {
		return s.client.SendText(ctx, msg.FromUserID, "Atlas error: "+err.Error(), msg.ContextToken, msg.RunID)
	}
	body := strings.TrimSpace(model.TextFromParts(parts))
	if strings.HasPrefix(body, "/") {
		if hasImagePart(parts) {
			return s.reply(ctx, msg, "Slash commands do not support images.")
		}
		return s.handleSlash(ctx, msg, body)
	}
	if body == "" && hasImagePart(parts) {
		parts = append([]model.ContentPart{{Type: model.ContentPartText, Text: "请分析这张图片。"}}, parts...)
		body = "请分析这张图片。"
	}
	if body == "" {
		return s.client.SendText(ctx, msg.FromUserID, "Atlas currently supports text and image messages.", msg.ContextToken, msg.RunID)
	}
	return s.startTurn(ctx, msg, body, parts)
}

// handleSlash handles local control commands in WeChat chat.
func (s *Server) handleSlash(ctx context.Context, msg WeixinMessage, body string) error {
	fields := strings.Fields(body)
	command := strings.ToLower(fields[0])
	args := fields[1:]
	switch command {
	case "/help":
		return s.reply(ctx, msg, weixinHelpText())
	case "/status":
		state, err := s.senderState(msg.FromUserID)
		if err != nil {
			return err
		}
		return s.reply(ctx, msg, fmt.Sprintf("cwd: %s\nsession: %s", state.CWD, displaySessionID(state.SessionID)))
	case "/cwd":
		return s.handleCWD(ctx, msg, slashCommandRest(body))
	case "/new":
		return s.updateSender(ctx, msg.FromUserID, func(state SenderState) (SenderState, string, error) {
			state.SessionID = ""
			return state, fmt.Sprintf("Started a new conversation in %s.", state.CWD), nil
		}, msg)
	case "/sessions":
		all := len(args) > 0 && strings.EqualFold(args[0], "all")
		return s.handleSessions(ctx, msg, all)
	case "/resume":
		if len(args) != 1 {
			return s.reply(ctx, msg, "Usage: /resume <session-id>")
		}
		return s.handleResume(ctx, msg, args[0])
	case "/compact":
		return s.handleCompact(ctx, msg)
	case "/cancel":
		return s.handleCancel(ctx, msg)
	default:
		return s.reply(ctx, msg, "Unknown command. Send /help for available commands.")
	}
}

// handleCWD reads or switches the sender's current working directory.
func (s *Server) handleCWD(ctx context.Context, msg WeixinMessage, target string) error {
	if target == "" {
		state, err := s.senderState(msg.FromUserID)
		if err != nil {
			return err
		}
		return s.reply(ctx, msg, state.CWD)
	}
	return s.updateSender(ctx, msg.FromUserID, func(state SenderState) (SenderState, string, error) {
		if target == "-" {
			if state.PreviousCWD == "" {
				return state, "", fmt.Errorf("previous cwd is empty")
			}
			target = state.PreviousCWD
		}
		if !filepath.IsAbs(target) {
			return state, "", fmt.Errorf("cwd must be an absolute path")
		}
		info, err := os.Stat(target)
		if err != nil {
			return state, "", err
		}
		if !info.IsDir() {
			return state, "", fmt.Errorf("cwd is not a directory")
		}
		if target != state.CWD {
			state.PreviousCWD = state.CWD
			state.CWD = target
			state.SessionID = ""
		}
		return state, fmt.Sprintf("Switched cwd to %s. The next message will start a new conversation.", state.CWD), nil
	}, msg)
}

// slashCommandRest returns the raw parameter text after the command name.
func slashCommandRest(body string) string {
	body = strings.TrimSpace(body)
	for index, char := range body {
		if unicode.IsSpace(char) {
			return strings.TrimSpace(body[index:])
		}
	}
	return ""
}

// handleSessions returns the recent session list for the current directory or globally.
func (s *Server) handleSessions(ctx context.Context, msg WeixinMessage, all bool) error {
	state, err := s.senderState(msg.FromUserID)
	if err != nil {
		return err
	}
	var sessions []session.Session
	if all {
		sessions, err = s.rt.ListSessions(ctx, defaultSessionListLimit)
	} else {
		sessions, err = s.rt.ListSessionsForCWD(ctx, state.CWD, defaultSessionListLimit)
	}
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return s.reply(ctx, msg, "No sessions.")
	}
	return s.reply(ctx, msg, formatSessionList(sessions, all))
}

// handleResume binds the sender's subsequent messages to the specified Atlas session.
func (s *Server) handleResume(ctx context.Context, msg WeixinMessage, sessionID string) error {
	info, _, err := s.rt.ShowSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.updateSender(ctx, msg.FromUserID, func(state SenderState) (SenderState, string, error) {
		if state.CWD != info.CWD {
			state.PreviousCWD = state.CWD
		}
		state.CWD = info.CWD
		state.SessionID = info.ID
		return state, fmt.Sprintf("Resumed session %s in %s.", info.ID, info.CWD), nil
	}, msg)
}

// handleCompact performs manual context compaction on the currently bound session.
func (s *Server) handleCompact(ctx context.Context, msg WeixinMessage) error {
	state, err := s.senderState(msg.FromUserID)
	if err != nil {
		return err
	}
	if state.SessionID == "" {
		return s.reply(ctx, msg, "No active session to compact.")
	}
	result, err := s.rt.CompactSession(ctx, runtime.CompactOptions{SessionID: state.SessionID, CWD: state.CWD})
	if err != nil {
		return err
	}
	if !result.Compacted {
		return s.reply(ctx, msg, "Session was not compacted: "+result.Reason)
	}
	return s.reply(ctx, msg, fmt.Sprintf("Compacted %d messages, kept %d messages.", result.CompactCount, result.KeepCount))
}

// handleCancel cancels the sender's currently running turn.
func (s *Server) handleCancel(ctx context.Context, msg WeixinMessage) error {
	s.stateMu.Lock()
	cancel := s.active[msg.FromUserID]
	s.stateMu.Unlock()
	if cancel == nil {
		return s.reply(ctx, msg, "No running turn.")
	}
	cancel()
	return s.reply(ctx, msg, "Cancelled current turn.")
}

// startTurn starts an asynchronous Atlas turn for a regular message.
func (s *Server) startTurn(ctx context.Context, msg WeixinMessage, prompt string, parts []model.ContentPart) error {
	state, err := s.senderState(msg.FromUserID)
	if err != nil {
		return err
	}
	turnCtx, cancel := context.WithCancel(ctx)
	s.stateMu.Lock()
	if s.active[msg.FromUserID] != nil {
		s.stateMu.Unlock()
		cancel()
		return s.reply(ctx, msg, "A turn is already running. Send /cancel to stop it.")
	}
	s.active[msg.FromUserID] = cancel
	s.stateMu.Unlock()

	go s.runTurn(turnCtx, msg, prompt, parts, state)
	return nil
}

// runTurn calls the runtime and sends the final reply back to WeChat.
func (s *Server) runTurn(ctx context.Context, msg WeixinMessage, prompt string, parts []model.ContentPart, state SenderState) {
	defer func() {
		s.stateMu.Lock()
		delete(s.active, msg.FromUserID)
		s.stateMu.Unlock()
	}()
	s.ensureTypingTicket(ctx, msg.ContextToken)
	s.sendTyping(ctx, msg.FromUserID, true)
	defer s.sendTyping(context.Background(), msg.FromUserID, false)

	var progress strings.Builder
	toolUpdatesSent := false
	result, err := s.rt.RunTurn(ctx, runtime.TurnOptions{
		SessionID: state.SessionID,
		Prompt:    prompt,
		Parts:     parts,
		CWD:       state.CWD,
		Observer: func(event agent.Event) {
			if event.Type == agent.EventToolStarted && event.ToolCall.Name != "" {
				title := tool.DisplayTitle(event.ToolCall, state.CWD)
				progress.WriteString(title)
				progress.WriteString("\n")
				if err := s.reply(context.Background(), msg, title); err != nil {
					fmt.Fprintf(s.output, "weixin tool update failed: %v\n", err)
				} else {
					toolUpdatesSent = true
					s.sendTyping(ctx, msg.FromUserID, true)
				}
			}
			if event.Type == agent.EventToolFinished && len(event.ToolMetadata.Todos) > 0 {
				var inProgress []string
				for _, todo := range event.ToolMetadata.Todos {
					if todo.Status == model.TodoStatusInProgress {
						inProgress = append(inProgress, todo.Content)
					}
				}
				if len(inProgress) > 0 {
					text := "进行中: " + strings.Join(inProgress, ", ")
					if err := s.reply(context.Background(), msg, text); err != nil {
						fmt.Fprintf(s.output, "weixin todo update failed: %v\n", err)
					}
				}
			}
		},
	})
	if err != nil {
		s.reply(context.Background(), msg, "Atlas error: "+err.Error())
		return
	}
	if result.SessionID != "" && result.SessionID != state.SessionID {
		if err := s.setSenderSession(msg.FromUserID, result.SessionID); err != nil {
			fmt.Fprintf(s.output, "weixin state save failed: %v\n", err)
		}
	}
	reply := strings.TrimSpace(result.Content)
	if reply == "" && !toolUpdatesSent {
		reply = strings.TrimSpace(progress.String())
	}
	if reply == "" {
		reply = "Done."
	}
	if err := s.reply(context.Background(), msg, reply); err != nil {
		fmt.Fprintf(s.output, "weixin reply failed: %v\n", err)
	}
}

// refreshTypingTicket refreshes the ticket needed for sendtyping.
func (s *Server) refreshTypingTicket(ctx context.Context, contextToken string) error {
	ticket, err := s.client.GetConfig(ctx, s.account.UserID, contextToken)
	if err != nil {
		return err
	}
	return s.updateState(func(state *channelState) {
		state.TypingTicket = ticket
	})
}

// ensureTypingTicket best-effort prepares the typing ticket after receiving message context.
func (s *Server) ensureTypingTicket(ctx context.Context, contextToken string) {
	if contextToken == "" {
		return
	}
	state, err := s.loadState()
	if err != nil || state.TypingTicket != "" {
		return
	}
	_ = s.refreshTypingTicket(ctx, contextToken)
}

// sendTyping best-effort sends or cancels the WeChat typing indicator.
func (s *Server) sendTyping(ctx context.Context, userID string, typing bool) {
	state, err := s.loadState()
	if err != nil || state.TypingTicket == "" {
		return
	}
	if err := s.client.SendTyping(ctx, userID, state.TypingTicket, typing); err != nil {
		fmt.Fprintf(s.output, "weixin sendtyping failed: %v\n", err)
	}
}

// reply sends text in segments according to WeChat message length limits.
func (s *Server) reply(ctx context.Context, msg WeixinMessage, text string) error {
	for _, chunk := range splitText(text, maxWeixinTextLength) {
		if err := s.client.SendText(ctx, msg.FromUserID, chunk, msg.ContextToken, msg.RunID); err != nil {
			return err
		}
	}
	return nil
}

// senderState returns the sender's current state, initializing with the default working directory on first use.
func (s *Server) senderState(sender string) (SenderState, error) {
	state, err := s.loadState()
	if err != nil {
		return SenderState{}, err
	}
	senderState := state.Senders[sender]
	if senderState.CWD == "" {
		senderState.CWD = s.defaultCWD()
		if err := s.updateState(func(state *channelState) {
			state.Senders[sender] = senderState
		}); err != nil {
			return SenderState{}, err
		}
	}
	return senderState, nil
}

// updateSender atomically updates the specified sender's state and replies with the command result.
func (s *Server) updateSender(ctx context.Context, sender string, update func(SenderState) (SenderState, string, error), msg WeixinMessage) error {
	var reply string
	var updateErr error
	if err := s.updateState(func(state *channelState) {
		current := state.Senders[sender]
		if current.CWD == "" {
			current.CWD = s.defaultCWD()
		}
		next, result, err := update(current)
		if err != nil {
			updateErr = err
			return
		}
		reply = result
		state.Senders[sender] = next
	}); err != nil {
		return err
	}
	if updateErr != nil {
		return s.reply(ctx, msg, updateErr.Error())
	}
	return s.reply(ctx, msg, reply)
}

// setSenderSession records the Atlas session currently bound to the sender.
func (s *Server) setSenderSession(sender, sessionID string) error {
	return s.updateState(func(state *channelState) {
		current := state.Senders[sender]
		if current.CWD == "" {
			current.CWD = s.defaultCWD()
		}
		current.SessionID = sessionID
		state.Senders[sender] = current
	})
}

// loadState serially reads the WeChat channel state file.
func (s *Server) loadState() (channelState, error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.store.loadState()
}

// updateState performs a serialized read-modify-write to avoid concurrent overwrites of the state file.
func (s *Server) updateState(update func(*channelState)) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	state, err := s.store.loadState()
	if err != nil {
		return err
	}
	update(&state)
	return s.store.saveState(state)
}

// defaultCWD returns the working directory used for the first conversation in the WeChat channel.
func (s *Server) defaultCWD() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// extractParts extracts text and image content items from a WeChat message.
func (s *Server) extractParts(ctx context.Context, items []MessageItem) ([]model.ContentPart, error) {
	var parts []model.ContentPart
	for _, item := range items {
		if item.Type == messageItemTypeText && item.TextItem != nil {
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: item.TextItem.Text})
			continue
		}
		if item.Type == messageItemTypeImage && item.ImageItem != nil {
			part, err := s.imagePartFromItem(ctx, item)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func hasImagePart(parts []model.ContentPart) bool {
	for _, part := range parts {
		if part.Type == model.ContentPartImage {
			return true
		}
	}
	return false
}

// splitText splits WeChat reply text by rune count.
func splitText(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{" "}
	}
	if limit <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	var chunks []string
	for len(runes) > limit {
		chunks = append(chunks, string(runes[:limit]))
		runes = runes[limit:]
	}
	chunks = append(chunks, string(runes))
	return chunks
}

// displaySessionID returns a session ID suitable for chat display.
func displaySessionID(id string) string {
	if id == "" {
		return "(new)"
	}
	return id
}

// weixinHelpText returns a concise description of WeChat slash commands.
func weixinHelpText() string {
	return strings.Join([]string{
		"Atlas commands:",
		"/status - Show current cwd and session.",
		"/cwd - Show current cwd.",
		"/cwd <absolute-path> - Switch cwd and start a new conversation.",
		"/cwd - - Switch back to the previous cwd.",
		"/new - Start a new conversation in the current cwd.",
		"/sessions - List recent sessions for the current cwd.",
		"/sessions all - List recent sessions across all cwd values.",
		"/resume <session-id> - Resume a session and switch to its cwd.",
		"/compact - Compact the current session context.",
		"/cancel - Cancel the running turn.",
	}, "\n")
}

// formatSessionList formats a session list as a table suitable for WeChat plain-text display.
func formatSessionList(sessions []session.Session, includeCWD bool) string {
	var b strings.Builder
	if includeCWD {
		b.WriteString("Recent sessions (all):\n")
	} else {
		b.WriteString("Recent sessions:\n")
	}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	if includeCWD {
		fmt.Fprintln(w, "ID\tUPDATED\tTITLE\tCWD")
		for _, item := range sessions {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.ID, item.UpdatedAt.Format("2006-01-02 15:04"), tableCell(item.Title, "(untitled)"), tableCell(item.CWD, "-"))
		}
	} else {
		fmt.Fprintln(w, "ID\tUPDATED\tTITLE")
		for _, item := range sessions {
			fmt.Fprintf(w, "%s\t%s\t%s\n", item.ID, item.UpdatedAt.Format("2006-01-02 15:04"), tableCell(item.Title, "(untitled)"))
		}
	}
	_ = w.Flush()
	return strings.TrimRight(b.String(), "\n")
}

// tableCell cleans up consecutive whitespace in table cells.
func tableCell(value, fallback string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return fallback
	}
	return value
}

// sleepContext responds to ctx cancellation during the wait.
func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
