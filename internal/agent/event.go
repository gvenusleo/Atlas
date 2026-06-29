package agent

import "github.com/liuyuxin/atlas/internal/model"

// EventType represents the type of an observable event in the agent turn loop.
type EventType string

const (
	// EventTurnStarted indicates that a user turn has entered the agent loop.
	EventTurnStarted EventType = "turn_started"
	// EventModelDelta indicates the model returned a text delta.
	EventModelDelta EventType = "model_delta"
	// EventModelReasoningDelta indicates the model returned a chain-of-thought delta.
	EventModelReasoningDelta EventType = "model_reasoning_delta"
	// EventModelResponse indicates a model step has returned.
	EventModelResponse EventType = "model_response"
	// EventToolStarted indicates a tool call is about to execute.
	EventToolStarted EventType = "tool_started"
	// EventToolFinished indicates a tool call has finished.
	EventToolFinished EventType = "tool_finished"
	// EventTurnFinished indicates a user turn has ended.
	EventTurnFinished EventType = "turn_finished"
)

// Event describes an observable event in the agent loop.
type Event struct {
	Type         EventType
	Step         int
	Content      string
	ToolCall     model.ToolCall
	ToolResult   string
	ToolMetadata model.ToolMetadata
	ToolError    bool
	Err          error
}

// Observer receives agent loop events.
type Observer func(Event)
