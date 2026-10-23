package core

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/skills"
	"github.com/gvenusleo/atlas/internal/store"
)

// translator 把一轮里的事件转成客户端更新：流式的增量直接转发，定稿的消息里没流出来的部分补上，
// 其余定稿的消息与回放历史用同一个转换（updatesOf）。
type translator struct {
	s             *session
	msg           string    // 正在回复的助手消息的 ID
	at            time.Time // 它开始的时刻
	text, thought bool      // 它流出过文字、思考
	calls         map[string]bool
	compaction    string // 进行中的压缩的 ID
}

// event 转换一个事件；seq 是定稿消息保存后的序号。
func (t *translator) event(e agent.Event, seq int64) []protocol.Update {
	switch e.Type {
	case agent.EventMessageStart:
		t.msg, t.at = t.s.newID(), time.Now()
		t.text, t.thought, t.calls = false, false, map[string]bool{}
	case agent.EventDelta:
		return t.delta(e.Delta)
	case agent.EventRetry:
		t.text, t.thought, t.calls = false, false, map[string]bool{}
		return []protocol.Update{protocol.RequestRetryUpdate{
			MessageID: t.msg, Attempt: e.Retry.Attempt, Max: e.Retry.Max,
			Delay: e.Retry.Delay, Error: e.Retry.Error,
		}}
	case agent.EventToolStart:
		return []protocol.Update{protocol.ToolCallUpdate{
			ToolCallID: e.Call.ID, Status: protocol.ToolInProgress,
		}}
	case agent.EventCompactionStart:
		t.compaction = t.s.newID()
		return []protocol.Update{protocol.CompactionUpdate{
			CompactionID: t.compaction, Status: protocol.CompactionInProgress,
		}}
	case agent.EventMessageEnd:
		return t.end(e.Message, seq)
	}
	return nil
}

// delta 转发一个增量。
func (t *translator) delta(d agent.Delta) []protocol.Update {
	switch d.Type {
	case agent.DeltaTypeText:
		t.text = true
		return []protocol.Update{protocol.AgentMessageChunk{
			MessageID: t.msg, Content: protocol.Text(d.Text), At: t.at}}
	case agent.DeltaTypeThinking:
		t.thought = true
		return []protocol.Update{protocol.AgentThoughtChunk{
			MessageID: t.msg, Content: protocol.Text(d.Text)}}
	case agent.DeltaTypeToolCall:
		return []protocol.Update{t.call(d.ToolCall)}
	}
	return nil
}

// call 第一次见到一个调用时发出 tool_call，之后只更新它的参数。
func (t *translator) call(call agent.ToolCall) protocol.Update {
	if t.calls[call.ID] {
		return protocol.ToolCallUpdate{ToolCallID: call.ID, RawInput: call.Arguments}
	}
	t.calls[call.ID] = true
	return toolCall(call, t.msg)
}

// end 转换一条定稿的消息。
func (t *translator) end(m agent.Message, seq int64) []protocol.Update {
	switch m := m.(type) {
	case agent.AssistantMessage:
		return assistantUpdates(m, t.msg, t.text, t.thought, t.call)
	case agent.SystemMessage:
		if t.compaction == "" {
			break
		}
		update := protocol.CompactionUpdate{CompactionID: t.compaction}
		t.compaction = ""
		if m.Type == agent.SystemMessageTypeCompaction {
			update.Status = protocol.CompactionCompleted
			update.Summary = m.Content
		} else {
			update.Status, update.Error = protocol.CompactionFailed, m.Content
		}
		return []protocol.Update{update}
	}
	return updatesOf(m, seq, t.s.newID())
}

// finish 在一轮结束时收尾：还在进行的压缩是被取消了。
func (t *translator) finish() []protocol.Update {
	if t.compaction == "" {
		return nil
	}
	return []protocol.Update{protocol.CompactionUpdate{
		CompactionID: t.compaction, Status: protocol.CompactionCancelled,
	}}
}

// historyUpdates 把一段历史记录转成更新，供分页读取。
func (c *Core) historyUpdates(
	ctx context.Context, id string, records []store.Record,
) ([]protocol.Update, error) {
	var out []protocol.Update
	for _, r := range records {
		m, err := c.decodeWithImages(ctx, id, r)
		if err != nil {
			return nil, err
		}
		out = append(out, updatesOf(m, r.Seq, "h"+strconv.FormatInt(r.Seq, 10))...)
	}
	return out, nil
}

