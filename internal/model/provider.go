package model

import "context"

// Provider 将具体模型后端适配到 Atlas 的通用聊天协议。
type Provider interface {
	// Stream 执行一次流式模型调用，并返回累计后的完整响应。
	Stream(ctx context.Context, req ChatRequest, emit func(StreamEvent) error) (ChatResponse, error)
}
