// Package ws exposes Atlas's agent capabilities via WebSocket.
package ws

import (
	"encoding/json"
	"fmt"
)

// ClientMessage holds the common fields for all client-sent messages.
type ClientMessage struct {
	Type            string        `json:"type"`
	SessionID       string        `json:"session_id,omitempty"`
	CWD             string        `json:"cwd,omitempty"`
	Content         string        `json:"content,omitempty"`
	Parts           []ContentPart `json:"parts,omitempty"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Cursor          string        `json:"cursor,omitempty"`
	Limit           int           `json:"limit,omitempty"`
}

// ContentPart describes a single content segment within a message.
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ServerMessage holds the common fields for all server-sent messages.
type ServerMessage struct {
	Type            string         `json:"type"`
	Event           string         `json:"event,omitempty"`
	Step            int            `json:"step,omitempty"`
	Content         string         `json:"content,omitempty"`
	ToolCall        *ToolCallInfo  `json:"tool_call,omitempty"`
	Result          string         `json:"result,omitempty"`
	Error           string         `json:"error,omitempty"`
	HasError        bool           `json:"error_flag,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
	Model           string         `json:"model,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Sessions        []SessionInfo  `json:"sessions,omitempty"`
	Session         *SessionDetail `json:"session,omitempty"`
	Messages        []MessageInfo  `json:"messages,omitempty"`
	Default         string         `json:"default,omitempty"`
	Models          []ModelInfo    `json:"models,omitempty"`
	Skills          []SkillInfo    `json:"skills,omitempty"`
	NextCursor      string         `json:"next_cursor,omitempty"`
	ContextWindow   int            `json:"context_window,omitempty"`
	Compacted       bool           `json:"compacted,omitempty"`
	TokensBefore    int            `json:"tokens_before,omitempty"`
	TokensAfter     int            `json:"tokens_after,omitempty"`
}

// ToolCallInfo describes the display information for a tool call.
type ToolCallInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// SessionInfo describes a single item in a session list.
type SessionInfo struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	CWD             string `json:"cwd"`
	UpdatedAt       string `json:"updated_at"`
	LastTotalTokens int    `json:"last_total_tokens"`
}

// SessionDetail describes the metadata for a single session.
type SessionDetail struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	CWD             string `json:"cwd"`
	UpdatedAt       string `json:"updated_at"`
	LastTotalTokens int    `json:"last_total_tokens"`
}

// MessageInfo describes a single message in a transcript.
type MessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ModelInfo describes a selectable model.
type ModelInfo struct {
	Value            string            `json:"value"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	ContextWindow    int               `json:"context_window"`
	MaxTokens        int               `json:"max_tokens"`
	InputFormats     []string          `json:"input_formats"`
	ReasoningEfforts []ReasoningEffort `json:"reasoning_efforts,omitempty"`
}

// ReasoningEffort describes a reasoning depth option supported by a model.
type ReasoningEffort struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SkillInfo describes an invocable skill.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Client message types.
const (
	MsgPrompt             = "prompt"
	MsgCancel             = "cancel"
	MsgListSessions       = "list_sessions"
	MsgShowSession        = "show_session"
	MsgDeleteSession      = "delete_session"
	MsgCompactSession     = "compact_session"
	MsgModelOptions       = "model_options"
	MsgSetModel           = "set_model"
	MsgSetReasoningEffort = "set_reasoning_effort"
	MsgSkillSummaries     = "skill_summaries"
)

// Server message types.
const (
	MsgEvent              = "event"
	MsgSessions           = "sessions"
	MsgSessionDetail      = "session_detail"
	MsgSessionDeleted     = "session_deleted"
	MsgSessionCompacted   = "session_compacted"
	MsgModelOptionsResp   = "model_options"
	MsgModelSet           = "model_set"
	MsgReasoningEffortSet = "reasoning_effort_set"
	MsgSkills             = "skills"
)

// Event type (corresponding to agent.EventType).
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

// ParseClientMessage parses a client JSON message.
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
