package host

import (
	"bytes"
	"slices"

	"github.com/gvenusleo/atlas/internal/agent"
)

// cloneMessage 复制已支持消息中的可变数据；自定义消息保持原值。
func cloneMessage(message agent.Message) agent.Message {
	switch message := message.(type) {
	case agent.UserMessage:
		message.Content = cloneContent(message.Content)
		return message
	case agent.AssistantMessage:
		message.Content = cloneContent(message.Content)
		return message
	case agent.ToolResultMessage:
		message.Content = cloneContent(message.Content)
		return message
	default:
		return message
	}
}

// cloneMessages 复制消息切片及每条消息中的可变数据。
func cloneMessages(messages []agent.Message) []agent.Message {
	cloned := slices.Clone(messages)
	for i, message := range cloned {
		cloned[i] = cloneMessage(message)
	}
	return cloned
}

// cloneEvent 独立复制各载荷，避免事件字段之间共享可变数据。
func cloneEvent(event agent.Event) agent.Event {
	event.Messages = cloneMessages(event.Messages)
	event.Message = cloneMessage(event.Message)
	event.Delta.ToolCall.Arguments = bytes.Clone(event.Delta.ToolCall.Arguments)
	event.Arguments = bytes.Clone(event.Arguments)
	event.Result.Content = cloneContent(event.Result.Content)
	return event
}

// cloneContent 复制内容切片及工具调用参数；其余内容块仅含字符串。
func cloneContent(content []agent.Content) []agent.Content {
	cloned := slices.Clone(content)
	for i, block := range cloned {
		if call, ok := block.(agent.ToolCall); ok {
			call.Arguments = bytes.Clone(call.Arguments)
			cloned[i] = call
		}
	}
	return cloned
}
