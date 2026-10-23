package agent

import "testing"

func userText(s string) UserMessage { return UserMessage{Content: []Content{TextContent{Text: s}}} }

func assistantText(s string) AssistantMessage {
	return AssistantMessage{StopReason: StopReasonStop, Content: []Content{TextContent{Text: s}}}
}

// 上下文由历史依次 Fold 得到：压缩记录换成摘要加它之前的最后 Kept 条，系统提示词快照与通知
// 不进上下文；Kept 超过已有条数时全部保留。
func TestFold(t *testing.T) {
	var messages []Message
	for _, m := range []Message{
		SystemMessage{Type: SystemMessageTypeSystemPrompt, Content: "系统提示词"},
		userText("u1"), assistantText("a1"), userText("u2"), assistantText("a2"),
		SystemMessage{Type: SystemMessageTypeNotice, Content: "通知"},
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
