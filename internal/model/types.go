package model

import "context"

// Role identifies the speaker for a model message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is the provider-neutral conversation unit used by Atlas.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall is a model-requested invocation of a local Atlas tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolDefinition describes a tool in the JSON schema shape expected by LLMs.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// StreamEvent is emitted by providers while a response is being generated.
type StreamEvent struct {
	TextDelta string
	ToolCall  *ToolCall
	Done      bool
}

// AssistantResult is the completed assistant message assembled by the agent loop.
type AssistantResult struct {
	Content   string
	ToolCalls []ToolCall
}

// ChatRequest is the complete input for a single model step.
type ChatRequest struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
}

// Provider streams model output for one chat request.
type Provider interface {
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, <-chan error)
}
