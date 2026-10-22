// Package agent 定义 agent 的领域模型与驱动循环：流式获取 assistant 响应、
// 执行工具调用，直到模型不再请求工具为止。
package agent

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"math"
	"time"

	"github.com/gvenusleo/atlas/internal/skills"
)

// ---- 消息模型 ----

// Role 是消息在对话中的角色。
type Role string

const (
	RoleSystem     Role = "system"
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

// Usage 报告一次 assistant 响应的 token 用量。口径已经过归一化：
// InputTokens 是未命中缓存的净输入（OpenAI 的 input_tokens 含缓存项，需扣除）；
// TotalTokens = InputTokens + CacheReadTokens + CacheWriteTokens + OutputTokens
type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// Validate 检查用量非负、总量不溢出且与各项一致。
func (u Usage) Validate() error {
	var total int64
	parts := [...]int64{u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens}
	for _, value := range parts {
		if value < 0 || value > math.MaxInt64-total {
			return errors.New("Usage 含负值或总量溢出")
		}
		total += value
	}
	if u.TotalTokens != total {
		return errors.New("Usage.TotalTokens 与四项用量之和不一致")
	}
	return nil
}

// StopReason 记录 assistant 响应结束的原因。provider/传输失败不返回 Go error，
// 而是编码为 StopReasonError 或 StopReasonCanceled、细节写入 ErrorMessage，
// 超时（DeadlineExceeded）是故障归 Error，只有主动取消才是 Canceled，
// 上下文溢出不设独立值，同归 Error，识别与恢复由上层负责。
type StopReason string

const (
	// 流式产出中，未定稿消息的占位值。
	StopReasonPending StopReason = "pending"
	StopReasonStop    StopReason = "stop"
	// 输出截断导致工具调用参数可能不完整，执行前须全部作废。
	StopReasonMaxOutputTokens StopReason = "max_output_tokens"
	StopReasonToolUse         StopReason = "tool_use"
	StopReasonError           StopReason = "error"
	StopReasonCanceled        StopReason = "canceled"
)

// Content 是消息内容块的公共契约，种类经 contentType 封闭、可穷举。
type Content interface {
	contentType() string
}

// TextContent 是文本内容块。
type TextContent struct {
	Text string `json:"text"`
}

// contentType 标记文本块，实现 Content 的封闭契约。
func (TextContent) contentType() string {
	return "text"
}

// ThinkingContent 是推理内容块。
type ThinkingContent struct {
	Thinking string `json:"thinking"`
	// Signature 保存 provider 的原始推理项，供适配器回传；调用方不解析。
	Signature string `json:"signature"`
}

// contentType 标记推理块，实现 Content 的封闭契约。
func (ThinkingContent) contentType() string {
	return "thinking"
}

// ImageContent 是图像内容块；Data 为 base64 编码的图像。
type ImageContent struct {
	Data     string `json:"data"`
	MIMEType string `json:"mime_type"`
}

// contentType 标记图片块，实现 Content 的封闭契约。
func (ImageContent) contentType() string {
	return "image"
}

// ToolCall 是 assistant 消息里的工具调用块。
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Arguments 保持原始 JSON；消息编码时作为字符串保存，执行时才解析。
	Arguments jsontext.Value `json:"arguments"`
}

// contentType 标记工具调用块，实现 Content 的封闭契约。
func (ToolCall) contentType() string {
	return "tool_call"
}

// Message 是会话中的一条消息；role 由具体类型派生，实现保持开放以便扩展自定义消息。
type Message interface {
	Role() Role
}

// SystemMessageType 区分系统提示词快照、对话中的系统通知与上下文压缩记录。
type SystemMessageType string

const (
	SystemMessageTypePrompt     SystemMessageType = "system_prompt"
	SystemMessageTypeNotice     SystemMessageType = "notice"
	SystemMessageTypeCompaction SystemMessageType = "compaction"
)

// SystemMessage 是提示词快照、系统通知或压缩记录；Type 必须显式指定。
// 压缩记录的 Content 是较早对话的摘要，Kept 是它之前的消息里原样保留的最近条数：
// 发给模型的上下文是这条摘要加上保留的那些消息，更早的消息只留在历史里。
type SystemMessage struct {
	Type      SystemMessageType `json:"type"`
	Content   string            `json:"content"`
	Kept      int               `json:"kept,omitzero"`
	Timestamp time.Time         `json:"timestamp"`
}

