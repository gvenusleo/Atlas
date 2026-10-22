// Package protocol 是 Atlas 对客户端开放的接口与数据类型，语义与命名照 ACP v1
// （https://agentclientprotocol.com）。ACP 不够用的地方用 _atlas 扩展补上。
// 桌面 GUI 在进程内直接调用；以后的 ACP、WebSocket 入口把它映射成 JSON-RPC。
// JSON 标签用 ACP 的线上名称；Atlas 的扩展字段放在 _meta 里，键以 atlas/ 开头。
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
	// ErrNotFound 表示会话不存在，或者这个连接还没有打开它。
	ErrNotFound = errors.New("会话不存在")
	// ErrClosed 表示 Atlas 正在关闭。
	ErrClosed = errors.New("Atlas 正在关闭")
	// ErrNothingToCompact 表示上下文里没有可压缩的内容。
	ErrNothingToCompact = errors.New("没有可压缩的内容")
)

// Agent 是 Atlas 对一个客户端连接开放的接口。
type Agent interface {
	// NewSession 建会话。会话先只在内存里，第一条消息保存时才写库；工作目录从此不变。
	NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error)
	// ResumeSession 打开会话并订阅它的更新，不回放历史（ACP session/resume）。
	ResumeSession(ctx context.Context, req ResumeSessionRequest) (ResumeSessionResponse, error)
	// LoadSession 打开会话并订阅，先把全部历史以更新回放，再返回（ACP session/load）。
	LoadSession(ctx context.Context, req LoadSessionRequest) (LoadSessionResponse, error)
	// ListSessions 按更新时间倒序列出已保存的会话。
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error)
	// DeleteSession 删除空闲的会话。
	DeleteSession(ctx context.Context, req DeleteSessionRequest) error
	// CloseSession 表示这个连接不再看这个会话。
	CloseSession(ctx context.Context, req CloseSessionRequest) error
	// Prompt 跑一轮，这一轮结束时才返回。以 / 开头的是命令：/compact 压缩上下文，
	// /技能名 参数 在原文后附上技能全文再运行。
	Prompt(ctx context.Context, req PromptRequest) (PromptResponse, error)
	// Cancel 取消正在运行的一轮；被取消的 Prompt 以 StopCancelled 返回。
	Cancel(req CancelNotification)
	// SetConfigOption 换模型或思考深度，保存后返回完整的配置项。
	SetConfigOption(ctx context.Context, req SetConfigOptionRequest) (SetConfigOptionResponse, error)

	// History 取一页历史，以更新的形式给出（_atlas/session/history）。
	History(ctx context.Context, req HistoryRequest) (HistoryResponse, error)
	// Prompts 取全部用户消息的摘要，供会话目录（_atlas/session/prompts）。
	Prompts(ctx context.Context, req PromptsRequest) (PromptsResponse, error)
	// Rename 改名；空标题表示清除（_atlas/session/rename）。
	Rename(ctx context.Context, req RenameRequest) error
}

// Client 接收核心推送的更新，每个连接一个。它在核心的 goroutine 里被调用，必须很快返回，
// 不得在里面同步调用 Agent。
type Client interface {
	SessionUpdate(n SessionNotification)
}

// ---- 会话 ----

type NewSessionRequest struct {
	Cwd string `json:"cwd"` // 绝对路径
}

type NewSessionResponse struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions"`
	State         SessionState   `json:"_meta"`
}

type ResumeSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type ResumeSessionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
	State         SessionState   `json:"_meta"`
}

type LoadSessionRequest = ResumeSessionRequest

type LoadSessionResponse = ResumeSessionResponse

// SessionState 是打开会话时它的当前状态，随响应给出（在 _meta 里），之后由更新推进。
type SessionState struct {
	Cwd      string    `json:"atlas/cwd"`
	Title    string    `json:"atlas/title,omitempty"`
	Commands []Command `json:"atlas/commands,omitempty"`
	Usage    Usage     `json:"atlas/usage"`
	Running  bool      `json:"atlas/running,omitzero"`
	Since    time.Time `json:"atlas/since,omitzero"` // 正在运行的一轮开始的时刻
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

type RenameRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

// ---- 一轮 ----

type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type PromptResponse struct {
	StopReason StopReason `json:"stopReason"`
}

// StopReason 是一轮结束的原因。模型调用失败不是结束原因，Prompt 返回错误。
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopMaxTokens StopReason = "max_tokens"
	StopCancelled StopReason = "cancelled"
)

type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// ---- 配置项 ----

// 配置项的 ID，也是它的类别。
const (
	ConfigModel    = "model"
	ConfigThinking = "thought_level"
)

// ConfigOption 是一个下拉式的配置项。
type ConfigOption struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Category     string        `json:"category"`
	Type         string        `json:"type"` // 总是 select
	CurrentValue string        `json:"currentValue"`
	Options      []ConfigValue `json:"options"`
}

type ConfigValue struct {
	Value string    `json:"value"`
	Name  string    `json:"name"`
	Meta  ValueMeta `json:"_meta,omitzero"`
}

// ValueMeta 是选项上的 Atlas 扩展。
type ValueMeta struct {
	Images bool `json:"atlas/images,omitzero"` // 模型接受图片输入
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

// ---- 扩展：历史与目录 ----

type HistoryRequest struct {
	SessionID string `json:"sessionId"`
	Before    int64  `json:"before,omitzero"` // 这个序号之前的；0 表示最新的一页
	Limit     int    `json:"limit"`
}

type HistoryResponse struct {
	Updates []SessionUpdate `json:"updates"`
	Oldest  int64           `json:"oldest"` // 这一页最早一条的序号；没有记录时为 0
}

type PromptsRequest struct {
	SessionID string `json:"sessionId"`
	Before    int64  `json:"before"`
}

type PromptsResponse struct {
	Prompts []PromptEntry `json:"prompts"`
}

// PromptEntry 是会话目录里的一条用户消息。
type PromptEntry struct {
	Seq    int64     `json:"seq"`
	Text   string    `json:"text"`            // 输入的文字，不含图片与技能全文
	Images bool      `json:"images,omitzero"` // 带了图片
	At     time.Time `json:"at"`
}

// ---- 内容 ----

// ContentBlock 是一块内容：文字或图片。
type ContentBlock struct {
	Type     string `json:"type"` // text 或 image
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // 图片的 base64
	MimeType string `json:"mimeType,omitempty"`
}

// Text 是一块文字。
func Text(s string) ContentBlock { return ContentBlock{Type: "text", Text: s} }

// Image 是一块 base64 编码的图片。
func Image(data, mimeType string) ContentBlock {
	return ContentBlock{Type: "image", Data: data, MimeType: mimeType}
}
