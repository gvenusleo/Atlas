// Package tool 提供模型工具调用到本地工具实现的分发层。
package tool

import (
	"context"

	"github.com/liuyuxin/atlas/internal/model"
)

// Tool 是 Atlas 可以由模型调用的本地能力。
type Tool interface {
	// Definition 返回发送给模型的工具定义。
	Definition() model.ToolDefinition
	// Run 使用模型给出的原始 JSON 参数执行工具。
	Run(ctx context.Context, arguments string) (string, error)
}