// Role 返回系统消息角色。
func (SystemMessage) Role() Role { return RoleSystem }

// UserMessage 是用户消息。
type UserMessage struct {
	Content   []Content `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Role 返回用户消息角色。
func (UserMessage) Role() Role { return RoleUser }

// ErrorKind 是流函数对失败原因的分类，只在流函数与循环之间传递，不保存。
type ErrorKind string

const (
	// ErrorKindRetryable 表示失败多半是暂时的（限流、服务端故障、网络中断），稍后重试可能成功。
	ErrorKindRetryable ErrorKind = "retryable"
	// ErrorKindContextOverflow 表示请求超出了模型的上下文窗口：重试没用，要先压缩。
	ErrorKindContextOverflow ErrorKind = "context_overflow"
)

// AssistantMessage 是一次模型响应的完整消息。
type AssistantMessage struct {
	Content []Content `json:"content"`
	// Model 是产生这条消息的模型的完整引用 provider/model-id，由循环按 LoopContext.Model 记下。
	Model        string     `json:"model"`
	Usage        Usage      `json:"usage,omitzero"`
	StopReason   StopReason `json:"stop_reason"`
	ErrorMessage string     `json:"error_message,omitempty"`
	// ErrorKind 只在 StopReason 为 error 时有意义，由流函数设置；空表示不应重试。
	ErrorKind ErrorKind `json:"-"`
	Timestamp time.Time `json:"timestamp"`
}

// Role 返回助手消息角色。
func (AssistantMessage) Role() Role { return RoleAssistant }

// ToolResultMessage 是工具执行结果消息。
type ToolResultMessage struct {
	ToolCallID string    `json:"tool_call_id"`
	ToolName   string    `json:"tool_name"`
	Content    []Content `json:"content"`
	IsError    bool      `json:"is_error,omitzero"`
	// Details 是工具随结果附带的结构化数据（合法的 JSON），供界面渲染，不发给模型。
	Details   jsontext.Value `json:"details,omitzero"`
	Timestamp time.Time      `json:"timestamp"`
}

// Role 返回工具结果角色。
func (ToolResultMessage) Role() Role { return RoleToolResult }

// 编译期断言：各内容块/消息以值类型实现对应接口，使用现场可按值传入。
var (
	_ Content = TextContent{}
	_ Content = ThinkingContent{}
	_ Content = ImageContent{}
	_ Content = ToolCall{}

	_ Message = SystemMessage{}
	_ Message = UserMessage{}
	_ Message = AssistantMessage{}
	_ Message = ToolResultMessage{}
)

// ---- 工具 ----

// ToolResult 是工具产出的最终结果。
type ToolResult struct {
	Content []Content
	// IsError 表示工具执行失败，Content 保留可交给模型处理的错误细节。
	IsError bool
	// Details 是结构化数据（合法的 JSON），随结果保存并供界面渲染，不发给模型；没有时为空。
	Details jsontext.Value
}

// Tool 是工具契约：元信息描述给模型，Execute 执行调用；
// Execute 返回的 error 会被 loop 转成 IsError 为 true 的结果，不中断循环。
type Tool struct {
	Name        string
	Description string
	// Parameters 是 JSON Schema 对象；省略时按无参工具处理。
	Parameters map[string]any
	// ReadOnly 表示工具不改变任何状态，同一轮里连续的只读调用可以并发执行。
	ReadOnly bool
	Execute  func(ctx context.Context, toolCallID string, params jsontext.Value) (ToolResult, error)
}

// ---- 流式调用 ----

// ThinkingLevel 是规范思考档位词表，用于解析用户输入与构建请求；
// 具体模型支持哪些档位由配置的 reasoning_options.values（其子集）声明。
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
	ThinkingMax     ThinkingLevel = "max"
)

// Valid 判断档位是否属于规范词表。
func (l ThinkingLevel) Valid() bool {
	switch l {
	case ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium,
		ThinkingHigh, ThinkingXHigh, ThinkingMax:
		return true
	}
	return false
}

// StreamRequest 是一次模型调用的输入。
type StreamRequest struct {
	Model        string // 完整引用 provider/model-id，即 LoopContext.Model。
	SystemPrompt string // 本次初始化的有效提示词，不属于对话历史。
	Messages     []Message
	Tools        []Tool
	// ThinkingLevel 为空时采用服务端默认深度；off 表示请求关闭思考。
	ThinkingLevel ThinkingLevel
}

// DeltaType 是随 message_update 事件发出的流式增量种类。
type DeltaType string

const (
	DeltaTypeText     DeltaType = "text_delta"
	DeltaTypeThinking DeltaType = "thinking_delta"
	DeltaTypeToolCall DeltaType = "tool_call_delta"
)

// Delta 是 assistant 响应流式产出的增量。
type Delta struct {
	Type DeltaType
	Text string
	// ToolCall 是累积快照，Arguments 为已累积到目前的原始 JSON。
	ToolCall ToolCall
}

// StreamFn 调用模型并流式产出增量，返回完整的 assistant 消息。契约：
// 失败不返回 Go error，而是编码为 StopReasonError 或 StopReasonCanceled、
// 细节写入 ErrorMessage，暂时性的失败再用 ErrorKind 标出；
// 无论成败都必须返回完整消息；emit 可调 0 到 n 次；
// 必须尊重 ctx 取消，取消时返回 StopReasonCanceled；emit 必须同步调用；
// 消息与内容块必须以值形式装入（*ToolCall 等指针形式不被识别）。
type StreamFn func(ctx context.Context, req StreamRequest, emit func(Delta)) AssistantMessage

// ---- 事件 ----

// EventType 区分事件的生命周期阶段：run、turn、消息、工具执行。
type EventType string

const (
	EventTypeAgentStart         EventType = "agent_start"
	EventTypeAgentEnd           EventType = "agent_end"
	EventTypeTurnStart          EventType = "turn_start"
	EventTypeTurnEnd            EventType = "turn_end"
	EventTypeMessageStart       EventType = "message_start"
	EventTypeMessageUpdate      EventType = "message_update"
	EventTypeMessageEnd         EventType = "message_end"
	EventTypeRetry              EventType = "retry"
	EventTypeToolExecutionStart EventType = "tool_execution_start"
	EventTypeToolExecutionEnd   EventType = "tool_execution_end"
)

// Event 是 agent 生命周期事件；Type 决定哪些载荷字段有效：
// agent_end 使用 Messages，turn_end 使用 Message 与 Messages；
// message_start/end 使用 Message，message_update 使用 Delta，retry 使用 Retry；
// retry 发生在 message_start 与 message_end 之间：这次响应失败了，已流出的内容作废，
// 等待之后重新请求，最终只有一条消息结束；
// tool_execution_start 使用调用标识与 Arguments，tool_execution_end 使用调用标识与 Result。
type Event struct {
	Type       EventType
	Messages   []Message
	Message    Message
	Delta      Delta
	Retry      RetryInfo
	ToolCallID string
	ToolName   string
	Arguments  jsontext.Value
	Result     ToolResult
	Timestamp  time.Time
}

// ---- 循环 ----

// LoopContext 是当前会话的完整状态：历史、工具、模型和运行策略都由调用方持有。
type LoopContext struct {
	SystemPrompt string // 初始化后复用；Messages 不含提示词快照。
	Messages     []Message
	Tools        []Tool
	Skills       []skills.Skill
	// Model 是会话当前模型的完整引用 provider/model-id，能唯一指出一个模型；
	// 会话的模型只记在这里，StreamFn 据此选用对应的客户端与模型 ID。
	Model string
	// ThinkingLevel 为空时采用服务端默认深度。
	ThinkingLevel ThinkingLevel
	// MaxTurns 限制一次 run 的 turn 数，0 表示不限；防御模型无限请求工具。
	MaxTurns int
	// Retry 是模型调用遇到暂时性失败时的重试策略，零值表示不重试。
	Retry RetryPolicy
	// ContextWindow 是模型的上下文窗口（token）；0 表示未知：不按阈值压缩，只在请求溢出时压缩。
	ContextWindow int64
	Stream        StreamFn
}
