package gui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/host"
)

// 流式事件先临时展示，定稿消息到达后校正，工具卡片接收结果。
func TestApplyEvents(t *testing.T) {
	ch := &chat{}
	text := func(s string) agent.Event {
		return agent.Event{Type: agent.EventTypeMessageUpdate,
			Delta: agent.Delta{Type: agent.DeltaTypeText, Text: s}}
	}
	call := agent.ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"path":"a.go"}`)}
	events := []agent.Event{
		{Type: agent.EventTypeMessageEnd, Message: agent.UserMessage{
			Content: []agent.Content{agent.TextContent{Text: "看看 a.go"}}}},
		{Type: agent.EventTypeMessageStart, Message: agent.AssistantMessage{}},
		text("好"), text("的"),
		{Type: agent.EventTypeMessageUpdate, Delta: agent.Delta{
			Type: agent.DeltaTypeToolCall, ToolCall: call}},
		{Type: agent.EventTypeMessageEnd, Message: agent.AssistantMessage{
			Content: []agent.Content{agent.TextContent{Text: "好的。"}, call}}},
		{Type: agent.EventTypeToolExecutionStart, ToolCallID: "c1", ToolName: "read"},
		{Type: agent.EventTypeToolExecutionEnd, ToolCallID: "c1", Result: agent.ToolResult{
			Content: []agent.Content{agent.TextContent{Text: "package a"}}}},
	}
	for _, e := range events {
		ch.apply(e)
	}

	if len(ch.items) != 3 {
		t.Fatalf("显示项 = %d，期望 3（用户、助手、工具）", len(ch.items))
	}
	if got := ch.items[1]; got.text != "好的。" || !got.done {
		t.Errorf("助手项 = %+v，期望以定稿文本为准且已完成", got)
	}
	if got := ch.items[2]; got.result != "package a" || got.running || !got.done {
		t.Errorf("工具项 = %+v，期望带结果且已结束", got)
	}
}

// 往前加载的一页可能把工具调用与它的结果拆开：结果在已有的后一页里，
// 调用在新加载的这一页里，合起来仍是一张卡片。
func TestPrependMergesSplitToolCall(t *testing.T) {
	call := agent.ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"path":"a.go"}`)}
	ch := &chat{}
	ch.load([]host.Record{
		{Seq: 5, Message: agent.ToolResultMessage{ToolCallID: "c1", ToolName: "read",
			Content: []agent.Content{agent.TextContent{Text: "package a"}}}},
		{Seq: 6, Message: agent.AssistantMessage{
			Content: []agent.Content{agent.TextContent{Text: "好的。"}}}},
	})
	ch.prepend([]host.Record{
		{Seq: 3, Message: agent.UserMessage{
			Content: []agent.Content{agent.TextContent{Text: "看看"}}}},
		{Seq: 4, Message: agent.AssistantMessage{Content: []agent.Content{call}}},
	})

	if len(ch.items) != 3 || ch.oldest != 3 {
		t.Fatalf("显示项 = %d，最早 Seq = %d，期望 3 与 3", len(ch.items), ch.oldest)
	}
	if got := ch.items[1]; got.kind != itemTool || got.args != string(call.Arguments) ||
		got.result != "package a" || !got.done {
		t.Errorf("工具项 = %+v，期望带参数与结果", got)
	}
}

// 视图把对话内容画出来；用无窗口的 Tester 运行。
func TestViewShowsConversation(t *testing.T) {
	a := newApp(nil)
	a.current.add(itemUser, "你好")
	a.current.add(itemAssistant, "你好，我是 Atlas。")
	tool := a.current.add(itemTool, "")
	tool.tool, tool.args = "read", `{"path":"main.go"}`

	tt := ui.NewTester(a.view, 900, 600)
	for _, text := range []string{"你好", "新对话", "发送"} {
		if !tt.HasText(text) {
			t.Errorf("界面缺少 %q，现有文本 %q", text, tt.Texts())
		}
	}
}
