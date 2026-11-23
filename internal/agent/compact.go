package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// 压缩的尺寸，单位 token；窗口很小时按窗口的四分之一收缩，免得没有可压缩的内容。
const (
	reserveTokens    = 16384 // 给回答留的空间：上下文超过 窗口 − reserve 就先压缩
	keepRecentTokens = 20000 // 压缩时原样保留的最近消息的大小
	clipChars        = 2000  // 摘要源里每个工具结果、每组工具参数最多保留的字符数
	imageTokens      = 1200  // 估算时一张图片的大小
)

const summarySystemPrompt = `你是上下文摘要助手。读取用户与 AI 助手之间的对话，
按指定格式输出结构化摘要。不要继续对话，不要回答对话中的问题，只输出摘要。`

const summaryInstructions = `以上是需要摘要的对话。写一份结构化的上下文检查点摘要，
供另一个模型据此继续工作。如果对话开头有此前的摘要，保留其中的信息，
并把新的内容并入。用与对话相同的语言书写。

严格使用以下格式：

## 目标
[用户要完成什么？会话涉及多项任务时可以有多条。]

## 约束与偏好
- [用户提到的约束、偏好与要求]
- [没有则写“（无）”]

## 进展
### 已完成
- [x] [已完成的任务与改动]

### 进行中
- [ ] [当前在做的事]

### 受阻
- [阻碍进展的问题，没有则省略]

## 关键决定
- **[决定]**：[简短的理由]

## 下一步
1. [接下来依次要做什么]

## 关键上下文
- [继续工作所需的数据、示例或引用]
- [没有则写“（无）”]

每一节保持简洁，保留准确的文件路径、函数名与错误信息。`

// limits 返回按窗口收缩后的预留量与保留量。
func limits(window int64) (reserve, keep int64) {
	reserve, keep = reserveTokens, keepRecentTokens
	if window > 0 {
		reserve, keep = min(reserve, window/4), min(keep, window/4)
	}
	return reserve, keep
}

// compactor 限制预压缩。提供方拒绝请求后的恢复由请求循环单独限制。
type compactor struct {
	r      *run
	failed bool // 预压缩失败后，本轮不再主动压缩
}

// prepareRequest 按新增用户消息后的上下文决定是否预压缩。
func (c *compactor) prepareRequest(ctx context.Context, keepMessages int) {
	window := c.r.Model.ContextWindow
	if c.failed || window <= 0 {
		return
	}
	reserve, keep := limits(window)
	if Tokens(c.r.Messages) > window-reserve {
		cut := min(cutPoint(c.r.Messages, keep), len(c.r.Messages)-keepMessages)
		c.failed = compact(ctx, c.r, cut)
	}
}

// Compact 手动压缩较早的上下文，优先原样保留最近的完整轮次。in 不被修改。
func Compact(ctx context.Context, in Input, emit func(Event)) []Message {
	r := newRun(in, emit)
	compact(ctx, r, manualCutPoint(in.Messages, in.Model.ContextWindow))
	return r.added
}

// Compactable 报告按当前模型的窗口保留近期消息后，是否还有可压缩的内容。
func Compactable(messages []Message, window int64) bool {
	return canCut(messages, manualCutPoint(messages, window))
}

// manualCutPoint 在预算内优先保留完整轮次。短会话也可以单独压缩已经谈完的轮次。
func manualCutPoint(messages []Message, window int64) int {
	_, keep := limits(window)
	var kept int64
	lastBoundary, fittingBoundary := -1, -1
	lastUser, fittingUser := -1, -1
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		kept += estimateMessage(message)
		if isBoundary(message) {
			if lastBoundary < 0 {
				lastBoundary = i
			}
			if kept <= keep {
				fittingBoundary = i
			}
		}
		if user, ok := message.(UserMessage); ok && !user.InTurn {
			if lastUser < 0 {
				lastUser = i
			}
			if kept > keep {
				break
			}
			fittingUser = i
		}
	}
	switch {
	case canCut(messages, fittingUser):
		return fittingUser
	case fittingUser >= 0:
		return lastUser
	case fittingBoundary >= 0:
		return fittingBoundary
	default:
		return lastBoundary
	}
}

