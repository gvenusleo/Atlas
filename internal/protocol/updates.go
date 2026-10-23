package protocol

import (
	"encoding/json/jsontext"
	"time"
)

// Notification 是一条推给订阅者的更新。
type Notification struct {
	SessionID string `json:"sessionId"`
	Update    Update `json:"update"`
}

// Update 是会话的一个更新，具体类型见本文件；方法 Type 返回它的种类名。
type Update interface {
	Type() string
}

// UserMessageChunk 是用户消息的一块。消息保存之后才发出，发消息的客户端也只在收到它时显示。
type UserMessageChunk struct {
	MessageID string       `json:"messageId"`
	Content   ContentBlock `json:"content"`
	Seq       int64        `json:"seq,omitzero"`    // 消息在历史里的序号
	At        time.Time    `json:"at,omitzero"`     // 消息的时间
	Skill     string       `json:"skill,omitempty"` // 这块是显式调用的技能全文，值是技能名
}

// AgentMessageChunk 是助手回复的一段文字。
type AgentMessageChunk struct {
	MessageID string       `json:"messageId"`
	Content   ContentBlock `json:"content"`
	At        time.Time    `json:"at,omitzero"` // 这条回复开始的时间
}

// AgentThoughtChunk 是助手的一段思考。
type AgentThoughtChunk struct {
	MessageID string       `json:"messageId"`
	Content   ContentBlock `json:"content"`
}

// ToolKind 是工具调用的种类。
type ToolKind string

const (
	ToolRead    ToolKind = "read"
	ToolEdit    ToolKind = "edit"
	ToolExecute ToolKind = "execute"
	ToolOther   ToolKind = "other"
)

// ToolStatus 是工具调用的状态。
type ToolStatus string

const (
	ToolPending    ToolStatus = "pending"
	ToolInProgress ToolStatus = "in_progress"
	ToolCompleted  ToolStatus = "completed"
	ToolFailed     ToolStatus = "failed"
)

// ToolCall 是模型发出的一个工具调用。RawInput 是参数原文，流式期间可能不完整。
type ToolCall struct {
	ToolCallID string         `json:"toolCallId"`
	Name       string         `json:"name"`
	Title      string         `json:"title"`
	Kind       ToolKind       `json:"kind"`
	Status     ToolStatus     `json:"status"`
	RawInput   jsontext.Value `json:"rawInput,omitempty"`
	// MessageID 是发出这个调用的助手消息；重试时调用随这条消息一起作废。
	MessageID string `json:"messageId,omitempty"`
}

// ToolCallUpdate 更新一个工具调用，零值字段表示不变。
type ToolCallUpdate struct {
	ToolCallID string            `json:"toolCallId"`
	Status     ToolStatus        `json:"status,omitempty"`
	RawInput   jsontext.Value    `json:"rawInput,omitempty"`
	Content    []ToolCallContent `json:"content,omitempty"`
	// RawOutput 是工具附带的结构化数据，形状见 ReadDetails 等。
	RawOutput jsontext.Value `json:"rawOutput,omitempty"`
}

// ToolCallContent 是工具结果的一块内容。
type ToolCallContent struct {
	Type    string       `json:"type"` // 总是 content
	Content ContentBlock `json:"content"`
}

// UsageUpdate 是上下文的占用：Used 是当前上下文的 token 数，Size 是窗口（未知时为 0）。
type UsageUpdate struct {
	Usage
}

// Usage 是上下文的占用。
type Usage struct {
	Used          int64 `json:"used"`
	ContextWindow int64 `json:"contextWindow"` // 未知时为 0
}

// CompactionStatus 是压缩的状态。
type CompactionStatus string

const (
	CompactionInProgress CompactionStatus = "in_progress"
	CompactionCompleted  CompactionStatus = "completed"
	CompactionFailed     CompactionStatus = "failed"
	CompactionCancelled  CompactionStatus = "cancelled"
)

// CompactionUpdate 报告压缩的进展；完成时带摘要，失败时带原因。
type CompactionUpdate struct {
	CompactionID string           `json:"compactionId"`
	Status       CompactionStatus `json:"status"`
	Summary      string           `json:"summary,omitempty"`
	Error        string           `json:"error,omitempty"`
}

// Notice 是一条系统通知。
type Notice struct {
	Severity string `json:"severity"` // info、warning 或 error
	Title    string `json:"title"`
}

// SessionInfoUpdate 报告会话的标题变了：第一次保存时有了标题，或者改了名。
type SessionInfoUpdate struct {
	Title string `json:"title"`
}

// ConfigOptionUpdate 是别处改了配置项之后的完整配置项。
type ConfigOptionUpdate struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// RunStateUpdate 报告一轮开始或结束，中途接入的客户端靠它知道会话在不在运行。
// 结束时（Running 为 false）带着停止原因，或者出错的原因。
type RunStateUpdate struct {
	Running    bool       `json:"running"`
	Since      time.Time  `json:"since,omitzero"`
	StopReason StopReason `json:"stopReason,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// RequestRetryUpdate 报告一次模型请求失败、等待重试：这条消息已流出的内容作废。
type RequestRetryUpdate struct {
	MessageID string        `json:"messageId"`
	Attempt   int           `json:"attempt"`
	Max       int           `json:"max"`
	Delay     time.Duration `json:"delay"`
	Error     string        `json:"error"`
}

func (UserMessageChunk) Type() string   { return "user_message_chunk" }
func (AgentMessageChunk) Type() string  { return "agent_message_chunk" }
func (AgentThoughtChunk) Type() string  { return "agent_thought_chunk" }
func (ToolCall) Type() string           { return "tool_call" }
func (ToolCallUpdate) Type() string     { return "tool_call_update" }
func (UsageUpdate) Type() string        { return "usage_update" }
func (CompactionUpdate) Type() string   { return "compaction_update" }
func (Notice) Type() string             { return "notice" }
func (SessionInfoUpdate) Type() string  { return "session_info_update" }
func (ConfigOptionUpdate) Type() string { return "config_option_update" }
func (RunStateUpdate) Type() string     { return "run_state" }
func (RequestRetryUpdate) Type() string { return "request_retry" }
