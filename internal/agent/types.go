package agent

import (
	"time"

	"github.com/liuyuxin/atlas/internal/model"
)

// EventType identifies a user-visible agent event.
type EventType string

const (
	EventSessionStarted EventType = "session_started"
	EventTurnStarted    EventType = "turn_started"
	EventTextDelta      EventType = "text_delta"
	EventToolStarted    EventType = "tool_started"
	EventToolFinished   EventType = "tool_finished"
	EventTurnFinished   EventType = "turn_finished"
	EventError          EventType = "error"
)

// Event is the stable boundary consumed by CLI, TUI, and future SDKs.
type Event struct {
	Type       EventType
	SessionID  string
	Text       string
	ToolName   string
	ToolCallID string
	Error      bool
	CreatedAt  time.Time
}

// Config controls one Atlas agent instance.
type Config struct {
	Workdir     string
	Model       string
	MaxSteps    int
	Temperature float64
	SkillRoots  []string
}

// TurnResult captures the final assistant response and tool calls.
type TurnResult struct {
	Message model.AssistantResult
	Steps   int
}