func canCut(messages []Message, cut int) bool {
	return cut > 0 && (cut != 1 || !isCompaction(messages[0]))
}

// Fold 把一条新记录并进上下文并返回新的上下文：压缩记录换成“摘要 + 它之前保留的 Kept 条”，
// 系统提示词快照（系统提示词单独发送）与通知（只给人看）不进上下文，其余追加。
// 上下文由历史依次 Fold 得到；运行中推进与重放历史用的是这同一条规则。
// 追加时可能复用 messages 的底层数组。
func Fold(messages []Message, m Message) []Message {
	if s, ok := m.(SystemMessage); ok {
		switch s.Type {
		case SystemMessageTypeSystemPrompt, SystemMessageTypeNotice:
			return messages
		case SystemMessageTypeCompaction:
			kept := messages[len(messages)-min(s.Kept, len(messages)):]
			return append([]Message{s}, kept...)
		}
	}
	return append(messages, m)
}

// compact 摘要完成后立即安装，用于手动压缩和预压缩。
func compact(ctx context.Context, r *run, cut int) (failed bool) {
	message, failed := prepareCompaction(ctx, r, cut)
	if message.Content != "" {
		r.add(message)
	}
	return failed
}

// prepareCompaction 生成摘要，调用方决定何时安装。失败时记录通知，保留原上下文。
func prepareCompaction(ctx context.Context, r *run, cut int) (SystemMessage, bool) {
	if !canCut(r.Messages, cut) {
		return SystemMessage{}, false
	}
	r.emit(Event{Type: EventCompactionStart})
	summary, err := summarize(ctx, r, r.Messages[:cut])
	if ctx.Err() != nil {
		return SystemMessage{}, true // 界面在本轮结束时清除进行中的压缩提示。
	}
	if err != nil {
		r.add(SystemMessage{
			Type:      SystemMessageTypeNotice,
			Content:   "上下文压缩失败，已继续使用原上下文：" + err.Error(),
			Timestamp: time.Now(),
		})
		return SystemMessage{}, true
	}
	return SystemMessage{
		Type: SystemMessageTypeCompaction, Content: summary,
		Kept: len(r.Messages) - cut, Timestamp: time.Now(),
	}, false
}

// summarize 用同一个模型为 messages 写摘要：独立的一次请求，没有工具，增量不外发。
func summarize(ctx context.Context, r *run, messages []Message) (string, error) {
	const prefix = "<conversation>\n"
	const suffix = "\n</conversation>\n\n" + summaryInstructions
	conversation := strings.Join(transcript(messages, clipChars), "\n\n")
	request := func() StreamRequest {
		prompt := UserMessage{
			Content:   []Content{TextContent{Text: prefix + conversation + suffix}},
			Timestamp: time.Now(),
		}
		return StreamRequest{
			Model: r.Model.Ref, SystemPrompt: summarySystemPrompt, Messages: []Message{prompt},
			ThinkingLevel: r.Thinking,
		}
	}
	shrink := func() bool {
		overhead := estimateText(summarySystemPrompt + prefix + suffix)
		budget := (estimateText(conversation) - overhead) / 2
		if budget <= 0 {
			return false
		}
		conversation = shrinkTranscript(transcript(messages, clipChars/2), budget,
			isCompaction(messages[0]))
		return conversation != ""
	}
	final := streamWithRetry(ctx, r, request, func(Delta) {}, shrink)
	summary := strings.TrimSpace(TextOf(final.Content))
	switch {
	case final.StopReason == StopReasonCanceled:
		return "", context.Canceled
	case final.StopReason == StopReasonError:
		return "", errors.New(final.ErrorMessage)
	case final.StopReason != StopReasonStop:
		return "", fmt.Errorf("摘要没有写完（%s）", final.StopReason)
	case summary == "":
		return "", errors.New("摘要为空")
	}
	return summary, nil
}

