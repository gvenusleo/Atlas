package gui

import (
	"testing"
	"time"

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

// 重试时这次响应已流出的文字和半截的工具调用都要丢掉，只留之前的内容；
// 提示一行在新的输出到来时消失；用来标记的显示项不在了就什么也不丢。
func TestRetryDiscardsPartialMessage(t *testing.T) {
	ch := &chat{}
	ch.add(itemUser, "你好")
	delta := func(d agent.Delta) agent.Event {
		return agent.Event{Type: agent.EventTypeMessageUpdate, Delta: d}
	}
	partial := agent.ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"pa`)}
	for _, e := range []agent.Event{
		{Type: agent.EventTypeMessageStart, Message: agent.AssistantMessage{}},
		delta(agent.Delta{Type: agent.DeltaTypeText, Text: "半截"}),
		delta(agent.Delta{Type: agent.DeltaTypeToolCall, ToolCall: partial}),
		{Type: agent.EventTypeRetry, Retry: agent.RetryInfo{
			Attempt: 1, Max: 4, Delay: 2 * time.Second, Error: "503"}},
	} {
		ch.apply(e)
	}
	if len(ch.items) != 1 || ch.retry == "" {
		t.Fatalf("重试后显示项 = %d，提示 = %q，期望只剩用户消息并有提示", len(ch.items), ch.retry)
	}

	ch.apply(delta(agent.Delta{Type: agent.DeltaTypeText, Text: "好"}))
	if len(ch.items) != 2 || ch.retry != "" || ch.items[1].text != "好" {
		t.Errorf("新输出到来后显示项 = %d，提示 = %q，期望 2 项且提示消失", len(ch.items), ch.retry)
	}

	ch.tail = &item{}
	ch.apply(agent.Event{Type: agent.EventTypeRetry, Retry: agent.RetryInfo{Attempt: 2, Max: 4}})
	if len(ch.items) != 2 {
		t.Errorf("标记不在列表里时显示项 = %d，期望什么也不丢", len(ch.items))
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

// 会话行的 ⋯ 菜单：选择在菜单关闭后的下一帧回报，那时指针在菜单上，不在这一行，
// 行尾的 ⋯ 按钮已换成零尺寸；它的标识若随位置变，选择就丢了，点“重命名”没有反应。
func TestSessionMenuChoiceAfterPointerLeaves(t *testing.T) {
	a := newApp(nil)
	a.infos = []host.Info{{ID: "s1", Name: "测试会话", Cwd: "/tmp", UpdatedAt: time.Now()}}
	tt := ui.NewTester(a.view, 900, 600)

	row, _ := tt.Find("测试会话")
	tt.Move(row.X+10, row.Y+row.H/2)
	tt.Advance(50 * time.Millisecond)
	more, ok := tt.Find("更多")
	if !ok {
		t.Fatalf("悬停后没有 ⋯ 按钮，现有文本 %q", tt.Texts())
	}
	tt.ClickAt(more.X+more.W/2, more.Y+more.H/2)
	tt.Advance(50 * time.Millisecond)
	tt.Move(600, 400) // 指针移到菜单上，离开会话行
	tt.Advance(50 * time.Millisecond)
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(50 * time.Millisecond)

	if !a.renameOpen || a.renameID != "s1" {
		t.Errorf("renameOpen = %v, renameID = %q，期望打开 s1 的重命名对话框",
			a.renameOpen, a.renameID)
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
