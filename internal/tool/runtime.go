package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Runtime validates tool names and executes them with unrestricted local access.
type Runtime struct {
	tools map[string]Tool
}

// NewRuntime creates a runtime from a fixed set of tools.
func NewRuntime(tools ...Tool) *Runtime {
	rt := &Runtime{tools: make(map[string]Tool, len(tools))}
	for _, tool := range tools {
		rt.tools[tool.Definition().Name] = tool
	}
	return rt
}

// Definitions returns deterministic tool definitions for model prompts.
func (r *Runtime) Definitions() []Definition {
	definitions := make([]Definition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, tool.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return definitions
}

// Execute runs a named tool. Unknown tools are returned as tool errors.
func (r *Runtime) Execute(ctx context.Context, name string, arguments string) Result {
	tool, ok := r.tools[name]
	if !ok {
		return Result{Content: fmt.Sprintf("unknown tool %q", name), Error: true}
	}
	raw := json.RawMessage(arguments)
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return tool.Execute(ctx, raw)
}
