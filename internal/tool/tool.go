// Package tool provides the dispatch layer from model tool calls to local tool implementations.
package tool

import (
	"context"

	"github.com/liuyuxin/atlas/internal/model"
)

// Tool is a local capability that Atlas can invoke via the model.
type Tool interface {
	// Definition returns the tool definition sent to the model.
	Definition() model.ToolDefinition
	// Run executes the tool using the raw JSON parameters from the model.
	Run(ctx context.Context, arguments string) (string, error)
}
