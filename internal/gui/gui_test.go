package gui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// 一轮对话的实时更新，与它保存后回放的历史：核心流式发出的是一段段增量与工具调用的状态，
// 回放的是定稿的整块。
var (
	call = []byte(`{"path":"a.go"}`)
	live = []protocol.Update{
		protocol.RunStateUpdate{Running: true},
		protocol.UserMessageChunk{MessageID: "m1", Content: protocol.Text("看看 a.go"), Seq: 2},
		protocol.AgentMessageChunk{MessageID: "m2", Content: protocol.Text("好")},
		protocol.AgentMessageChunk{MessageID: "m2", Content: protocol.Text("的。")},
		protocol.ToolCall{ToolCallID: "c1", Name: "read", RawInput: []byte(`{"pa`),
			Status: protocol.ToolPending, MessageID: "m2"},
		protocol.ToolCallUpdate{ToolCallID: "c1", RawInput: call},
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolInProgress},
		protocol.ToolCallUpdate{ToolCallID: "c1", Status: protocol.ToolCompleted,
			Content: []protocol.ToolCallContent{{Content: protocol.Text("package a")}}},
		protocol.RunStateUpdate{StopReason: protocol.StopEndTurn},
	}
	replayed = []protocol.Update{
		protocol.UserMessageChunk{MessageID: "h2", Content: protocol.Text("看看 a.go"), Seq: 2},
		protocol.AgentMessageChunk{MessageID: "h3", Content: protocol.Text("好的。")},
		protocol.ToolCall{ToolCallID: "c1", Name: "read", RawInput: call,
			Status: protocol.ToolPending, MessageID: "h3"},
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