const transcriptUser = "[用户]: "
const transcriptInTurn = "[用户补充]: "

// transcript 将消息排成文本。工具结果和参数只保留首尾，思考内容不参与摘要。
func transcript(messages []Message, limit int) []string {
	var parts []string
	for _, m := range messages {
		switch m := m.(type) {
		case SystemMessage:
			if m.Type == SystemMessageTypeCompaction {
				parts = append(parts, "[此前的摘要]\n"+m.Content)
			}
		case UserMessage:
			if text := TextOf(m.Content); text != "" {
				label := transcriptUser
				if m.InTurn {
					label = transcriptInTurn
				}
				parts = append(parts, label+text)
			}
		case AssistantMessage:
			if text := TextOf(m.Content); text != "" {
				parts = append(parts, "[助手]: "+text)
			}
			var calls []string
			for _, block := range m.Content {
				if call, ok := block.(ToolCall); ok {
					calls = append(calls,
						fmt.Sprintf("%s(%s)", call.Name, clip(string(call.Arguments), limit)))
				}
			}
			if len(calls) > 0 {
				parts = append(parts, "[助手调用工具]: "+strings.Join(calls, "; "))
			}
		case ToolResultMessage:
			if text := TextOf(m.Content); text != "" {
				parts = append(parts, "[工具结果]: "+clip(text, limit))
			}
		}
	}
	return parts
}

// shrinkTranscript 先给最近任务的用户原文分配预算，再保留前次摘要和近期进展。
func shrinkTranscript(parts []string, budget int64, hasSummary bool) string {
	const omitted = "[已省略部分对话]"
	budget -= estimateText(omitted) + 2
	selected := make([]string, len(parts))
	keep := func(i int, limit int64) {
		if part := fitTranscriptPart(parts[i], min(limit, budget-2)); part != "" {
			selected[i] = part
			budget -= estimateText(part) + 2
		}
	}
	task := len(parts) - 1
	for task >= 0 && !strings.HasPrefix(parts[task], transcriptUser) {
		task--
	}
	userBudget := budget / 2
	before := budget
	if task >= 0 {
		keep(task, userBudget)
	}
	for i := len(parts) - 1; i > task && before-budget < userBudget; i-- {
		if strings.HasPrefix(parts[i], transcriptInTurn) {
			keep(i, userBudget-(before-budget))
		}
	}
	start := 0
	if hasSummary {
		keep(0, budget/2)
		start = 1
	}
	for i := len(parts) - 1; i >= start; i-- {
		if selected[i] == "" {
			keep(i, budget)
			if selected[i] != parts[i] {
				break
			}
		}
	}
	if !slices.ContainsFunc(selected[start:], func(part string) bool { return part != "" }) {
		return ""
	}
	var result []string
	omittedAdded := false
	for _, part := range selected {
		if part != "" {
			result = append(result, part)
		} else if !omittedAdded {
			result = append(result, omitted)
			omittedAdded = true
		}
	}
	return strings.Join(result, "\n\n")
}

func fitTranscriptPart(part string, budget int64) string {
	if budget <= 0 {
		return ""
	}
	if estimateText(part) <= budget {
		return part
	}
	return clip(part, int(budget)) // 单个字符的估算至多是一个 token。
}

// TextOf 连接内容里的文本；图片以占位文字表示。
func TextOf(content []Content) string {
	var parts []string
	for _, block := range content {
		switch b := block.(type) {
		case TextContent:
			parts = append(parts, b.Text)
		case ImageContent:
			parts = append(parts, "[图片]")
		}
	}
	return strings.Join(parts, "\n")
}

