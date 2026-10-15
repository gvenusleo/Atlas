package llm

import (
	"encoding/json/jsontext"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/gvenusleo/atlas/internal/agent"
)

// messageOf 按终态输出顺序构造完整消息，并映射结束原因与 token 用量；
// 推理项同时保留展示文本与原始 JSON，仅含加密数据的项也用于后续回传。
func messageOf(r responses.Response, model string) agent.AssistantMessage {
	msg := agent.AssistantMessage{Model: model, Usage: usageOf(r.Usage)}
	for _, item := range r.Output {
		switch v := item.AsAny().(type) {
		case responses.ResponseReasoningItem:
			text := reasoningText(v)
			if text != "" || v.RawJSON() != "" {
				msg.Content = append(msg.Content,
					agent.ThinkingContent{Thinking: text, Signature: v.RawJSON()})
			}
		case responses.ResponseOutputMessage:
			for _, p := range v.Content {
				switch t := p.AsAny().(type) {
				case responses.ResponseOutputText:
					msg.Content = append(msg.Content, agent.TextContent{Text: t.Text})
				case responses.ResponseOutputRefusal:
					// 拒答回传，避免静默丢内容。
					msg.Content = append(msg.Content, agent.TextContent{Text: t.Refusal})
				}
			}
		case responses.ResponseFunctionToolCall:
			msg.Content = append(msg.Content, agent.ToolCall{
				ID:        v.CallID,
				Name:      v.Name,
				Arguments: jsontext.Value(v.Arguments),
			})
		}
	}
	msg.StopReason, msg.ErrorMessage = stopOf(r, msg.Content)
	return msg
}

// stopOf 映射响应状态；完成且含工具调用时为 tool_use。
func stopOf(r responses.Response, content []agent.Content) (agent.StopReason, string) {
	switch r.Status {
	case responses.ResponseStatusCompleted:
		for _, c := range content {
			if _, ok := c.(agent.ToolCall); ok {
				return agent.StopReasonToolUse, ""
			}
		}
		return agent.StopReasonStop, ""
	case responses.ResponseStatusIncomplete:
		if r.IncompleteDetails.Reason == "max_output_tokens" {
			return agent.StopReasonMaxOutputTokens, ""
		}
		return agent.StopReasonError, "响应不完整：" + r.IncompleteDetails.Reason
	case responses.ResponseStatusFailed:
		return agent.StopReasonError, r.Error.Message
	case responses.ResponseStatusCancelled:
		return agent.StopReasonCanceled, ""
	default:
		return agent.StopReasonError, "非终态或未知的响应状态：" + string(r.Status)
	}
}

// reasoningText 取终态推理文本：content 优先，为空时取 summary。
func reasoningText(item responses.ResponseReasoningItem) string {
	content := make([]string, 0, len(item.Content))
	for _, c := range item.Content {
		if c.Text != "" {
			content = append(content, c.Text)
		}
	}
	if text := strings.Join(content, "\n\n"); text != "" {
		return text
	}
	summary := make([]string, 0, len(item.Summary))
	for _, stm := range item.Summary {
		if stm.Text != "" {
			summary = append(summary, stm.Text)
		}
	}
	return strings.Join(summary, "\n\n")
}

// usageOf 归一化 token 口径：InputTokens 是未命中缓存的净输入（假定 provider 的
// input_tokens 含缓存读写项）；TotalTokens 由四项相加得出。
func usageOf(u responses.ResponseUsage) agent.Usage {
	read := u.InputTokensDetails.CachedTokens
	write := u.InputTokensDetails.CacheWriteTokens
	input := max(0, u.InputTokens-read-write)
	total := input + read + write + u.OutputTokens
	return agent.Usage{
		InputTokens:      input,
		CacheReadTokens:  read,
		CacheWriteTokens: write,
		OutputTokens:     u.OutputTokens,
		TotalTokens:      total,
	}
}
