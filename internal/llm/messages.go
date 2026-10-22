package llm

import (
	"context"
	"slices"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

// withoutImages 包装流函数：请求里的图片换成一句说明，历史里的图片不动。用于不接受图片
// 输入的模型；否则图片一旦进了历史，换到这样的模型后，每次请求都会被拒绝。
func withoutImages(stream agent.StreamFn) agent.StreamFn {
	return func(
		ctx context.Context, req agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		req.Messages = slices.Clone(req.Messages)
		for i, message := range req.Messages {
			switch m := message.(type) {
			case agent.UserMessage:
				m.Content = omitImages(m.Content)
				req.Messages[i] = m
			case agent.ToolResultMessage:
				m.Content = omitImages(m.Content)
				req.Messages[i] = m
			}
		}
		return stream(ctx, req, emit)
	}
}

// omitImages 返回把图片换成说明文字的内容副本。
func omitImages(content []agent.Content) []agent.Content {
	content = slices.Clone(content)
	for i, c := range content {
		if _, ok := c.(agent.ImageContent); ok {
			content[i] = agent.TextContent{Text: "[图片：当前模型不支持图片输入]"}
		}
	}
	return content
}

// messagesForRequest 创建模型请求使用的历史投影，不修改原始消息或写入存储。
// 失败和取消的助手消息不重放；缺失工具结果仅在此投影中补齐。
func messagesForRequest(messages []agent.Message) []agent.Message {
	result := make([]agent.Message, 0, len(messages))
	var pending []agent.ToolCall
	answered := make(map[string]bool)
	var held []agent.Message
	closeCalls := func() {
		for _, call := range pending {
			if answered[call.ID] {
				continue
			}
			result = append(result, agent.ToolResultMessage{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				IsError:    true,
				Timestamp:  time.Now(),
				Content: []agent.Content{agent.TextContent{
					Text: "未提供工具结果，工具是否执行及其执行结果未知。未重新执行该调用。",
				}},
			})
		}
		pending = nil
		clear(answered)
		result = append(result, held...)
		held = nil
	}

	for _, message := range messages {
		switch m := message.(type) {
		case agent.AssistantMessage:
			closeCalls()
			if m.StopReason == agent.StopReasonError || m.StopReason == agent.StopReasonCanceled {
				continue
			}
			for _, content := range m.Content {
				if call, ok := content.(agent.ToolCall); ok {
					pending = append(pending, call)
				}
			}
		case agent.ToolResultMessage:
			answered[m.ToolCallID] = true
		case agent.SystemMessage:
			// 通知不结束工具结果匹配，延后到真实结果和合成结果之后。
			if len(pending) != 0 {
				held = append(held, m)
				continue
			}
		case agent.UserMessage:
			closeCalls()
		}
		result = append(result, message)
	}
	closeCalls()
	return result
}
