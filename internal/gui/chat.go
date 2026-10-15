package gui

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/host"
)

// itemKind 区分对话中一块内容的呈现方式。
type itemKind int

const (
	itemUser itemKind = iota
	itemAssistant
	itemThinking
	itemTool
	itemNotice
)

// item 是对话中的一块可显示内容，只在主线程读写。
type item struct {
	id   int
	kind itemKind
	text string // 用户、助手、思考的文本，或系统通知
	done bool   // 文本已定稿；助手文本定稿后改用完整 Markdown 渲染
	open bool   // 思考与工具卡片是否展开

	started time.Time // 思考：开始流出的时刻与用时，历史记录中为零
	took    time.Duration

	callID  string // 工具卡片：调用 ID、名称、参数与结果
	tool    string
	args    string
	result  string
	failed  bool
	running bool
}

// block 是对话中连续展示的一组显示项：连续的工具调用合并为一组。
type block struct{ items []*item }

// chat 是一个会话的界面状态；每个会话各有一份，可以同时运行。
type chat struct {
	key     string        // 本地稳定标识，会话创建前后保持不变
	id      string        // 持久化会话 ID，空表示尚未创建
	cwd     string        // 会话工作目录
	session *host.Session // 空表示尚未创建或需要重新恢复
	items   []*item
	live    []*item // 当前助手消息已流出的内容块，定稿时对账
	next    int
	list    ui.ListState
	input   string
	running bool
	loading bool
	err     string

	// 界面上选定的模型与思考深度，发送时应用到会话。
	model    string
	thinking agent.ThinkingLevel
	usage    int64 // 最近一次响应结束时的上下文词元数

	// 会话的终端面板：各个标签、当前标签、是否显示，以及创建失败的原因。
	terms      []*shell
	termTab    int
	termOpen   bool
	termReveal int // 需要滚进可见范围的标签
	termErr    string
}

// add 追加一个已定稿的显示项。
func (ch *chat) add(kind itemKind, text string) *item {
	ch.next++
	it := &item{id: ch.next, kind: kind, text: text, done: true}
	ch.items = append(ch.items, it)
	return it
}

// tool 返回指定调用对应的工具卡片，没有时创建。
func (ch *chat) tool(callID string) *item {
	for _, it := range slices.Backward(ch.items) {
		if it.kind == itemTool && it.callID == callID {
			return it
		}
	}
	it := ch.add(itemTool, "")
	it.callID, it.done = callID, false
	return it
}

// stream 返回正在流出的同类内容块，没有时开始一个新块。
func (ch *chat) stream(kind itemKind) *item {
	if n := len(ch.live); n > 0 && ch.live[n-1].kind == kind && !ch.live[n-1].done {
		return ch.live[n-1]
	}
	it := ch.add(kind, "")
	it.done, it.started = false, time.Now()
	ch.live = append(ch.live, it)
	return it
}

// blocks 把显示项分组：连续的工具调用成为一组，其余各自成组。
func (ch *chat) blocks() []block {
	var out []block
	for _, it := range ch.items {
		if n := len(out); n > 0 && it.kind == itemTool && out[n-1].items[0].kind == itemTool {
			out[n-1].items = append(out[n-1].items, it)
			continue
		}
		out = append(out, block{items: []*item{it}})
	}
	return out
}

// load 用持久化记录重建对话，提示词快照不显示。
func (ch *chat) load(records []host.Record) {
	ch.items, ch.live = nil, nil
	for _, record := range records {
		ch.append(record.Message)
	}
}

// append 把一条定稿消息追加为显示项。
func (ch *chat) append(message agent.Message) {
	switch m := message.(type) {
	case agent.SystemMessage:
		if m.Type == agent.SystemMessageTypeNotice {
			ch.add(itemNotice, m.Content)
		}
	case agent.UserMessage:
		ch.add(itemUser, textOf(m.Content))
	case agent.AssistantMessage:
		ch.track(m)
		for _, block := range m.Content {
			switch b := block.(type) {
			case agent.ThinkingContent:
				if b.Thinking != "" {
					ch.add(itemThinking, b.Thinking)
				}
			case agent.TextContent:
				ch.add(itemAssistant, b.Text)
			case agent.ToolCall:
				ch.setCall(b)
			}
		}
	case agent.ToolResultMessage:
		it := ch.tool(m.ToolCallID)
		it.tool, it.result, it.failed = m.ToolName, textOf(m.Content), m.IsError
		it.running, it.done = false, true
	}
}

// track 记录一次响应结束时的上下文大小：该响应的输入与输出之和就是下一次请求的起点。
func (ch *chat) track(m agent.AssistantMessage) {
	if n := m.Usage.TotalTokens; n > 0 {
		ch.usage = n
	}
}

// setCall 记录工具调用的名称与参数；流式期间参数是累积快照。
func (ch *chat) setCall(call agent.ToolCall) {
	it := ch.tool(call.ID)
	it.tool, it.args = call.Name, string(call.Arguments)
}

// apply 应用运行事件。增量只做临时展示，定稿消息到达时以其为准。
func (ch *chat) apply(e agent.Event) {
	switch e.Type {
	case agent.EventTypeMessageStart:
		ch.live = nil
	case agent.EventTypeMessageUpdate:
		switch e.Delta.Type {
		case agent.DeltaTypeText:
			ch.stream(itemAssistant).text += e.Delta.Text
		case agent.DeltaTypeThinking:
			ch.stream(itemThinking).text += e.Delta.Text
		case agent.DeltaTypeToolCall:
			ch.setCall(e.Delta.ToolCall)
		}
	case agent.EventTypeMessageEnd:
		if m, ok := e.Message.(agent.AssistantMessage); ok {
			ch.finish(m)
		} else {
			ch.append(e.Message)
		}
	case agent.EventTypeToolExecutionStart:
		it := ch.tool(e.ToolCallID)
		it.tool, it.running = e.ToolName, true
	case agent.EventTypeToolExecutionEnd:
		it := ch.tool(e.ToolCallID)
		it.result, it.failed = textOf(e.Result.Content), e.Result.IsError
		it.running, it.done = false, true
	}
}

// finish 用定稿的助手消息校正已流出的内容块，保持原有显示项以延续渲染状态。
func (ch *chat) finish(m agent.AssistantMessage) {
	ch.track(m)
	pending := ch.live
	ch.live = nil
	take := func(kind itemKind) *item {
		if i := slices.IndexFunc(pending, func(it *item) bool { return it.kind == kind }); i >= 0 {
			it := pending[i]
			pending = slices.Delete(pending, i, i+1)
			return it
		}
		return ch.add(kind, "")
	}
	for _, block := range m.Content {
		switch b := block.(type) {
		case agent.ThinkingContent:
			if b.Thinking != "" {
				it := take(itemThinking)
				it.text, it.done = b.Thinking, true
				if !it.started.IsZero() {
					it.took = time.Since(it.started)
				}
			}
		case agent.TextContent:
			it := take(itemAssistant)
			it.text, it.done = b.Text, true
		case agent.ToolCall:
			ch.setCall(b)
		}
	}
	for _, it := range pending {
		it.done = true
	}
}

// textOf 连接内容中的文本；图片以占位文字表示。
func textOf(content []agent.Content) string {
	var parts []string
	for _, block := range content {
		switch b := block.(type) {
		case agent.TextContent:
			parts = append(parts, b.Text)
		case agent.ImageContent:
			parts = append(parts, "[图片]")
		}
	}
	return strings.Join(parts, "\n")
}
