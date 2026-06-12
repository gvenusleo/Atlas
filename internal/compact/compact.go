// Package compact 提供上下文压缩的规划和摘要辅助能力。
package compact

import (
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	// DefaultTriggerRatio 是触发自动压缩的默认上下文窗口使用比例。
	DefaultTriggerRatio = 0.8
	// DefaultKeepRecentTokens 是压缩后保留最近上下文的目标 token 数。
	DefaultKeepRecentTokens = 20000

	toolResultMaxChars = 2000
	summaryPrefix      = "Context summary from earlier conversation:"
)

// Plan 描述可摘要的历史前缀和必须原样保留的最近后缀。
type Plan struct {
	CompactCount int
	KeepCount    int
	TokensBefore int
	TokensAfter  int
}

// EstimateMessages 估算一组消息的 token 数。
func EstimateMessages(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateMessage(msg)
	}
	return total
}

// EstimateMessage 估算单条消息的 token 数。
func EstimateMessage(msg model.Message) int {
	total := estimateText(msg.Content) + estimateText(msg.ReasoningContent) + estimateText(msg.ToolCallID) + 4
	for _, call := range msg.ToolCalls {
		total += estimateText(call.ID) + estimateText(call.Name) + estimateText(call.Arguments) + 8
	}
	return total
}

// ShouldAutoCompact 判断上下文使用量是否达到配置的自动压缩比例。
func ShouldAutoCompact(inputTokens, contextWindow int, triggerRatio float64) bool {
	if inputTokens <= 0 || contextWindow <= 0 {
		return false
	}
	if triggerRatio <= 0 {
		triggerRatio = DefaultTriggerRatio
	}
	return float64(inputTokens) >= float64(contextWindow)*triggerRatio
}

// BuildActiveMessages 根据已保存的压缩元数据构造发送给模型的活动消息。
func BuildActiveMessages(summary string, compactedCount int, full []model.Message) []model.Message {
	if compactedCount < 0 {
		compactedCount = 0
	}
	if compactedCount > len(full) {
		compactedCount = len(full)
	}
	active := make([]model.Message, 0, len(full)-compactedCount+1)
	if strings.TrimSpace(summary) != "" {
		active = append(active, SummaryMessage(summary))
	}
	active = append(active, full[compactedCount:]...)
	return active
}

// SummaryMessage 将已保存摘要转换成模型上下文中的合成用户消息。
func SummaryMessage(summary string) model.Message {
	return model.Message{
		Role:    model.RoleUser,
		Content: summaryPrefix + "\n\n" + strings.TrimSpace(summary),
	}
}

// SelectPlan 选择一个安全的自动压缩切分点。
func SelectPlan(messages []model.Message, alreadyCompacted, keepRecentTokens int) (Plan, bool) {
	if alreadyCompacted < 0 {
		alreadyCompacted = 0
	}
	if alreadyCompacted > len(messages) {
		alreadyCompacted = len(messages)
	}
	if keepRecentTokens <= 0 {
		keepRecentTokens = DefaultKeepRecentTokens
	}
	if len(messages)-alreadyCompacted < 2 {
		return Plan{}, false
	}

	recentTokens := 0
	best := 0
	for i := len(messages) - 1; i > alreadyCompacted; i-- {
		recentTokens += EstimateMessage(messages[i])
		if safeCutBefore(messages, i) {
			best = i
			if recentTokens >= keepRecentTokens {
				break
			}
		}
	}
	if best <= alreadyCompacted {
		return Plan{}, false
	}
	compactMessages := messages[alreadyCompacted:best]
	keptMessages := messages[best:]
	return Plan{
		CompactCount: best,
		KeepCount:    len(keptMessages),
		TokensBefore: EstimateMessages(messages),
		TokensAfter:  EstimateMessages(keptMessages),
	}, len(compactMessages) > 0
}

// SelectManualPlan 选择最近的安全切分点，并保留最新一轮完整对话。
func SelectManualPlan(messages []model.Message, alreadyCompacted int) (Plan, bool) {
	if alreadyCompacted < 0 {
		alreadyCompacted = 0
	}
	if alreadyCompacted > len(messages) {
		alreadyCompacted = len(messages)
	}
	for i := len(messages) - 1; i > alreadyCompacted; i-- {
		if !safeCutBefore(messages, i) {
			continue
		}
		keptMessages := messages[i:]
		return Plan{
			CompactCount: i,
			KeepCount:    len(keptMessages),
			TokensBefore: EstimateMessages(messages),
			TokensAfter:  EstimateMessages(keptMessages),
		}, i > alreadyCompacted
	}
	return Plan{}, false
}

// SerializeMessages 将消息转换成摘要请求使用的纯文本。
func SerializeMessages(messages []model.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case model.RoleUser:
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, "[User]: "+msg.Content)
			}
		case model.RoleAssistant:
			if strings.TrimSpace(msg.ReasoningContent) != "" {
				parts = append(parts, "[Assistant reasoning]: "+msg.ReasoningContent)
			}
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, "[Assistant]: "+msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				var calls []string
				for _, call := range msg.ToolCalls {
					calls = append(calls, fmt.Sprintf("%s(%s)", call.Name, call.Arguments))
				}
				parts = append(parts, "[Assistant tool calls]: "+strings.Join(calls, "; "))
			}
		case model.RoleTool:
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, "[Tool result]: "+truncate(msg.Content, toolResultMaxChars))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// BuildSummaryMessages 构造生成压缩摘要时使用的请求消息。
func BuildSummaryMessages(previousSummary string, messages []model.Message, instruction string) []model.Message {
	var b strings.Builder
	b.WriteString("Summarize the conversation history below for future continuation.\n\n")
	b.WriteString("Do not continue the conversation. Preserve concrete facts, constraints, decisions, file paths, commands, errors, and next steps.\n")
	b.WriteString("Use this exact Markdown structure:\n")
	b.WriteString("## Goal\n## Constraints And Preferences\n## Progress\n## Key Decisions\n## Relevant Files\n## Next Steps\n## Critical Context\n")
	if strings.TrimSpace(instruction) != "" {
		b.WriteString("\nAdditional user instruction:\n")
		b.WriteString(strings.TrimSpace(instruction))
		b.WriteString("\n")
	}
	if strings.TrimSpace(previousSummary) != "" {
		b.WriteString("\nPrevious summary to update:\n")
		b.WriteString(strings.TrimSpace(previousSummary))
		b.WriteString("\n")
	}
	b.WriteString("\nConversation to summarize:\n")
	b.WriteString(SerializeMessages(messages))
	return []model.Message{{
		Role:    model.RoleUser,
		Content: b.String(),
	}}
}

// safeCutBefore 判断是否可以在指定消息前切分，避免拆开用户轮次和工具调用。
func safeCutBefore(messages []model.Message, index int) bool {
	if index <= 0 || index >= len(messages) {
		return false
	}
	if messages[index].Role != model.RoleUser {
		return false
	}
	prev := messages[index-1]
	if prev.Role == model.RoleAssistant && len(prev.ToolCalls) > 0 {
		return false
	}
	return true
}

// estimateText 用字符数粗略估算 token 数。
func estimateText(text string) int {
	if text == "" {
		return 0
	}
	return (len([]rune(text)) + 3) / 4
}

// truncate 按字符数截断文本，避免切坏 UTF-8 内容。
func truncate(text string, maxChars int) string {
	runes := []rune(text)
	if maxChars <= 0 || len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n\n[truncated]"
}
