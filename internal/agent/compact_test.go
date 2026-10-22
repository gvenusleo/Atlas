package agent

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// text 是约 n 个 token 的纯 ASCII 文本。
func text(n int) string { return strings.Repeat("abcd", n) }

func userText(s string) UserMessage { return UserMessage{Content: []Content{TextContent{Text: s}}} }

func assistantText(s string) AssistantMessage {
	return AssistantMessage{StopReason: StopReasonStop, Content: []Content{TextContent{Text: s}}}
}

func toolResult(s string) ToolResultMessage {
	return ToolResultMessage{
		ToolCallID: "c", ToolName: "read", Content: []Content{TextContent{Text: s}},
	}
}

func callMessage() AssistantMessage {
	return AssistantMessage{StopReason: StopReasonToolUse, Content: []Content{
		ToolCall{ID: "c", Name: "read", Arguments: []byte(`{"path":"a"}`)}}}
}

// 切点只落在用户或助手消息上，工具调用与它的结果不会被拆开；没有可压缩的内容时不压缩。
func TestCutPoint(t *testing.T) {
	summary := SystemMessage{Type: SystemMessageTypeCompaction, Content: "摘要", Kept: 1}
	messages := []Message{
		userText("你好"), callMessage(), toolResult(text(1000)), toolResult(text(1000)),
		userText("继续"), assistantText("好"),
	}
	for _, tt := range []struct {
		name     string
		messages []Message
		keep     int64
		want     int
	}{
		{"尾部落在工具结果上时往后找", messages, 1500, 4},
		{"全部加起来不到保留量", messages, 3000, -1},
		{"保留量很小时只保留最后一条", messages, 1, 5},
		{"尾部都是工具结果时往前找", messages[:3], 500, 1},
		{"切点前只有此前的摘要", []Message{summary, userText(text(500)), assistantText("好")},
			100, -1},
	} {
		if got := cutPoint(tt.messages, tt.keep); got != tt.want {
			t.Errorf("%s：cutPoint = %d，期望 %d", tt.name, got, tt.want)
		}
	}
}

// 压缩之后，保留段里的助手响应的用量含有已被摘要掉的内容，不能当作现在的上下文大小。
func TestContextTokensIgnoresUsageBeforeCompaction(t *testing.T) {
	stale := assistantText("好")
	stale.Usage = Usage{InputTokens: 90000, TotalTokens: 90000}
	messages := []Message{
		SystemMessage{Type: SystemMessageTypeCompaction, Content: "摘要", Kept: 2},
		userText("你好"), stale,
	}
	if got := Tokens(messages); got >= 1000 {
		t.Errorf("压缩后没有新响应时 Tokens = %d，不应采用压缩前的 90000", got)
	}

	fresh := assistantText("好")
	fresh.Usage = Usage{InputTokens: 5000, TotalTokens: 5000}
	messages = append(messages, fresh, userText(text(250)))
	if got := Tokens(messages); got != 5000+250 {
		t.Errorf("Tokens = %d，期望 5250：压缩后的响应用量加其后消息的估算", got)
	}
}

// 上下文由历史依次 Fold 得到：压缩记录换成摘要加它之前的最后 Kept 条，提示词快照不进上下文；
// Kept 超过已有条数时全部保留。
func TestFold(t *testing.T) {
	var messages []Message
	for _, m := range []Message{
		SystemMessage{Type: SystemMessageTypePrompt, Content: "提示词"},
		userText("u1"), assistantText("a1"), userText("u2"), assistantText("a2"),
		SystemMessage{Type: SystemMessageTypeCompaction, Content: "摘要", Kept: 2},
		userText("u3"),
	} {
		messages = Fold(messages, m)
	}
	if len(messages) != 4 || !isCompaction(messages[0]) ||
		TextOf(messages[1].(UserMessage).Content) != "u2" {
		t.Errorf("上下文 = %v，期望 [摘要, u2, a2, u3]", messages)
	}
	all := SystemMessage{Type: SystemMessageTypeCompaction, Content: "摘要", Kept: 99}
	if got := Fold(messages, all); len(got) != 5 {
		t.Errorf("Kept 过大时上下文 = %d 条，期望摘要加已有的 4 条", len(got))
	}
}

