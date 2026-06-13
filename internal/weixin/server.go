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
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

const (
	defaultSessionListLimit = 10
	maxWeixinTextLength     = 3500
)

// Runtime 描述微信通道需要调用的 Atlas 核心能力。
type Runtime interface {
	RunTurn(context.Context, runtime.TurnOptions) (runtime.TurnResult, error)
	CompactSession(context.Context, runtime.CompactOptions) (runtime.CompactResult, error)
	ListSessions(context.Context, int) ([]session.Session, error)
	ListSessionsForCWD(context.Context, string, int) ([]session.Session, error)
	ShowSession(context.Context, string) (session.Session, *transcript.Transcript, error)
}

// ServerOptions 描述微信通道服务参数。
type ServerOptions struct {
	Runtime   Runtime
	Store     *Store
	Client    *Client
	Account   Account
	Config    config.WeixinConfig
	Output    io.Writer
	PollDelay time.Duration
}

// Server 连接微信 iLink Bot，并把消息转发给 Atlas runtime。
type Server struct {
	rt        Runtime
	store     *Store
	client    *Client
	account   Account
	cfg       config.WeixinConfig
	output    io.Writer
	pollDelay time.Duration

	stateMu sync.Mutex
	active  map[string]context.CancelFunc
}

// NewServer 创建微信通道服务。
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
	return &Server{
		rt:        opts.Runtime,
		store:     opts.Store,
		client:    opts.Client,
		account:   opts.Account,
		cfg:       opts.Config,
		output:    opts.Output,
		pollDelay: opts.PollDelay,
		active:    map[string]context.CancelFunc{},
	}, nil
}

// Run 开始长轮询微信消息，直到 ctx 取消。
func (s *Server) Run(ctx context.Context) error {
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

// HandleMessage 处理单条微信消息。
func (s *Server) HandleMessage(ctx context.Context, msg WeixinMessage) error {
	if msg.MessageType != messageTypeUser {
		return nil
	}
	if msg.FromUserID != s.account.UserID {
		return nil
	}
	body := strings.TrimSpace(extractText(msg.Items))
	if body == "" {
		return s.client.SendText(ctx, msg.FromUserID, "Atlas currently supports text messages only.", msg.ContextToken, msg.RunID)
	}
	if strings.HasPrefix(body, "/") {
		return s.handleSlash(ctx, msg, body)
	}
	return s.startTurn(ctx, msg, body)
}

// handleSlash 处理微信聊天中的本地控制命令。
func (s *Server) handleSlash(ctx context.Context, msg WeixinMessage, body string) error {
	fields := strings.Fields(body)
	command := strings.ToLower(fields[0])
	args := fields[1:]
	switch command {
	case "/help":
		return s.reply(ctx, msg, "Commands: /status, /cwd, /cwd <absolute-path>, /cwd -, /new, /sessions, /sessions all, /resume <session-id>, /compact, /cancel")
	case "/status":
		state, err := s.senderState(msg.FromUserID)
		if err != nil {
			return err
		}
		return s.reply(ctx, msg, fmt.Sprintf("cwd: %s\nsession: %s", state.CWD, displaySessionID(state.SessionID)))
	case "/cwd":
		return s.handleCWD(ctx, msg, args)
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

// handleCWD 读取或切换发送人的当前工作目录。
func (s *Server) handleCWD(ctx context.Context, msg WeixinMessage, args []string) error {
	if len(args) == 0 {
		state, err := s.senderState(msg.FromUserID)
		if err != nil {
			return err
		}
		return s.reply(ctx, msg, state.CWD)
	}
	target := args[0]
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

// handleSessions 返回当前目录或全局最近会话列表。
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
	lines := make([]string, 0, len(sessions)+1)
	lines = append(lines, "Recent sessions:")
	for _, item := range sessions {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "(untitled)"
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s", item.ID, item.UpdatedAt.Format("2006-01-02 15:04"), title))
	}
	return s.reply(ctx, msg, strings.Join(lines, "\n"))
}

// handleResume 把发送人的后续消息绑定到指定 Atlas session。
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

// handleCompact 对当前绑定 session 执行手动上下文压缩。
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

// handleCancel 取消发送人当前正在运行的 turn。
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

// startTurn 为普通文本消息启动一次异步 Atlas turn。
func (s *Server) startTurn(ctx context.Context, msg WeixinMessage, prompt string) error {
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

	go s.runTurn(turnCtx, msg, prompt, state)
	return nil
}

// runTurn 调用 runtime 并把最终回复发回微信。
func (s *Server) runTurn(ctx context.Context, msg WeixinMessage, prompt string, state SenderState) {
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
		CWD:       state.CWD,
		Observer: func(event agent.Event) {
			if event.Type == agent.EventToolStarted && event.ToolCall.Name != "" {
				title := tool.DisplayTitle(event.ToolCall)
				progress.WriteString(title)
				progress.WriteString("\n")
				if err := s.reply(context.Background(), msg, title); err != nil {
					fmt.Fprintf(s.output, "weixin tool update failed: %v\n", err)
				} else {
					toolUpdatesSent = true
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

// refreshTypingTicket 刷新 sendtyping 所需的 ticket。
func (s *Server) refreshTypingTicket(ctx context.Context, contextToken string) error {
	ticket, err := s.client.GetConfig(ctx, s.account.UserID, contextToken)
	if err != nil {
		return err
	}
	return s.updateState(func(state *channelState) {
		state.TypingTicket = ticket
	})
}

// ensureTypingTicket 在收到消息上下文后尽力准备输入状态 ticket。
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

// sendTyping 尽力发送或取消微信输入状态。
func (s *Server) sendTyping(ctx context.Context, userID string, typing bool) {
	state, err := s.loadState()
	if err != nil || state.TypingTicket == "" {
		return
	}
	if err := s.client.SendTyping(ctx, userID, state.TypingTicket, typing); err != nil {
		fmt.Fprintf(s.output, "weixin sendtyping failed: %v\n", err)
	}
}

// reply 把文本按微信消息长度限制分片发送。
func (s *Server) reply(ctx context.Context, msg WeixinMessage, text string) error {
	for _, chunk := range splitText(text, maxWeixinTextLength) {
		if err := s.client.SendText(ctx, msg.FromUserID, chunk, msg.ContextToken, msg.RunID); err != nil {
			return err
		}
	}
	return nil
}

// senderState 返回发送人的当前状态，首次使用时填入默认工作目录。
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

// updateSender 原子更新指定发送人的状态，并回复命令结果。
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

// setSenderSession 记录发送人当前绑定的 Atlas session。
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

// loadState 串行读取微信通道状态文件。
func (s *Server) loadState() (channelState, error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.store.loadState()
}

// updateState 串行执行一次读改写，避免并发覆盖状态文件。
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

// defaultCWD 返回微信通道首次对话使用的工作目录。
func (s *Server) defaultCWD() string {
	if s.cfg.DefaultCWD != "" {
		return s.cfg.DefaultCWD
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// extractText 提取微信消息中的第一个文本内容项。
func extractText(items []MessageItem) string {
	for _, item := range items {
		if item.Type == messageItemTypeText && item.TextItem != nil {
			return item.TextItem.Text
		}
	}
	return ""
}

// splitText 按 rune 数分割微信回复文本。
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

// displaySessionID 返回适合聊天展示的 session ID。
func displaySessionID(id string) string {
	if id == "" {
		return "(new)"
	}
	return id
}

// sleepContext 在等待期间响应 ctx 取消。
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