// clip 将长文本截到 limit 个字符，保留开头的背景和末尾的结论。
func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	const omitted = "\n[… 已省略中间内容]\n"
	kept := limit - utf8.RuneCountInString(omitted)
	if kept <= 0 {
		return ""
	}
	runes := []rune(s)
	return string(runes[:kept/2]) + omitted + string(runes[len(runes)-(kept+1)/2:])
}

// cutPoint 返回压缩后保留的第一条消息的下标，没有可压缩的内容时返回 -1。
// 从尾部往回累计到至少 keep 个 token，再就近找一条用户或助手消息作为起点：
// 起点不会是工具结果，所以工具调用与它的结果不会被拆开。
func cutPoint(messages []Message, keep int64) int {
	var kept int64
	from := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if kept += estimateMessage(messages[i]); kept >= keep {
			from = i
			break
		}
	}
	if from <= 0 {
		return -1 // 全部消息加起来也不到 keep，没有可压缩的
	}
	cut := from
	for cut < len(messages) && !isBoundary(messages[cut]) {
		cut++
	}
	if cut == len(messages) {
		for cut = from; cut > 0 && !isBoundary(messages[cut]); cut-- {
		}
	}
	// 切点前只剩此前的摘要时，没有新的内容可摘要。
	if !canCut(messages, cut) {
		return -1
	}
	return cut
}

// isBoundary 报告消息能否作为保留段的起点：用户消息或助手消息。
func isBoundary(m Message) bool {
	switch m.(type) {
	case UserMessage, AssistantMessage:
		return true
	}
	return false
}

// isCompaction 报告消息是否是压缩记录。
func isCompaction(m Message) bool {
	s, ok := m.(SystemMessage)
	return ok && s.Type == SystemMessageTypeCompaction
}

// Tokens 估算上下文的大小：压缩点之后最新一条带用量的助手响应的用量，加上它之后各消息的估算；
// 没有这样的响应时整体估算（系统提示词与工具定义不计，预留量已经覆盖）。
// 保留段里的响应是压缩之前的，它们的用量含有已被摘要掉的内容，不能用。
func Tokens(messages []Message) int64 {
	trusted := 0
	if len(messages) > 0 && isCompaction(messages[0]) {
		trusted = 1 + messages[0].(SystemMessage).Kept
	}
	for i := len(messages) - 1; i >= trusted; i-- {
		if m, ok := messages[i].(AssistantMessage); ok && m.Usage.TotalTokens > 0 {
			return m.Usage.TotalTokens + estimateMessages(messages[i+1:])
		}
	}
	return estimateMessages(messages)
}

// estimateMessages 估算一组消息的大小。
func estimateMessages(messages []Message) int64 {
	var n int64
	for _, m := range messages {
		n += estimateMessage(m)
	}
	return n
}

// estimateMessage 估算一条消息的大小。
func estimateMessage(m Message) int64 {
	switch m := m.(type) {
	case SystemMessage:
		return estimateText(m.Content)
	case UserMessage:
		return estimateContent(m.Content)
	case AssistantMessage:
		return estimateContent(m.Content)
	case ToolResultMessage:
		return estimateContent(m.Content)
	}
	return 0
}

// estimateContent 估算内容块的大小。
func estimateContent(content []Content) int64 {
	var n int64
	for _, block := range content {
		switch b := block.(type) {
		case TextContent:
			n += estimateText(b.Text)
		case ThinkingContent:
			n += estimateText(b.Thinking) + estimateText(b.Signature)
		case ToolCall:
			n += estimateText(b.Name) + estimateText(string(b.Arguments))
		case ImageContent:
			n += imageTokens
		}
	}
	return n
}

// estimateText 估算文本的 token 数：ASCII 约四个字节一个，其余字符按一个字符一个计，偏保守。
func estimateText(s string) int64 {
	var ascii, other int64
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}
