package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/liuyuxin/atlas/internal/model"
)

// Registry 按工具名分发模型发起的工具调用。
type Registry struct {
	tools  map[string]Tool
	order  []string
	runner RunFunc
}

// RunResult 描述一次工具调用的文本结果和结构化展示数据。
type RunResult struct {
	Content  string
	Metadata model.ToolMetadata
}

// RunFunc 执行一次工具调用。
type RunFunc func(context.Context, model.ToolCall) (RunResult, error)

// NewRegistry 创建一个工具注册表。
// 工具名必须唯一，否则后续分发会变得不确定。
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

// WithRunner 返回使用自定义执行器的新注册表，工具定义保持不变。
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

// Definitions 按注册顺序返回工具定义。
func (r *Registry) Definitions() []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, cloneDefinition(r.tools[name].Definition()))
	}
	return defs
}

// Run 执行一次模型请求的工具调用。
func (r *Registry) Run(ctx context.Context, call model.ToolCall) (RunResult, error) {
	if r.runner != nil {
		return r.runner(ctx, call)
	}
	return r.RunDefault(ctx, call)
}

// RunDefault 使用注册表中的工具实现执行一次调用。
func (r *Registry) RunDefault(ctx context.Context, call model.ToolCall) (RunResult, error) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return RunResult{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	result, err := tool.Run(ctx, call.Arguments)
	return RunResult{Content: result}, err
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
