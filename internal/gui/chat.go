package gui

import (
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// itemKind 区分对话中一块内容的呈现方式。
type itemKind int

const (
	itemUser itemKind = iota
	itemAssistant
	itemThinking
	itemTool
	itemNotice
	itemCompaction // 上下文压缩：text 是摘要，没完成时是进行中的占位
	itemSkill      // 显式调用的技能：skill 是技能名，text 是附给模型的技能块内容
)

// item 是对话中的一块可显示内容，只在主线程读写。
type item struct {
	id    int
	kind  itemKind
	msg   string // 所属消息的 ID；重试时这条消息的内容一起丢掉
	text  string // 用户、助手、思考的文本，或系统通知
	done  bool   // 文本已定稿；助手文本定稿后改用完整 Markdown 渲染
	open  bool   // 思考、工具与技能卡片是否展开
	skill string // 技能的名称

	at       time.Time // 用户与助手消息的时间，未知时为零
	copiedAt time.Time // 最近一次点复制的时刻，用来短暂显示“已复制”
	seq      int64     // 用户消息的记录序号

	callID   string // 工具卡片：调用 ID、名称、参数与结果
	shown    bool   // 一组步骤是否展开，记在组的第一项上
	call     *callView
	tool     string
	args     string
	result   string
	details  jsontext.Value // 工具随结果附带的结构化数据
	pictures []*picture     // 工具结果或用户消息里的图片
	failed   bool
}

// block 是对话中连续展示的一组显示项：一条用户消息，一次 run 的全部输出，或一个系统事件。
type block struct{ items []*item }

// chat 是一个会话的界面状态；每个会话各有一份，可以同时运行。
//
// 会话的视图（对话、配置项、命令、占用、标题、是否在运行）只由核心的响应与更新写入，
// 界面从不自己决定或推算；其余是界面自己的状态。
type chat struct {
	key string // 本地稳定标识，会话建好前后保持不变
	id  string // 会话 ID；空表示新会话还没建好
	cwd string // 会话的工作目录，建会话时定下

	// 会话的视图。
	ready     bool // 核心已经打开了这个会话
	title     string
	options   []protocol.ConfigOption
	commands  []protocol.Command
	usage     protocol.Usage
	running   bool
	since     time.Time // 这一轮开始的时刻，运行中输入框上方的计时从它算起
	items     []*item
	streaming *item  // 正在流出的文字或思考
	retry     string // 正在重试时的提示，下一个增量或这一轮结束时清掉
	next      int

	// 界面自己的状态。
	oldest  int64 // 已加载的最早一条记录的 Seq；大于 1 表示前面还有更早的记录
	loading bool  // 正在加载最近一页历史
	paging  bool  // 正在加载更早的记录
	// seekSeq 是目录要跳到的提问的记录序号：它还没加载时，一路加载到它为止，到了就滚过去；
	// 0 表示没有。
	seekSeq int64
	list    ui.ListState
	outline outline // 会话目录
	input   string
	// 输入框里待发送的图片，与 input 一样是草稿；preparing 是还在后台整理的张数。
	attachments []*attachment
	preparing   int
	err         string
	unread      bool // 在别的会话时运行结束，用户还没回来看过

	menu  menuState  // 输入框上方补全列表的状态
	index pathIndex  // 工作目录下的路径，@ 补全与文件面板的筛选共用
	files *filePanel // 文件面板，第一次打开时才建

	// 会话的终端面板：各个标签、当前标签、是否显示，以及创建失败的原因。
	terms      []*shell
	termTab    int
	termOpen   bool
	termReveal int // 需要滚进可见范围的标签
	termErr    string

	expanded sidePanel // 扩展到整个主区的面板；没有时为 noPanel
}

// fresh 报告会话是不是还没发过消息的新会话：它还没写进库，换工作目录就是换一个新会话。
func (ch *chat) fresh() bool {
	return !ch.loading && ch.oldest == 0 && len(ch.items) == 0 && !ch.running
}

// canSend 报告现在能不能发送：会话建好了，没在运行，历史与图片都准备好了。
func (ch *chat) canSend() bool {
	return ch.id != "" && !ch.running && !ch.loading && ch.preparing == 0
}

// attach 记下核心打开会话时给的配置项与状态。
func (ch *chat) attach(options []protocol.ConfigOption, state protocol.SessionState) {
	ch.ready, ch.options, ch.commands = true, options, state.Commands
	ch.usage, ch.running, ch.since = state.Usage, state.Running, state.Since
	ch.cwd, ch.title = state.Cwd, state.Title
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
	ch.endStream()
	it := ch.add(itemTool, "")
	it.callID, it.done = callID, false
	return it
}

// stream 返回 msg 正在流出的同类内容块，没有时开始一个新块。
func (ch *chat) stream(kind itemKind, msg string) *item {
	if it := ch.streaming; it != nil && it.kind == kind && it.msg == msg {
		return it
	}
	ch.endStream()
	it := ch.add(kind, "")
	it.msg, it.done = msg, false
	ch.streaming = it
	return it
}

// endStream 结束正在流出的内容块：下一块、工具调用或别的消息开始时，它就已经写完。
func (ch *chat) endStream() {
	if ch.streaming != nil {
		ch.streaming.done = true
		ch.streaming = nil
	}
}

// settle 在一轮结束时收尾：流出的内容块都已写完；还没有结果的工具调用不会再有结果，
// 标成失败，免得一直停在进行中。
func (ch *chat) settle(reason string) {
	ch.endStream()
	for _, it := range ch.items {
		if it.kind == itemTool && !it.done {
			it.finishCall([]protocol.ToolCallContent{{Content: protocol.Text(reason)}}, nil, true)
		}
	}
}

// isStep 报告显示项是否是过程中的一步：思考或工具调用。
func isStep(k itemKind) bool { return k == itemThinking || k == itemTool }

// isSystem 报告显示项是否是系统事件：上下文压缩或通知。
func isSystem(k itemKind) bool { return k == itemCompaction || k == itemNotice }

// blocks 把显示项分组：用户消息自成一组，其后到下一条用户消息之前，是一次 run 的全部输出；
// 一次 run 开头与末尾的系统事件不属于它的输出，也各自成组。
func (ch *chat) blocks() []block {
	var runs [][]*item
	for _, it := range ch.items {
		if n := len(runs); n > 0 && it.kind != itemUser && runs[n-1][0].kind != itemUser {
			runs[n-1] = append(runs[n-1], it)
			continue
		}
		runs = append(runs, []*item{it})
	}
	var out []block
	for _, run := range runs {
		for _, items := range peel(run) {
			out = append(out, block{items: items})
		}
	}
	return out
}

// peel 把一次 run 的输出拆开：开头与末尾的系统事件各自成组，夹在输出中间的留在里面。
func peel(items []*item) [][]*item {
	var out [][]*item
	for len(items) > 0 && isSystem(items[0].kind) {
		out, items = append(out, items[:1]), items[1:]
	}
	end := len(items)
	for end > 0 && isSystem(items[end-1].kind) {
		end--
	}
	if end > 0 {
		out = append(out, items[:end])
	}
	for _, it := range items[end:] {
		out = append(out, []*item{it})
	}
	return out
}

// parts 把一次 run 的输出分段：连续的思考与工具调用成为一组，其余各自成段。
func parts(items []*item) [][]*item {
	var out [][]*item
	for _, it := range items {
		if n := len(out); n > 0 && isStep(it.kind) && isStep(out[n-1][0].kind) {
			out[n-1] = append(out[n-1], it)
			continue
		}
		out = append(out, []*item{it})
	}
	return out
}

// replyText 是一次 run 里所有助手文字，段与段之间空一行，at 是最后一段的时间；
// 思考、工具与通知不算。
func replyText(items []*item) (text string, at time.Time) {
	var texts []string
	for _, it := range items {
		if it.kind == itemAssistant && it.text != "" {
			texts, at = append(texts, it.text), it.at
		}
	}
	return strings.Join(texts, "\n\n"), at
}

// load 用一页历史重建对话；oldest 是这一页最早一条的记录序号。
func (ch *chat) load(updates []protocol.SessionUpdate, oldest int64) {
	ch.items, ch.streaming = nil, nil
	for _, u := range updates {
		ch.apply(u)
	}
	ch.endStream()
	ch.oldest = oldest
}

// prepend 把更早的一页历史排到已有显示项之前。页的边界可能把一次工具调用与它的结果拆开：
// 结果已在后一页里时，调用先在这页里建好，把结果并过来，不重复显示。
func (ch *chat) prepend(updates []protocol.SessionUpdate, oldest int64) {
	older := &chat{next: ch.next}
	older.load(updates, oldest)
	for _, it := range older.items {
		if it.kind != itemTool || it.done {
			continue
		}
		i := slices.IndexFunc(ch.items, func(o *item) bool {
			return o.kind == itemTool && o.callID == it.callID
		})
		if i >= 0 {
			o := ch.items[i]
			it.result, it.details, it.failed, it.done = o.result, o.details, o.failed, true
			it.pictures = o.pictures
			ch.items = slices.Delete(ch.items, i, i+1)
		}
	}
	ch.items = slices.Concat(older.items, ch.items)
	ch.next, ch.oldest = older.next, older.oldest
}

// apply 应用核心推来的一个更新。打开会话时的历史与运行中的实时更新走的都是这里。
func (ch *chat) apply(u protocol.SessionUpdate) {
	switch u := u.(type) {
	case protocol.UserMessageChunk:
		ch.userChunk(u)
	case protocol.AgentMessageChunk:
		ch.retry = ""
		it := ch.stream(itemAssistant, u.MessageID)
		it.text += u.Content.Text
		it.at = u.Meta.At
	case protocol.AgentThoughtChunk:
		ch.retry = ""
		ch.stream(itemThinking, u.MessageID).text += u.Content.Text
	case protocol.ToolCall:
		it := ch.tool(u.ToolCallID)
		it.msg, it.tool, it.args = u.Meta.MessageID, u.Name, string(u.RawInput)
	case protocol.ToolCallUpdate:
		ch.toolUpdate(u)
	case protocol.CompactionUpdate:
		ch.compaction(u)
	case protocol.Notice:
		ch.endStream()
		ch.add(itemNotice, u.Title)
	case protocol.UsageUpdate:
		ch.usage = u.Usage
	case protocol.SessionInfoUpdate:
		ch.title = u.Title
	case protocol.ConfigOptionUpdate:
		ch.options = u.ConfigOptions
	case protocol.StateUpdate:
		ch.running, ch.since = u.Running, u.Since
		if !u.Running {
			ch.retry = ""
			ch.settle("已中断")
		}
	case protocol.RetryUpdate:
		// 这条回复作废：丢掉它已流出的文字、思考与半截的工具调用，等重新请求。
		ch.endStream()
		ch.items = slices.DeleteFunc(ch.items, func(it *item) bool {
			return it.msg == u.MessageID && it.kind != itemUser
		})
		ch.retry = retryNotice(u)
	}
}

// userChunk 把用户消息的一块排进对话：同一条消息的文字与图片在一项里，技能全文另成一项。
func (ch *chat) userChunk(u protocol.UserMessageChunk) {
	ch.endStream()
	if u.Meta.Skill != "" {
		it := ch.add(itemSkill, u.Content.Text)
		it.msg, it.skill = u.MessageID, u.Meta.Skill
		return
	}
	var it *item
	if n := len(ch.items); n > 0 && ch.items[n-1].kind == itemUser &&
		ch.items[n-1].msg == u.MessageID {
		it = ch.items[n-1]
	} else {
		it = ch.add(itemUser, "")
		it.msg, it.seq, it.at = u.MessageID, u.Meta.Seq, u.Meta.At
	}
	switch u.Content.Type {
	case "image":
		it.pictures = append(it.pictures, &picture{data: u.Content, edge: userPictureEdge})
	default:
		if it.text != "" {
			it.text += "\n"
		}
		it.text += u.Content.Text
	}
}

// toolUpdate 更新工具卡片：流出的参数、开始执行，或者结果。
func (ch *chat) toolUpdate(u protocol.ToolCallUpdate) {
	it := ch.tool(u.ToolCallID)
	if u.RawInput != nil {
		it.args = string(u.RawInput)
	}
	switch u.Status {
	case protocol.ToolCompleted, protocol.ToolFailed:
		it.finishCall(u.Content, u.RawOutput, u.Status == protocol.ToolFailed)
	}
}

// compaction 画压缩的进展：进行中是一行占位，结束时由摘要取代，失败时由通知取代，
// 取消时消失。
func (ch *chat) compaction(u protocol.CompactionUpdate) {
	ch.endStream()
	ch.items = slices.DeleteFunc(ch.items, func(it *item) bool {
		return it.kind == itemCompaction && !it.done
	})
	switch u.Status {
	case protocol.CompactionInProgress:
		ch.add(itemCompaction, "").done = false
	case protocol.CompactionCompleted:
		ch.add(itemCompaction, u.Summary)
	case protocol.CompactionFailed:
		ch.add(itemNotice, u.Error)
	}
}

// hasDraft 报告输入框里有没有可发送的内容：文字或图片。
func (ch *chat) hasDraft() bool {
	return strings.TrimSpace(ch.input) != "" || len(ch.attachments) > 0
}

// finishCall 记录工具调用的结果；展示数据依赖结果，所以丢掉按进行中的状态算出的。
func (it *item) finishCall(
	content []protocol.ToolCallContent, details jsontext.Value, failed bool,
) {
	var texts []string
	it.pictures = nil
	for _, c := range content {
		if c.Content.Type == "image" {
			it.pictures = append(it.pictures, &picture{data: c.Content, edge: pictureEdge})
			texts = append(texts, "[图片]")
		} else {
			texts = append(texts, c.Content.Text)
		}
	}
	it.result, it.details, it.failed = strings.Join(texts, "\n"), details, failed
	it.done = true
	it.call = nil
}

// retryNotice 是重试时给用户的一行提示：等多久、第几次、为什么。
func retryNotice(r protocol.RetryUpdate) string {
	reason, _, _ := strings.Cut(r.Error, "\n")
	return fmt.Sprintf("请求失败，%d 秒后重试（第 %d/%d 次）：%s",
		int(r.Delay.Seconds()), r.Attempt, r.Max, truncate(reason, 80))
}

// option 返回会话的一个配置项。
func (ch *chat) option(id string) (protocol.ConfigOption, bool) {
	i := slices.IndexFunc(ch.options, func(o protocol.ConfigOption) bool { return o.ID == id })
	if i < 0 {
		return protocol.ConfigOption{}, false
	}
	return ch.options[i], true
}

// acceptsImages 报告会话选定的模型是否接受图片输入。
func (ch *chat) acceptsImages() bool {
	model, _ := ch.option(protocol.ConfigModel)
	i := slices.IndexFunc(model.Options, func(v protocol.ConfigValue) bool {
		return v.Value == model.CurrentValue
	})
	return i >= 0 && model.Options[i].Meta.Images
}
