package tool

import (
	"context"
	"encoding/json"
)

// Definition is the model-visible description of one local tool.
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Result is returned to the model after a tool finishes.
type Result struct {
	Content string
	Error   bool
}

// Tool executes one named action for the agent.
type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, raw json.RawMessage) Result
}