// compactRun 用 window 大小的上下文跑一轮：steps 依次决定每次模型调用的结果，
// 摘要请求（系统提示词不同）与主请求共用。这一轮不修改传入的上下文。
func compactRun(
	t *testing.T, window int64, history []Message, steps ...AssistantMessage,
) (requests []StreamRequest, events []Event, messages []Message) {
	t.Helper()
	in := Input{Messages: history, Model: Model{Ref: "m", Window: window,
		Stream: func(_ context.Context, req StreamRequest, emit func(Delta)) AssistantMessage {
			requests = append(requests, req)
			emit(Delta{Type: DeltaTypeText, Text: "增量"})
			return steps[min(len(requests), len(steps))-1]
		}}}
	before := slices.Clone(history)
	messages = Run(t.Context(), in, userText("继续"), func(e Event) {
		events = append(events, e)
	})
	if !reflect.DeepEqual(history, before) {
		t.Error("Run 修改了传入的上下文")
	}
	return requests, events, messages
}

// 上下文接近窗口上限时先压缩：摘要请求不带工具、增量不外发，主请求看到的是摘要加保留段，
// 压缩记录排在新响应之前入历史。
func TestCompactOnThreshold(t *testing.T) {
	history := func() []Message {
		return []Message{
			userText(text(750)), assistantText(text(750)),
			userText(text(750)), assistantText(text(750)),
		}
	}
	summary := assistantText("## 目标\n继续工作")
	failure := AssistantMessage{StopReason: StopReasonError, ErrorMessage: "boom"}
	done := assistantText("好")

	t.Run("成功", func(t *testing.T) {
		requests, events, messages := compactRun(t, 1000, history(), summary, done)
		if len(requests) != 2 || requests[0].SystemPrompt != summarySystemPrompt {
			t.Fatalf("请求 = %d 个，期望先摘要再主请求", len(requests))
		}
		entry, _ := requests[1].Messages[0].(SystemMessage)
		if entry.Type != SystemMessageTypeCompaction || entry.Kept != 2 ||
			len(requests[1].Messages) != 3 {
			t.Errorf("主请求 = %d 条、第一条 = %+v，期望摘要加保留的两条（旧的助手回复与新提示）",
				len(requests[1].Messages), entry)
		}
		if len(messages) != 3 || !isCompaction(messages[1]) || messages[2].Role() != RoleAssistant {
			t.Errorf("新消息 = %v，期望 [提示, 压缩记录, 响应]", messages)
		}
		updates := 0
		for _, e := range events {
			if e.Type == EventDelta {
				updates++
			}
		}
		if updates != 1 {
			t.Errorf("增量事件有 %d 个，期望 1 个（主请求的）：摘要的增量不能外发", updates)
		}
	})

	t.Run("摘要失败则继续使用原上下文", func(t *testing.T) {
		requests, _, messages := compactRun(t, 1000, history(), failure, done)
		if len(requests) != 2 || len(requests[1].Messages) != 6 {
			t.Fatalf("请求 %d 个、主请求 %d 条，期望主请求带着原来的 5 条加一条通知",
				len(requests), len(requests[1].Messages))
		}
		notice, ok := messages[1].(SystemMessage)
		if !ok || notice.Type != SystemMessageTypeNotice ||
			!strings.Contains(notice.Content, "压缩失败") {
			t.Errorf("新消息 = %v，期望压缩失败的通知", messages)
		}
		if isCompaction(requests[1].Messages[0]) {
			t.Error("摘要失败后历史不应被压缩")
		}
	})
}

// 请求溢出时压缩一次再重发同一个 turn，失败的溢出尝试不入历史；压缩后仍溢出就不再重试。
// 配置里没有窗口（0）、对话又不长时同样要能压缩：保留量按刚溢出的大小算。
func TestCompactOnOverflow(t *testing.T) {
	overflow := AssistantMessage{
		StopReason: StopReasonError, ErrorMessage: "too long", ErrorKind: ErrorKindContextOverflow,
	}
	history := func() []Message {
		return []Message{userText(text(300)), assistantText(text(300))}
	}
	summary, done := assistantText("摘要"), assistantText("好")

	for _, window := range []int64{1000, 0} {
		requests, _, messages := compactRun(t, window, history(), overflow, summary, done)
		if len(requests) != 3 || len(messages) != 3 || !isCompaction(messages[1]) ||
			messages[2].(AssistantMessage).StopReason != StopReasonStop {
			t.Errorf("窗口 %d：请求 %d 个、新消息 %v，期望溢出→摘要→重发成功，历史里没有失败的尝试",
				window, len(requests), messages)
		}
	}

	requests, _, messages := compactRun(t, 1000, history(), overflow, summary, overflow)
	last := messages[len(messages)-1].(AssistantMessage)
	if len(requests) != 3 || last.StopReason != StopReasonError {
		t.Errorf("请求 %d 个、结束 %q，期望压缩后仍溢出就以错误收尾、不再重试",
			len(requests), last.StopReason)
	}
}
