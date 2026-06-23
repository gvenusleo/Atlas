// Package model 定义 Atlas 内部使用的模型消息和工具调用协议。
package model

import "strings"

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

// ToolLocation 描述工具调用关联的文件位置。
type ToolLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// ToolDiff 描述工具调用产生的文件内容变更。
type ToolDiff struct {
	Path    string  `json:"path"`
	OldText *string `json:"old_text,omitempty"`
	NewText string  `json:"new_text"`
}

// ToolMetadata 保存界面可用的工具调用展示数据。
type ToolMetadata struct {
	Locations []ToolLocation `json:"locations,omitempty"`
	Diff      *ToolDiff      `json:"diff,omitempty"`
}

// ContentPartType 表示一段消息内容的模态类型。
type ContentPartType string

const (
	// ContentPartText 表示普通文本内容。
	ContentPartText ContentPartType = "text"
	// ContentPartImage 表示内联图片内容。
	ContentPartImage ContentPartType = "image"
)

// ImageDetail 表示模型处理图片时的清晰度偏好。
type ImageDetail string

const (
	// ImageDetailAuto 让 provider 或模型自动选择图片清晰度。
	ImageDetailAuto ImageDetail = "auto"
	// ImageDetailLow 表示低清晰度图片输入。
	ImageDetailLow ImageDetail = "low"
	// ImageDetailHigh 表示高清晰度图片输入。
	ImageDetailHigh ImageDetail = "high"
)

// ContentPart 描述一条消息中的一个文本或图片片段。
type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	MimeType string          `json:"mime_type,omitempty"`
	DataURL  string          `json:"data_url,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Detail   ImageDetail     `json:"detail,omitempty"`
}

// ProviderItem 保存特定 API 格式下一轮必须原样回放的输出项。
type ProviderItem struct {
	Type string `json:"type"`
	JSON string `json:"json"`
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
	// Parts 保存模型可见的结构化内容；为空时 Content 会作为单个文本片段使用。
	Parts []ContentPart
	// ReasoningContent 只对 assistant 消息有意义，用于支持 provider 的思维链续接。
	ReasoningContent string
	// ToolCalls 只对 assistant 消息有意义。
	ToolCalls []ToolCall
	// ToolCallID 只对 tool 消息有意义，用来关联对应的工具调用。
	ToolCallID string
	// ToolMetadata 只对 tool 消息有意义，用来支持 ACP 等客户端展示结构化结果。
	ToolMetadata ToolMetadata
	// Usage 只对 assistant 消息有意义，记录 provider 返回的 token 用量。
	Usage Usage
	// ProviderItems 只对 assistant 消息有意义，用于 provider 续接特定 API 状态。
	ProviderItems []ProviderItem
}

// TextMessage 构造包含单个文本片段的消息。
func TextMessage(role Role, content string) Message {
	return Message{
		Role:    role,
		Content: content,
		Parts:   []ContentPart{{Type: ContentPartText, Text: content}},
	}
}

// MessageParts 返回消息的结构化内容；旧消息自动回退为文本片段。
func MessageParts(msg Message) []ContentPart {
	if len(msg.Parts) > 0 {
		parts := make([]ContentPart, len(msg.Parts))
		copy(parts, msg.Parts)
		return parts
	}
	if msg.Content == "" {
		return nil
	}
	return []ContentPart{{Type: ContentPartText, Text: msg.Content}}
}

// TextFromParts 提取内容片段中的文本，供标题、记忆和兼容字段使用。
func TextFromParts(parts []ContentPart) string {
	var text string
	for _, part := range parts {
		if part.Type != ContentPartText || part.Text == "" {
			continue
		}
		text = joinText(text, part.Text)
	}
	return text
}

func joinText(left, right string) string {
	if left == "" {
		return right
	}
	return left + "\n\n" + strings.TrimLeft(right, "\n")
}

// HasImagePart 返回消息是否包含图片片段。
func HasImagePart(msg Message) bool {
	for _, part := range MessageParts(msg) {
		if part.Type == ContentPartImage {
			return true
		}
	}
	return false
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
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	CacheReadInputTokens  int
	CacheWriteInputTokens int
}

// StreamEventType 表示 provider stream 中的增量事件类型。
type StreamEventType string

const (
	// StreamTextDelta 表示 assistant 文本增量。
	StreamTextDelta StreamEventType = "text_delta"
	// StreamReasoningDelta 表示 assistant 思维链增量。
	StreamReasoningDelta StreamEventType = "reasoning_delta"
)

// StreamEvent 是 provider 在一次模型 step 中产生的实时事件。
type StreamEvent struct {
	Type  StreamEventType
	Delta string
}

// ResponseFormat 描述 provider 返回文本的格式要求。
type ResponseFormat string

const (
	// ResponseFormatJSONObject 表示 provider 应返回一个 JSON object。
	ResponseFormatJSONObject ResponseFormat = "json_object"
)

// ChatRequest 是一次模型 step 的完整通用输入。
type ChatRequest struct {
	System          string
	Messages        []Message
	Tools           []ToolDefinition
	SessionID       string
	MaxTokens       int
	Temperature     float64
	ReasoningEffort string
	ResponseFormat  ResponseFormat
}

// ChatResponse 是一次模型 step 的完整通用输出。
type ChatResponse struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	StopReason       StopReason
	Usage            Usage
	ProviderItems    []ProviderItem
	// RawFinish 保留 provider 的原始结束原因，方便调试。
	RawFinish string
}
