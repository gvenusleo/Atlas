package gui

import (
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/host"
	"github.com/gvenusleo/atlas/internal/skills"
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
	text  string // 用户、助手、思考的文本，或系统通知
	done  bool   // 文本已定稿；助手文本定稿后改用完整 Markdown 渲染
	open  bool   // 思考、工具与技能卡片是否展开
	skill string // 技能的名称

	at       time.Time // 用户与助手消息的时间，未知时为零
	copiedAt time.Time // 最近一次点复制的时刻，用来短暂显示“已复制”
	seq      int64     // 用户消息的记录序号；这次打开后新发的消息没有，为 0

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
type chat struct {
	key     string        // 本地稳定标识，会话创建前后保持不变
	id      string        // 持久化会话 ID，空表示尚未创建
	cwd     string        // 会话工作目录
	session *host.Session // 空表示尚未创建或需要重新恢复
	items   []*item
	live    []*item // 当前助手消息已流出的内容块，定稿时对账
	tail    *item   // 当前消息开始流出时的最后一个显示项；重试时丢弃它之后的半截内容
	retry   string  // 正在重试时的提示，下一个增量或这条消息结束时清掉
	next    int
	oldest  int64 // 已加载的最早一条记录的 Seq；大于 1 表示前面还有更早的记录
	paging  bool  // 正在加载更早的记录
	// seekSeq 是目录要跳到的提问的记录序号：它还没加载时，一路加载到它为止，到了就滚过去；
	// 0 表示没有。
	seekSeq int64
	list    ui.ListState
	outline outline // 会话目录
	input   string
	running bool
	// runStart 是这次运行开始的时刻，运行中输入框上方的计时从它算起。
	runStart time.Time
	// 输入框里待发送的图片，与 input 一样是草稿；preparing 是还在后台整理的张数。
	attachments []*attachment
	preparing   int
	loading     bool
	err         string
	unread      bool // 在别的会话时运行结束，用户还没回来看过

	// 还没有会话时，后台发现的工作目录下的技能，供命令列表；skillsCwd 是它对应的工作目录。
	skills    []skills.Skill
	skillsCwd string
	menu      menuState  // 输入框上方补全列表的状态
	index     pathIndex  // 工作目录下的路径，@ 补全与文件面板的筛选共用
	files     *filePanel // 文件面板，第一次打开时才建

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

	expanded sidePanel // 扩展到整个主区的面板；没有时为 noPanel
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
	ch.endBlock()
	it := ch.add(kind, "")
	it.done = false
	ch.live = append(ch.live, it)
	return it
}

// endBlock 结束正在流出的内容块：下一块或工具调用开始流出时，它就已经写完。
func (ch *chat) endBlock() {
	if n := len(ch.live); n > 0 {
		ch.live[n-1].done = true
	}
}

// settle 在一次 run 结束时收尾：流出的内容块都已写完；还没有结果的工具调用不会再有结果，
// 标成失败，免得一直停在进行中。
func (ch *chat) settle(reason string) {
	for _, it := range ch.live {
		it.done = true
	}
	ch.live = nil
	for _, it := range ch.items {
		if it.kind == itemTool && !it.done {
			it.finishCall([]agent.Content{agent.TextContent{Text: reason}}, nil, true)
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

// load 用持久化记录重建对话，提示词快照不显示。
func (ch *chat) load(records []host.Record) {
	ch.items, ch.live, ch.oldest = nil, nil, 0
	for _, record := range records {
		ch.append(record.Message, record.Seq)
	}
	if len(records) > 0 {
		ch.oldest = records[0].Seq
	}
}

// prepend 把更早的一页记录排到已有显示项之前。页的边界可能把一次工具调用与它的结果拆开：
// 结果已在后一页里时，调用先在这页里建好，把结果并过来，不重复显示。
func (ch *chat) prepend(records []host.Record) {
	older := &chat{next: ch.next}
	older.load(records)
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

// append 把一条定稿消息追加为显示项；seq 是它的记录序号，运行中新来的消息还没有，为 0。
func (ch *chat) append(message agent.Message, seq int64) {
	switch m := message.(type) {
	case agent.SystemMessage:
		ch.dropCompacting() // 压缩的结果（摘要或失败的通知）取代进行中的占位
		switch m.Type {
		case agent.SystemMessageTypeNotice:
			ch.add(itemNotice, m.Content)
		case agent.SystemMessageTypeCompaction:
			ch.add(itemCompaction, m.Content)
			ch.usage = 0 // 上下文变小了，下一次响应才有新的数字
		}
	case agent.UserMessage:
		ch.addUser(m).seq = seq
	case agent.AssistantMessage:
		ch.track(m)
		for _, block := range m.Content {
			switch b := block.(type) {
			case agent.ThinkingContent:
				if b.Thinking != "" {
					ch.add(itemThinking, b.Thinking)
				}
			case agent.TextContent:
				ch.add(itemAssistant, b.Text).at = m.Timestamp
			case agent.ToolCall:
				ch.setCall(b)
			}
		}
	case agent.ToolResultMessage:
		it := ch.tool(m.ToolCallID)
		it.tool = m.ToolName
		it.finishCall(m.Content, m.Details, m.IsError)
	}
}

// hasDraft 报告输入框里有没有可发送的内容：文字或图片。
func (ch *chat) hasDraft() bool {
	return strings.TrimSpace(ch.input) != "" || len(ch.attachments) > 0
}

// addUser 把用户消息排成显示项：显式调用技能时，技能块另成一项，排在输入的原文之后。
func (ch *chat) addUser(m agent.UserMessage) *item {
	typed, skill, content := splitUser(m)
	it := ch.add(itemUser, agent.TextOf(typed))
	it.at, it.pictures = m.Timestamp, picturesOf(m.Content, userPictureEdge)
	if skill != "" {
		ch.add(itemSkill, content).skill = skill
	}
	return it
}

// splitUser 把用户消息分成输入的文字，和显式调用的技能（名称与附给模型的内容）。
// 图片另外显示，不算在文字里。
func splitUser(m agent.UserMessage) (typed []agent.Content, skill, content string) {
	for _, c := range m.Content {
		switch b := c.(type) {
		case agent.TextContent:
			if name, body, ok := skills.ParseInvocationBlock(b.Text); ok {
				skill, content = name, body
				continue
			}
		case agent.ImageContent:
			continue
		}
		typed = append(typed, c)
	}
	return typed, skill, content
}

// skillList 返回会话的技能：会话已存在时取它的快照，与模型看到的技能目录同一份；
// 还没有会话时（新会话，或尚未恢复的旧会话）取后台发现的。
func (ch *chat) skillList() []skills.Skill {
	if ch.session != nil {
		return ch.session.State().Skills
	}
	return ch.skills
}

// finishCall 记录工具调用的结果；展示数据依赖结果，所以丢掉按进行中的状态算出的。
func (it *item) finishCall(content []agent.Content, details jsontext.Value, failed bool) {
	it.result, it.details, it.failed = agent.TextOf(content), details, failed
	it.pictures = picturesOf(content, pictureEdge)
	it.done = true
	it.call = nil
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
		ch.mark()
		system, ok := e.Message.(agent.SystemMessage)
		if ok && system.Type == agent.SystemMessageTypeCompaction {
			ch.add(itemCompaction, "").done = false
		}
	case agent.EventTypeRetry:
		// 这次响应作废：丢掉它已流出的文字与半截的工具调用，等重新请求。
		// 中途往前加载的更早记录排在 tail 之前，不受影响；tail 不在列表里就什么也不丢。
		if i := slices.Index(ch.items, ch.tail); ch.tail == nil || i >= 0 {
			ch.items = ch.items[:i+1]
		}
		ch.live = nil
		ch.retry = retryNotice(e.Retry)
	case agent.EventTypeMessageUpdate:
		ch.retry = ""
		switch e.Delta.Type {
		case agent.DeltaTypeText:
			ch.stream(itemAssistant).text += e.Delta.Text
		case agent.DeltaTypeThinking:
			ch.stream(itemThinking).text += e.Delta.Text
		case agent.DeltaTypeToolCall:
			ch.endBlock()
			ch.setCall(e.Delta.ToolCall)
		}
	case agent.EventTypeMessageEnd:
		ch.retry = ""
		if m, ok := e.Message.(agent.AssistantMessage); ok {
			ch.finish(m)
		} else {
			ch.append(e.Message, 0)
			ch.mark()
		}
	case agent.EventTypeToolExecutionStart:
		it := ch.tool(e.ToolCallID)
		it.tool = e.ToolName
	case agent.EventTypeToolExecutionEnd:
		ch.tool(e.ToolCallID).finishCall(e.Result.Content, e.Result.Details, e.Result.IsError)
	}
}

// mark 记下现在的最后一个显示项：之后这条消息被重试时，丢掉它之后流出的半截内容。
func (ch *chat) mark() {
	ch.tail = nil
	if n := len(ch.items); n > 0 {
		ch.tail = ch.items[n-1]
	}
}

// dropCompacting 去掉进行中的压缩占位行：压缩结束时它由结果取代。
func (ch *chat) dropCompacting() {
	ch.items = slices.DeleteFunc(ch.items, func(it *item) bool {
		return it.kind == itemCompaction && !it.done
	})
}

// retryNotice 是重试时给用户的一行提示：等多久、第几次、为什么。
func retryNotice(r agent.RetryInfo) string {
	reason, _, _ := strings.Cut(r.Error, "\n")
	return fmt.Sprintf("请求失败，%d 秒后重试（第 %d/%d 次）：%s",
		int(r.Delay.Seconds()), r.Attempt, r.Max, truncate(reason, 80))
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
			}
		case agent.TextContent:
			it := take(itemAssistant)
			it.text, it.done, it.at = b.Text, true, m.Timestamp
		case agent.ToolCall:
			ch.setCall(b)
		}
	}
	for _, it := range pending {
		it.done = true
	}
}