// updatesOf 把一条定稿的消息转成更新；seq 是它的序号，id 是它的消息 ID。
func updatesOf(m agent.Message, seq int64, id string) []protocol.Update {
	var out []protocol.Update
	switch m := m.(type) {
	case agent.UserMessage:
		for _, block := range m.Content {
			chunk := protocol.UserMessageChunk{
				MessageID: id, Content: contentBlock(block), Seq: seq, At: m.Timestamp,
			}
			if name, body, ok := skills.ParseInvocationBlock(chunk.Content.Text); ok {
				chunk.Skill, chunk.Content = name, protocol.Text(body)
			}
			out = append(out, chunk)
		}
	case agent.AssistantMessage:
		call := func(c agent.ToolCall) protocol.Update { return toolCall(c, id) }
		out = assistantUpdates(m, id, false, false, call)
	case agent.ToolResultMessage:
		update := protocol.ToolCallUpdate{ToolCallID: m.ToolCallID,
			Status: protocol.ToolCompleted, RawOutput: m.Details}
		if m.IsError {
			update.Status = protocol.ToolFailed
		}
		for _, block := range m.Content {
			update.Content = append(update.Content,
				protocol.ToolCallContent{Type: "content", Content: contentBlock(block)})
		}
		out = append(out, update)
	case agent.SystemMessage:
		switch m.Type {
		case agent.SystemMessageTypeCompaction:
			out = append(out, protocol.CompactionUpdate{CompactionID: id,
				Status: protocol.CompactionCompleted, Summary: m.Content})
		case agent.SystemMessageTypeNotice:
			out = append(out, protocol.Notice{Severity: "info", Title: m.Content})
		}
	}
	return out
}

// assistantUpdates 把一条定稿的助手消息转成更新：思考、文字与工具调用按原来的顺序，紧挨着的
// 两块文字之间补空行，与流式时一致（见 llm）。text、thought 为真表示这类内容已经流出过，不再发；
// call 转换工具调用。
func assistantUpdates(
	m agent.AssistantMessage, id string, text, thought bool,
	call func(agent.ToolCall) protocol.Update,
) []protocol.Update {
	var out []protocol.Update
	afterText := false
	for _, block := range m.Content {
		switch b := block.(type) {
		case agent.ThinkingContent:
			if b.Thinking == "" {
				continue // 空的思考不显示，也不隔开前后的文字
			}
			if !thought {
				out = append(out, protocol.AgentThoughtChunk{
					MessageID: id, Content: protocol.Text(b.Thinking)})
			}
		case agent.TextContent:
			if !text {
				s := b.Text
				if afterText {
					s = "\n\n" + s
				}
				out = append(out, protocol.AgentMessageChunk{MessageID: id,
					Content: protocol.Text(s), At: m.Timestamp})
			}
		case agent.ToolCall:
			out = append(out, call(b))
		}
		_, afterText = block.(agent.TextContent)
	}
	return out
}

// toolCall 是一个刚发出的工具调用。
func toolCall(call agent.ToolCall, msg string) protocol.ToolCall {
	kind := protocol.ToolOther
	switch call.Name {
	case "read":
		kind = protocol.ToolRead
	case "write", "edit":
		kind = protocol.ToolEdit
	case "shell":
		kind = protocol.ToolExecute
	}
	return protocol.ToolCall{ToolCallID: call.ID, Name: call.Name, Title: call.Name, Kind: kind,
		Status: protocol.ToolPending, RawInput: call.Arguments, MessageID: msg}
}

// contentBlock 把文字或图片内容转成内容块。
func contentBlock(c agent.Content) protocol.ContentBlock {
	if img, ok := c.(agent.ImageContent); ok {
		return protocol.Image(img.Data, img.MIMEType)
	}
	return protocol.Text(agent.TextOf([]agent.Content{c}))
}

// userMessageSummary 是会话大纲里的一条用户消息：输入的文字，不含图片与技能全文。
func userMessageSummary(seq int64, m agent.UserMessage) protocol.UserMessageSummary {
	summary := protocol.UserMessageSummary{Seq: seq, At: m.Timestamp}
	var texts []string
	for _, block := range m.Content {
		switch b := block.(type) {
		case agent.TextContent:
			if _, _, ok := skills.ParseInvocationBlock(b.Text); !ok {
				texts = append(texts, b.Text)
			}
		case agent.ImageContent:
			summary.Images = true
		}
	}
	summary.Text = strings.Join(texts, "\n")
	return summary
}
