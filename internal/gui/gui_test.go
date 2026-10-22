package gui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 一轮对话的实时更新，与它保存后回放的历史：核心流式发出的是一段段增量与工具调用的状态，
// 回放的是定稿的整块。
var (
	call = []byte(`{"path":"a.go"}`)
	live = []protocol.SessionUpdate{
		protocol.StateUpdate{Running: true},
		protocol.UserMessageChunk{MessageID: "m1", Content: protocol.Text("看看 a.go"),
			Meta: protocol.MessageMeta{Seq: 2}},
		protocol.AgentMessageChunk{MessageID: "m2", Content: protocol.Text("好")},
		protocol.AgentMessageChunk{MessageID: "m2", Content: protocol.Text("的。")},
		protocol.ToolCall{ToolCallID: "c1", Name: "read", RawInput: []byte(`{"pa`),
			Status: protocol.ToolPending, Meta: protocol.MessageMeta{MessageID: "m2"}},
		protocol.ToolCallUpdate{ToolCallID: "c1", RawInput: call},
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolInProgress},
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolCompleted,
			Content: []protocol.ToolCallContent{{Content: protocol.Text("package a")}}},
		protocol.StateUpdate{},
	}
	replayed = []protocol.SessionUpdate{
		protocol.UserMessageChunk{MessageID: "h2", Content: protocol.Text("看看 a.go"),
			Meta: protocol.MessageMeta{Seq: 2}},
		protocol.AgentMessageChunk{MessageID: "h3", Content: protocol.Text("好的。")},
		protocol.ToolCall{ToolCallID: "c1", Name: "read", RawInput: call,
			Status: protocol.ToolPending, Meta: protocol.MessageMeta{MessageID: "h3"}},
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolCompleted,
			Content: []protocol.ToolCallContent{{Content: protocol.Text("package a")}}},
	}
)

// shape 是显示项里与渲染有关的部分。
func shape(ch *chat) []string {
	var out []string
	for _, it := range ch.items {
		out = append(out, strings.Join([]string{
			strings.Repeat("*", int(it.kind)), it.text, it.tool, it.args, it.result,
		}, "|"))
		if !it.done {
			out = append(out, "未完成")
		}
	}
	return out
}

// 实时更新与回放的历史走同一条路，画出同样的对话：用户、助手、带结果的工具卡片，都已定稿。
func TestLiveAndReplayAgree(t *testing.T) {
	ch := &chat{}
	for _, u := range live {
		ch.apply(u)
	}
	history := &chat{}
	history.load(replayed, 1)

	if got := shape(ch); len(ch.items) != 3 || !slices.Equal(got, shape(history)) {
		t.Errorf("实时 = %q\n回放 = %q，期望一样的三项", got, shape(history))
	}
	if ch.running {
		t.Error("一轮结束后仍在运行")
	}
}

// 压缩进行中是一行占位，结束时由摘要取代，失败时由通知取代。重试时这条回复已流出的文字
// 与半截的工具调用都丢掉，压缩记录还在，提示一行在新的输出到来时消失。
func TestCompactionAndRetryRows(t *testing.T) {
	ch := &chat{}
	ch.add(itemUser, "你好")
	running := func() bool {
		return slices.ContainsFunc(ch.items, func(it *item) bool {
			return it.kind == itemCompaction && !it.done
		})
	}
	compaction := func(status protocol.CompactionStatus, s string) protocol.CompactionUpdate {
		return protocol.CompactionUpdate{CompactionID: "k", Status: status, Summary: s, Error: s}
	}

	ch.apply(compaction(protocol.CompactionInProgress, ""))
	if len(ch.items) != 2 || !running() {
		t.Fatalf("压缩开始后显示项 = %d，期望 2 项且有进行中的占位", len(ch.items))
	}
	ch.apply(compaction(protocol.CompactionCompleted, "摘要"))
	if got := ch.items[1]; len(ch.items) != 2 || got.kind != itemCompaction || !got.done ||
		got.text != "摘要" {
		t.Fatalf("压缩结束后显示项 = %+v，期望摘要取代占位", got)
	}

	ch.apply(protocol.AgentMessageChunk{MessageID: "m1", Content: protocol.Text("半截")})
	ch.apply(protocol.ToolCall{ToolCallID: "c1", Name: "read",
		Meta: protocol.MessageMeta{MessageID: "m1"}})
	ch.apply(protocol.RetryUpdate{MessageID: "m1", Attempt: 1, Max: 4, Error: "503"})
	if len(ch.items) != 2 || ch.items[1].kind != itemCompaction || ch.retry == "" {
		t.Fatalf("重试后显示项 = %d，提示 = %q，期望只丢掉这条回复的内容、留下压缩记录并有提示",
			len(ch.items), ch.retry)
	}
	ch.apply(protocol.AgentMessageChunk{MessageID: "m1", Content: protocol.Text("好")})
	if ch.retry != "" {
		t.Errorf("新输出到来后提示 = %q，期望消失", ch.retry)
	}

	ch.apply(compaction(protocol.CompactionInProgress, ""))
	ch.apply(compaction(protocol.CompactionFailed, "上下文压缩失败"))
	if last := ch.items[len(ch.items)-1]; running() || last.kind != itemNotice {
		t.Errorf("压缩失败后进行中 = %v，最后一项 = %v，期望占位被通知取代", running(), last.kind)
	}
}

// 往前加载的一页可能把工具调用与它的结果拆开：结果在已有的后一页里，
// 调用在新加载的这一页里，合起来仍是一张卡片。
func TestPrependMergesSplitToolCall(t *testing.T) {
	ch := &chat{}
	ch.load([]protocol.SessionUpdate{
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolCompleted,
			Content: []protocol.ToolCallContent{{Content: protocol.Text("package a")}}},
		protocol.AgentMessageChunk{MessageID: "h6", Content: protocol.Text("好的。")},
	}, 5)
	ch.prepend([]protocol.SessionUpdate{
		protocol.UserMessageChunk{MessageID: "h3", Content: protocol.Text("看看"),
			Meta: protocol.MessageMeta{Seq: 3}},
		protocol.ToolCall{ToolCallID: "c1", Name: "read", RawInput: call},
	}, 3)

	if len(ch.items) != 3 || ch.oldest != 3 {
		t.Fatalf("显示项 = %d，最早 Seq = %d，期望 3 与 3", len(ch.items), ch.oldest)
	}
	if got := ch.items[1]; got.kind != itemTool || got.args != string(call) ||
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
	a := newApp()
	a.infos = []protocol.SessionInfo{
		{SessionID: "s1", Title: "测试会话", Cwd: "/tmp", UpdatedAt: time.Now()},
	}
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
	a := newApp()
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
