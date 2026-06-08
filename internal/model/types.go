package model

// Role 表示聊天消息的角色。
type Role string

const (
	// RoleSystem 表示系统级指令。
	RoleSystem Role = "system"
	// RoleUser 表示用户输入。
	RoleUser Role = "user"
	// RoleAssistant 表示模型输出。
	RoleAssistant Role = "assistant"
	// RoleTool 表示工具调用结果。
	RoleTool Role = "tool"
)

// ToolCall 表示模型请求执行的一个本地 Atlas 工具。
type ToolCall struct {
	// ID 由 provider 生成，匹配的工具结果必须带回同一个 ID。
	ID   string
	Name string
	// Arguments 是模型输出的原始 JSON 对象字符串。
	// agent 匹配工具名后，再由具体工具实现解析参数。
	Arguments string
}

// ToolDefinition 描述发送给模型的函数调用工具定义。
type ToolDefinition struct {
	Name        string
	Description string
	// Parameters 是 JSON Schema 对象。model 包保持弱类型，避免为 schema 引入额外依赖。
	Parameters map[string]any
}

// Message 是 agent 循环使用的通用对话单元。
type Message struct {
	Role    Role
	Content string
	// ToolCalls 只对 assistant 消息有意义。
	ToolCalls []ToolCall
	// ToolCallID 只对 tool 消息有意义，用来关联对应的工具调用。
	ToolCallID string
}

// StopReason 表示一次模型 step 结束的原因。
type StopReason string

const (
	// StopEndTurn 表示模型已经给出终止本 turn 的 assistant 回复。
	StopEndTurn StopReason = "end_turn"
	// StopToolUse 表示模型请求工具调用，agent 执行工具后应继续下一步。
	StopToolUse StopReason = "tool_use"
	// StopMaxTokens 表示 provider 因输出 token 上限停止。
	StopMaxTokens StopReason = "max_tokens"
	// StopUnknown 表示无法归类的 provider 原始结束原因。
	StopUnknown StopReason = "unknown"
)

// Usage 记录 provider 返回的 token 用量。
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// StreamEventType 表示 provider stream 中的增量事件类型。
type StreamEventType string

const (
	// StreamTextDelta 表示 assistant 文本增量。
	StreamTextDelta StreamEventType = "text_delta"
)

// StreamEvent 是 provider 在一次模型 step 中产生的实时事件。
type StreamEvent struct {
	Type  StreamEventType
	Delta string
}

// ChatRequest 是一次模型 step 的完整通用输入。
type ChatRequest struct {
	System      string
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
}

// ChatResponse 是一次模型 step 的完整通用输出。
type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason StopReason
	Usage      Usage
	// RawFinish 保留 provider 的原始结束原因，方便调试。
	RawFinish string
}
