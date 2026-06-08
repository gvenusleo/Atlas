package agent

import "github.com/liuyuxin/atlas/internal/model"

// EventType 表示 agent turn loop 中可观察的事件类型。
type EventType string

const (
	// EventTurnStarted 表示一个用户 turn 已进入 agent loop。
	EventTurnStarted EventType = "turn_started"
	// EventModelResponse 表示一次模型 step 已返回。
	EventModelResponse EventType = "model_response"
	// EventToolStarted 表示一个工具调用即将执行。
	EventToolStarted EventType = "tool_started"
	// EventToolFinished 表示一个工具调用已经结束。
	EventToolFinished EventType = "tool_finished"
	// EventTurnFinished 表示一个用户 turn 已结束。
	EventTurnFinished EventType = "turn_finished"
)

// Event 描述 agent loop 中发生的一件可观察事件。
type Event struct {
	Type       EventType
	Step       int
	Content    string
	ToolCall   model.ToolCall
	ToolResult string
	ToolError  bool
	Err        error
}

// Observer 接收 agent loop 事件。
type Observer func(Event)
