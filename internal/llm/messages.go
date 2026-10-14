package llm

import (
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

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
