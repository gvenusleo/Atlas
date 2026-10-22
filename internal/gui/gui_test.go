package gui

import (
	"slices"
	"strings"
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
		return agent.Event{Type: agent.EventDelta,
			Delta: agent.Delta{Type: agent.DeltaTypeText, Text: s}}
	}
	call := agent.ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"path":"a.go"}`)}
	events := []agent.Event{
		{Type: agent.EventMessageEnd, Message: agent.UserMessage{
			Content: []agent.Content{agent.TextContent{Text: "看看 a.go"}}}},
		{Type: agent.EventMessageStart, Message: agent.AssistantMessage{}},
		text("好"), text("的"),
		{Type: agent.EventDelta, Delta: agent.Delta{
			Type: agent.DeltaTypeToolCall, ToolCall: call}},
		{Type: agent.EventMessageEnd, Message: agent.AssistantMessage{
			Content: []agent.Content{agent.TextContent{Text: "好的。"}, call}}},
		{Type: agent.EventToolStart, Call: call},
		{Type: agent.EventMessageEnd, Message: agent.ToolResultMessage{
			ToolCallID: "c1", ToolName: "read",
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
	if got := ch.items[2]; got.result != "package a" || !got.done {
		t.Errorf("工具项 = %+v，期望带结果且已结束", got)
	}
}

// 压缩进行中是一行占位，结束时由摘要取代，失败时由通知取代。压缩发生在一次响应中途时，
// 这条响应之后被重试：已流出的文字与半截的工具调用都丢掉，压缩记录还在，
// 提示一行在新的输出到来时消失；用来标记的显示项不在了就什么也不丢。
func TestCompactionAndRetryRows(t *testing.T) {
	ch := &chat{usage: 5000}
	ch.add(itemUser, "你好")
	event := func(typ agent.EventType, m agent.Message) agent.Event {
		return agent.Event{Type: typ, Message: m}
	}
	delta := func(d agent.Delta) agent.Event {
		return agent.Event{Type: agent.EventDelta, Delta: d}
	}
	compactStart := agent.Event{Type: agent.EventCompactStart}
	retry := agent.Event{Type: agent.EventRetry, Retry: agent.RetryInfo{
		Attempt: 1, Max: 4, Delay: 2 * time.Second, Error: "503"}}
	running := func() bool {
		return slices.ContainsFunc(ch.items, func(it *item) bool {
			return it.kind == itemCompaction && !it.done
		})
	}
	compaction := agent.SystemMessage{Type: agent.SystemMessageTypeCompaction}

	ch.apply(event(agent.EventMessageStart, agent.AssistantMessage{}))
	ch.apply(compactStart)
	if len(ch.items) != 2 || !running() {
		t.Fatalf("压缩开始后显示项 = %d，进行中 = %v，期望 2 项且有进行中的占位",
			len(ch.items), running())
	}

	compaction.Content, compaction.Kept = "摘要", 1
	ch.apply(event(agent.EventMessageEnd, compaction))
	if got := ch.items[1]; len(ch.items) != 2 || got.kind != itemCompaction || !got.done ||
		got.text != "摘要" || ch.usage != 0 {
		t.Fatalf("压缩结束后显示项 = %+v，usage = %d，期望摘要取代占位且用量清零", got, ch.usage)
	}

	partial := agent.ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"pa`)}
	ch.apply(delta(agent.Delta{Type: agent.DeltaTypeText, Text: "半截"}))
	ch.apply(delta(agent.Delta{Type: agent.DeltaTypeToolCall, ToolCall: partial}))
	ch.apply(retry)
	if len(ch.items) != 2 || ch.items[1].kind != itemCompaction || ch.retry == "" {
		t.Fatalf("重试后显示项 = %d，提示 = %q，期望只丢掉半截内容、留下压缩记录并有提示",
			len(ch.items), ch.retry)
	}
	ch.apply(delta(agent.Delta{Type: agent.DeltaTypeText, Text: "好"}))
	if ch.retry != "" {
		t.Errorf("新输出到来后提示 = %q，期望消失", ch.retry)
	}

	ch.apply(compactStart)
	notice := agent.SystemMessage{Type: agent.SystemMessageTypeNotice, Content: "上下文压缩失败"}
	ch.apply(event(agent.EventMessageEnd, notice))
	if last := ch.items[len(ch.items)-1]; running() || last.kind != itemNotice {
		t.Errorf("压缩失败后进行中 = %v，最后一项 = %v，期望占位被通知取代", running(), last.kind)
	}

	before := len(ch.items)
	ch.tail = &item{}
	ch.apply(retry)
	if len(ch.items) != before {
		t.Errorf("标记不在列表里时显示项 %d → %d，期望什么也不丢", before, len(ch.items))
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

// 一次 run 的输出是一块：以用户消息为界，里面穿插的文字、思考与工具不会把它切开；
// 复制的是其中所有的助手文字，思考与工具不算。
func TestRunIsOneBlock(t *testing.T) {
	ch := &chat{}
	ch.add(itemUser, "问")
	ch.add(itemThinking, "想")
	ch.add(itemAssistant, "甲")
	ch.add(itemTool, "")
	ch.add(itemTool, "")
	ch.add(itemAssistant, "乙")
	ch.add(itemUser, "再问")
	ch.add(itemAssistant, "丙")

	blocks := ch.blocks()
	if len(blocks) != 4 {
		t.Fatalf("块 = %d，期望 4（用户、一次 run、用户、一次 run）", len(blocks))
	}
	run := blocks[1].items
	if got := len(parts(run)); got != 4 {
		t.Errorf("段 = %d，期望 4（思考、甲、两次工具调用一组、乙）", got)
	}
	if text, _ := replyText(run); text != "甲\n\n乙" {
		t.Errorf("复制的文本 = %q，期望 %q", text, "甲\n\n乙")
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

// 下拉的值是每帧重算的副本，选中的值在 onChange 里读，不会被下一帧的重算覆盖；
// disabled 时点不开。
func TestDropdown(t *testing.T) {
	var picked string
	pick := func(disabled bool) *ui.Tester {
		picked = ""
		tt := ui.NewTester(func(c *ui.Context) {
			theme := applyTheme(c)
			value := "a"
			dropdown(c, theme, "选择", &value, []string{"a", "b"}, strings.ToUpper, disabled,
				func() { picked = value })
		}, 400, 300)
		tt.Advance(time.Second)
		_ = tt.Click("选择")
		tt.Advance(time.Second)
		return tt
	}

	tt := pick(false)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if picked != "b" {
		t.Errorf("选中 = %q，期望 b", picked)
	}

	if tt := pick(true); tt.HasText("B") {
		t.Error("disabled 的下拉不应打开")
	}
}
