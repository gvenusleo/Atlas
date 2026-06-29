// Package compact provides context compaction planning and summarization helpers.
package compact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	// DefaultTriggerRatio is the default context window usage ratio that triggers auto-compaction.
	DefaultTriggerRatio = 0.8
	// DefaultKeepRecentTokens is the target token count for retaining recent context after compaction.
	DefaultKeepRecentTokens = 20000

	toolResultMaxChars = 2000
	summaryPrefix      = "Context summary from earlier conversation:"
)

// Plan describes the summarizable history prefix and the recent suffix that must be preserved verbatim.
type Plan struct {
	CompactCount int
	KeepCount    int
	TokensBefore int
	TokensAfter  int
}

// EstimateMessages estimates the token count for a group of messages.
func EstimateMessages(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateMessage(msg)
	}
	return total
}

// EstimateMessage estimates the token count for a single message.
func EstimateMessage(msg model.Message) int {
	total := estimateParts(model.MessageParts(msg)) + estimateText(msg.ReasoningContent) + estimateText(msg.ToolCallID) + 4
	for _, call := range msg.ToolCalls {
		total += estimateText(call.ID) + estimateText(call.Name) + estimateText(call.Arguments) + 8
	}
	return total
}

// ShouldAutoCompact determines whether context usage has reached the configured auto-compaction ratio.
func ShouldAutoCompact(inputTokens, contextWindow int, triggerRatio float64) bool {
	if inputTokens <= 0 || contextWindow <= 0 {
		return false
	}
	if triggerRatio <= 0 {
		triggerRatio = DefaultTriggerRatio
	}
	return float64(inputTokens) >= float64(contextWindow)*triggerRatio
}

// BuildActiveMessages constructs the active messages sent to the model based on saved compaction metadata.
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

// SummaryMessage converts a saved summary into a synthetic user message in the model context.
func SummaryMessage(summary string) model.Message {
	return model.TextMessage(model.RoleUser, summaryPrefix+"\n\n"+strings.TrimSpace(summary))
}

// SelectPlan selects a safe auto-compaction split point.
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

// SelectManualPlan selects the most recent safe split point, keeping the latest complete conversation turn.
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

// SerializeMessages converts messages into plain text for use in summary requests.
func SerializeMessages(messages []model.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case model.RoleUser:
			if content := serializeContentParts(model.MessageParts(msg)); strings.TrimSpace(content) != "" {
				parts = append(parts, "[User]: "+content)
			}
		case model.RoleAssistant:
			if strings.TrimSpace(msg.ReasoningContent) != "" {
				parts = append(parts, "[Assistant reasoning]: "+msg.ReasoningContent)
			}
			if content := serializeContentParts(model.MessageParts(msg)); strings.TrimSpace(content) != "" {
				parts = append(parts, "[Assistant]: "+content)
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

// BuildSummaryMessages constructs the request messages used to generate a compaction summary.
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
	if todos := extractLastTodos(messages); len(todos) > 0 {
		b.WriteString("\nCurrent todo list (include incomplete tasks in the Progress section):\n")
		for _, t := range todos {
			fmt.Fprintf(&b, "- [%s] %s\n", t.Status, t.Content)
		}
	}
	b.WriteString("\nConversation to summarize:\n")
	b.WriteString(SerializeMessages(messages))
	return []model.Message{model.TextMessage(model.RoleUser, b.String())}
}

// extractLastTodos scans messages for the last todo_write call and returns its todo list.
// Only returns a list containing incomplete items; returns nil when all are completed.
func extractLastTodos(messages []model.Message) []model.TodoEntry {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != model.RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.Name != "todo_write" {
				continue
			}
			var params struct {
				Todos []struct {
					Content string `json:"content"`
					Status  string `json:"status"`
				} `json:"todos"`
			}
			if err := json.Unmarshal([]byte(call.Arguments), &params); err != nil {
				return nil
			}
			var entries []model.TodoEntry
			hasIncomplete := false
			for _, item := range params.Todos {
				entry := model.TodoEntry{
					Content: strings.TrimSpace(item.Content),
					Status:  model.TodoStatus(item.Status),
				}
				if entry.Content == "" {
					continue
				}
				entries = append(entries, entry)
				if entry.Status != model.TodoStatusCompleted {
					hasIncomplete = true
				}
			}
			if !hasIncomplete {
				return nil
			}
			return entries
		}
	}
	return nil
}

// safeCutBefore determines whether it is safe to split before the specified message, avoiding breaking user turns and tool calls.
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

// estimateText roughly estimates token count from character count.
func estimateText(text string) int {
	if text == "" {
		return 0
	}
	return (len([]rune(text)) + 3) / 4
}

// estimateParts estimates the token count for structured content parts; images are counted by placeholder and fixed overhead only.
func estimateParts(parts []model.ContentPart) int {
	total := 0
	for _, part := range parts {
		switch part.Type {
		case model.ContentPartImage:
			total += 256 + estimateText(imagePlaceholder(part))
		default:
			total += estimateText(part.Text)
		}
	}
	return total
}

// serializeContentParts converts structured content into plain text for use in summaries and memory.
func serializeContentParts(parts []model.ContentPart) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case model.ContentPartImage:
			lines = append(lines, imagePlaceholder(part))
		default:
			if strings.TrimSpace(part.Text) != "" {
				lines = append(lines, part.Text)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func imagePlaceholder(part model.ContentPart) string {
	mimeType := strings.TrimSpace(part.MimeType)
	if mimeType == "" {
		mimeType = "image"
	}
	detail := part.Detail
	if detail == "" {
		detail = model.ImageDetailAuto
	}
	return fmt.Sprintf("[Image: %s, detail=%s]", mimeType, detail)
}

// truncate truncates text by character count, avoiding corruption of UTF-8 content.
func truncate(text string, maxChars int) string {
	runes := []rune(text)
	if maxChars <= 0 || len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n\n[truncated]"
}
