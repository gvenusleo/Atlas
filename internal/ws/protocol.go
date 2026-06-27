// Package ws 通过 WebSocket 暴露 Atlas 的 Agent 能力。
package ws

import (
	"encoding/json"
	"fmt"
)

// ClientMessage 是客户端发送的所有消息的公共字段。
type ClientMessage struct {
	Type      string        `json:"type"`
	SessionID string        `json:"session_id,omitempty"`
	CWD       string        `json:"cwd,omitempty"`
	Content   string        `json:"content,omitempty"`
	Parts     []ContentPart `json:"parts,omitempty"`
	Model     string        `json:"model,omitempty"`
	Cursor    string        `json:"cursor,omitempty"`
	Limit     int           `json:"limit,omitempty"`
}

// ContentPart 描述一条消息中的一个内容片段。
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ServerMessage 是服务端发送的所有消息的公共字段。
type ServerMessage struct {
	Type          string         `json:"type"`
	Event         string         `json:"event,omitempty"`
	Step          int            `json:"step,omitempty"`
	Content       string         `json:"content,omitempty"`
	ToolCall      *ToolCallInfo  `json:"tool_call,omitempty"`
	Result        string         `json:"result,omitempty"`
	Error         string         `json:"error,omitempty"`
	HasError      bool           `json:"error_flag,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	Model         string         `json:"model,omitempty"`
	Sessions      []SessionInfo  `json:"sessions,omitempty"`
	Session       *SessionDetail `json:"session,omitempty"`
	Messages      []MessageInfo  `json:"messages,omitempty"`
	Default       string         `json:"default,omitempty"`
	Models        []ModelInfo    `json:"models,omitempty"`
	Skills        []SkillInfo    `json:"skills,omitempty"`
	NextCursor    string         `json:"next_cursor,omitempty"`
	ContextWindow int            `json:"context_window,omitempty"`
}

// ToolCallInfo 描述一个工具调用的展示信息。
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// SessionInfo 描述会话列表中的一项。
type SessionInfo struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	CWD             string `json:"cwd"`
	UpdatedAt       string `json:"updated_at"`
	LastTotalTokens int    `json:"last_total_tokens"`
}

// SessionDetail 描述单个会话的元数据。
type SessionDetail struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	CWD             string `json:"cwd"`
	UpdatedAt       string `json:"updated_at"`
	LastTotalTokens int    `json:"last_total_tokens"`
}

// MessageInfo 描述 transcript 中的一条消息。
type MessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ModelInfo 描述一个可选模型。
type ModelInfo struct {
	Value            string            `json:"value"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	ContextWindow    int               `json:"context_window"`
	MaxTokens        int               `json:"max_tokens"`
	InputFormats     []string          `json:"input_formats"`
	ReasoningEfforts []ReasoningEffort `json:"reasoning_efforts,omitempty"`
}

// ReasoningEffort 描述模型支持的一个思考深度选项。
type ReasoningEffort struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SkillInfo 描述一个可调用 skill。
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// 客户端消息类型。
const (
	MsgPrompt         = "prompt"
	MsgCancel         = "cancel"
	MsgListSessions   = "list_sessions"
	MsgShowSession    = "show_session"
	MsgDeleteSession  = "delete_session"
	MsgCompactSession = "compact_session"
	MsgModelOptions   = "model_options"
	MsgSetModel       = "set_model"
	MsgSkillSummaries = "skill_summaries"
)

// 服务端消息类型。
const (
	MsgEvent            = "event"
	MsgSessions         = "sessions"
	MsgSessionDetail    = "session_detail"
	MsgSessionDeleted   = "session_deleted"
	MsgSessionCompacted = "session_compacted"
	MsgModelOptionsResp = "model_options"
	MsgModelSet         = "model_set"
	MsgSkills           = "skills"
)

// 事件类型（对应 agent.EventType）。
const (
	EventTurnStarted         = "turn_started"
	EventModelDelta          = "model_delta"
	EventModelReasoningDelta = "model_reasoning_delta"
	EventModelResponse       = "model_response"
	EventToolStarted         = "tool_started"
	EventToolFinished        = "tool_finished"
	EventTurnFinished        = "turn_finished"
	EventError               = "error"
)

// ParseClientMessage 解析客户端 JSON 消息。
func ParseClientMessage(data []byte) (ClientMessage, error) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return ClientMessage{}, fmt.Errorf("parse message: %w", err)
	}
	if msg.Type == "" {
		return ClientMessage{}, fmt.Errorf("message type is required")
	}
	return msg, nil
}
