package model

import "context"

// Provider 将具体模型后端适配到 Atlas 的通用聊天协议。
type Provider interface {
	// Chat 执行一次非流式模型调用。
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
