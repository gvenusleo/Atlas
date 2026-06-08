package tool

import (
	"context"
	"fmt"
	"maps"

	"github.com/liuyuxin/atlas/internal/model"
)

// Registry 按工具名分发模型发起的工具调用。
type Registry struct {
	tools map[string]Tool
	order []string
}

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

// Definitions 按注册顺序返回工具定义。
func (r *Registry) Definitions() []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, cloneDefinition(r.tools[name].Definition()))
	}
	return defs
}

// Run 执行一次模型请求的工具调用。
func (r *Registry) Run(ctx context.Context, call model.ToolCall) (string, error) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
	result, err := tool.Run(ctx, call.Arguments)
	return result, err
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
