// Package protocol 是 Atlas 对客户端开放的接口与数据类型。桌面 GUI 在进程内直接调用；
// 以后的远程入口把它映射成线上的消息格式。
//
// 这个包只有数据，不依赖其他包。
package protocol

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrBusy 表示会话正在运行一轮或修改配置。
	ErrBusy = errors.New("会话正忙")
	// ErrNotFound 表示会话不存在，或者这个连接没有打开它（包括已卸载、已失效）。
	ErrNotFound = errors.New("会话不存在")
	// ErrConflict 表示写入时发现别的进程已写入该会话；会话在本进程内失效，需要重新打开。
	ErrConflict = errors.New("会话已在别处更新，请重新打开")
	// ErrClosed 表示 Atlas 正在关闭。
	ErrClosed = errors.New("Atlas 正在关闭")
	// ErrNothingToCompact 表示上下文里没有可压缩的内容。
	ErrNothingToCompact = errors.New("没有可压缩的内容")
)

// Service 是 Atlas 对一个客户端连接开放的接口。
type Service interface {
	// NewSession 新建会话。会话先只在内存里，保存第一条用户消息时才写库；工作目录此后不变。
	NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error)
	// OpenSession 加载会话并订阅它的更新，不回放历史；历史用 ListHistory 分页读取。
	OpenSession(ctx context.Context, req OpenSessionRequest) (OpenSessionResponse, error)
	// ListSessions 按更新时间倒序列出已保存的会话。
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error)
	// DeleteSession 删除空闲的会话；会话不存在时不报错。
	DeleteSession(ctx context.Context, req DeleteSessionRequest) error
	// CloseSession 取消当前连接对该会话的订阅，不影响正在进行的一轮。会话没有订阅者且
	// 空闲时卸载；若正忙，待这一轮结束后卸载。
	CloseSession(ctx context.Context, req CloseSessionRequest) error
	// RenameSession 修改标题；空标题表示清除。
	RenameSession(ctx context.Context, req RenameSessionRequest) error
	// SendPrompt 发送一条用户消息。受理成功即返回，这一轮在会话内异步运行；停止原因与
	// 出错信息通过 RunStateUpdate 送达。以 / 开头的是命令：/compact 压缩上下文，
	// /技能名 参数 在原文后附上技能全文再运行。
	SendPrompt(ctx context.Context, req SendPromptRequest) (SendPromptResponse, error)
	// CancelTurn 取消正在运行的一轮，该轮以 StopCancelled 结束；没有正在运行的一轮时不报错。
	CancelTurn(ctx context.Context, req CancelTurnRequest) error
	// SetConfigOption 更换模型或思考深度，保存后返回完整的配置项。
	SetConfigOption(
		ctx context.Context, req SetConfigOptionRequest,
	) (SetConfigOptionResponse, error)
	// ListHistory 返回一页历史，以更新的形式给出。
	ListHistory(ctx context.Context, req ListHistoryRequest) (ListHistoryResponse, error)
	// ListUserMessages 返回全部用户消息的摘要，供会话大纲。
	ListUserMessages(
		ctx context.Context, req ListUserMessagesRequest,
	) (ListUserMessagesResponse, error)
}

// Subscriber 接收核心推送的更新，每个连接一个。核心在自己的 goroutine 里同步调用它，
// 实现必须很快返回，不得在里面调用 Service。
type Subscriber interface {
	Receive(n Notification)
}

// ---- 会话 ----

type NewSessionRequest struct {
	Cwd string `json:"cwd"` // 绝对路径
}

type NewSessionResponse struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions"`
	State         SessionState   `json:"state"`
}

type OpenSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type OpenSessionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
	State         SessionState   `json:"state"`
}

// SessionState 是打开会话时它的当前状态，随响应给出，之后由更新推进。
type SessionState struct {
	Cwd      string    `json:"cwd"`
	Title    string    `json:"title,omitempty"`
	Commands []Command `json:"commands,omitempty"`
	Usage    Usage     `json:"usage"`
	Running  bool      `json:"running,omitzero"`
	Since    time.Time `json:"since,omitzero"` // 正在运行的一轮开始的时刻
}

type ListSessionsRequest struct {
	Cwd    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type ListSessionsResponse struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type SessionInfo struct {
	SessionID string    `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	Title     string    `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DeleteSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type CloseSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type RenameSessionRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

// ---- 一轮 ----

type SendPromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type SendPromptResponse struct{}

// StopReason 是一轮结束的原因。模型调用失败不是结束原因，由 RunStateUpdate 的 Error 给出。
type StopReason string

const (
	StopEndTurn         StopReason = "end_turn"
	StopMaxTokens       StopReason = "max_tokens"
	StopMaxTurnRequests StopReason = "max_turn_requests"
	StopCancelled       StopReason = "cancelled"
)

type CancelTurnRequest struct {
	SessionID string `json:"sessionId"`
}

// ---- 配置项 ----

// 配置项的 ID。
const (
	ConfigModel    = "model"
	ConfigThinking = "thinking"
)

// ConfigOption 是一个下拉式的配置项。
type ConfigOption struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	CurrentValue string        `json:"currentValue"`
	Options      []ConfigValue `json:"options"`
}

type ConfigValue struct {
	Value  string `json:"value"`
	Name   string `json:"name"`
	Images bool   `json:"images,omitzero"` // 模型接受图片输入
}

type SetConfigOptionRequest struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     string `json:"value"`
}

type SetConfigOptionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// Command 是可用的斜杠命令。Input 不为空时命令带参数：补全后接着输入，否则选了就执行。
type Command struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Input       *CommandInput `json:"input,omitempty"`
}

type CommandInput struct {
	Hint string `json:"hint"`
}

// ---- 历史与大纲 ----

type ListHistoryRequest struct {
	SessionID string `json:"sessionId"`
	Before    int64  `json:"before,omitzero"` // 这个序号之前的；0 表示最新的一页
	Limit     int    `json:"limit"`
}

type ListHistoryResponse struct {
	Updates []Update `json:"updates"`
	Oldest  int64    `json:"oldest"` // 这一页最早一条的序号；没有记录时为 0
}

type ListUserMessagesRequest struct {
	SessionID string `json:"sessionId"`
	Before    int64  `json:"before"`
}

type ListUserMessagesResponse struct {
	Messages []UserMessageSummary `json:"messages"`
}

// UserMessageSummary 是会话大纲里的一条用户消息。
type UserMessageSummary struct {
	Seq    int64     `json:"seq"`
	Text   string    `json:"text"`            // 输入的文字，不含图片与技能全文
	Images bool      `json:"images,omitzero"` // 带了图片
	At     time.Time `json:"at"`
}

// ---- 内容 ----

// 内容块的种类。
const (
	ContentText  = "text"
	ContentImage = "image"
)

// ContentBlock 是一块内容：文字或图片。
type ContentBlock struct {
	Type     string `json:"type"` // ContentText 或 ContentImage
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // 图片的 base64
	MimeType string `json:"mimeType,omitempty"`
}

// Text 是一块文字。
func Text(s string) ContentBlock { return ContentBlock{Type: ContentText, Text: s} }

// Image 是一块 base64 编码的图片。
func Image(data, mimeType string) ContentBlock {
	return ContentBlock{Type: ContentImage, Data: data, MimeType: mimeType}
}
