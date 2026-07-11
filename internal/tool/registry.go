package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/liuyuxin/atlas/internal/model"
)

// Registry dispatches model-initiated tool calls by tool name.
type Registry struct {
	tools  map[string]Tool
	order  []string
	runner RunFunc
}

// RunResult describes the text result and structured presentation data for a tool call.
type RunResult struct {
	Content  string
	Metadata model.ToolMetadata
}

// RunFunc executes a single tool call.
type RunFunc func(context.Context, model.ToolCall) (RunResult, error)

// NewRegistry creates a tool registry.
// Tool names must be unique; otherwise dispatch becomes non-deterministic.
func NewRegistry(tools ...Tool) (*Registry, error) {
	r := &Registry{
		tools: make(map[string]Tool),
	}
	for _, tool := range tools {
		name := tool.Definition().Name
		if _, ok := r.tools[name]; ok {
			return nil, fmt.Errorf("duplicate tool %q", name)
		}
		r.tools[name] = tool
		r.order = append(r.order, name)
	}
	return r, nil
}

// WithRunner returns a new registry using a custom executor, with tool definitions unchanged.
func (r *Registry) WithRunner(runner RunFunc) *Registry {
	if runner == nil {
		return r
	}
	return &Registry{
		tools:  r.tools,
		order:  r.order,
		runner: runner,
	}
}

// Definitions returns tool definitions in registration order.
func (r *Registry) Definitions() []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, cloneDefinition(r.tools[name].Definition()))
	}
	return defs
}

// Run executes a single model-requested tool call.
func (r *Registry) Run(ctx context.Context, call model.ToolCall) (RunResult, error) {
	if r.runner != nil {
		return r.runner(ctx, call)
	}
	return r.RunDefault(ctx, call)
}

// MetadataTool is an optional tool interface that allows tools to return structured metadata after execution.
type MetadataTool interface {
	Metadata(arguments string, content string) model.ToolMetadata
}

// RunResultTool is an optional tool interface for returning structured data, including on errors.
type RunResultTool interface {
	RunResult(context.Context, string) (RunResult, error)
}

// RunDefault executes a single call using the tool implementation from the registry.
func (r *Registry) RunDefault(ctx context.Context, call model.ToolCall) (RunResult, error) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return RunResult{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	if resultTool, ok := tool.(RunResultTool); ok {
		return resultTool.RunResult(ctx, call.Arguments)
	}
	result, err := tool.Run(ctx, call.Arguments)
	runResult := RunResult{Content: result}
	if err == nil {
		if mt, ok := tool.(MetadataTool); ok {
			runResult.Metadata = mt.Metadata(call.Arguments, result)
		}
	}
	return runResult, err
}

func cloneDefinition(def model.ToolDefinition) model.ToolDefinition {
	if def.Parameters == nil {
		return def
	}
	params := make(map[string]any, len(def.Parameters))
	maps.Copy(params, def.Parameters)
	def.Parameters = params
	return def
}
