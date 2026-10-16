// Package llm 把 agent.StreamFn 落到具体的 LLM API 上。
// 适配器命名 = 厂商_wireAPI（如 OpenAIResponses），wire API 不同即为不同适配器。
package llm

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/gvenusleo/atlas/internal/agent"
)

// pendingCall 保存单个工具调用的元信息与参数累积，仅用于实时快照。
type pendingCall struct {
	id, name string
	args     strings.Builder
}

// OpenAIResponsesStreamFn 返回基于 OpenAI Responses API 的 StreamFn；
// 失败按契约编码进返回消息：ctx 取消 → StopReasonCanceled，其余 → StopReasonError，
// 其中暂时性的（限流、服务端故障、连接中断）标出 ErrorKind 供循环重试；
// 请求构造类错误在发起 HTTP 之前返回。
func OpenAIResponsesStreamFn(client openai.Client) agent.StreamFn {
	return func(
		ctx context.Context, req agent.StreamRequest, emit func(agent.Delta),
	) agent.AssistantMessage {
		params, err := requestOf(req)
		if err != nil {
			return failure(req.Model, agent.StopReasonError, err.Error(), "")
		}

		stream := client.Responses.NewStreaming(ctx, params)
		defer func() { _ = stream.Close() }()

		final, detail, aborted := consume(stream, emit)
		if aborted {
			return failure(req.Model, agent.StopReasonError, detail.text, "")
		}
		if err := stream.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return failure(req.Model, agent.StopReasonCanceled, err.Error(), "")
			}
			return failure(req.Model, agent.StopReasonError, err.Error(), errorKind(err))
		}
		if final == nil {
			// 没有 error 事件就结束的流是被截断了，可以重试；有错误码的按错误码判断。
			if detail == (streamDetail{}) {
				return failure(req.Model, agent.StopReasonError,
					"流结束但未收到终态事件", agent.ErrorKindRetryable)
			}
			text := cmp.Or(detail.text, detail.code)
			return failure(req.Model, agent.StopReasonError, text, codeKind(detail.code))
		}
		return messageOf(*final, req.Model)
	}
}

// failure 构造不含内容的失败消息。
func failure(
	model string, stop agent.StopReason, detail string, kind agent.ErrorKind,
) agent.AssistantMessage {
	return agent.AssistantMessage{
		Model: model, StopReason: stop, ErrorMessage: detail, ErrorKind: kind,
	}
}

// streamDetail 是流里 error 事件的内容，或事件序列违反协议的说明（只有 text）。
type streamDetail struct{ code, text string }

// consume 读取事件流并发出增量；返回终态响应与 error 事件的细节，
// aborted 为真表示事件序列违反协议，detail.text 即原因。
func consume(
	stream *ssestream.Stream[responses.ResponseStreamEventUnion], emit func(agent.Delta),
) (final *responses.Response, detail streamDetail, aborted bool) {
	calls := map[string]*pendingCall{}
	for stream.Next() {
		ev := stream.Current()
		switch ev.Type { // wire 值的权威定义见 SDK 的 shared/constant 包。
		case "response.output_text.delta":
			emit(agent.Delta{Type: agent.DeltaTypeText, Text: ev.Delta})
		case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			emit(agent.Delta{Type: agent.DeltaTypeThinking, Text: ev.Delta})
		case "response.reasoning_summary_part.done":
			// 多段摘要之间补空行，供实时显示分段。
			emit(agent.Delta{Type: agent.DeltaTypeThinking, Text: "\n\n"})
		case "response.output_item.added":
			if fc, ok := ev.Item.AsAny().(responses.ResponseFunctionToolCall); ok {
				// item ID 关联参数增量，call ID 关联后续工具结果，两者不能混用。
				calls[fc.ID] = &pendingCall{id: fc.CallID, name: fc.Name}
			}
		case "response.function_call_arguments.delta":
			p := calls[ev.ItemID]
			if p == nil {
				text := fmt.Sprintf("工具参数增量对应的调用 %q 尚未声明", ev.ItemID)
				return nil, streamDetail{text: text}, true
			}
			p.args.WriteString(ev.Delta)
			emit(agent.Delta{Type: agent.DeltaTypeToolCall, ToolCall: agent.ToolCall{
				ID:        p.id,
				Name:      p.name,
				Arguments: jsontext.Value(p.args.String()),
			}})
		case "response.completed", "response.incomplete", "response.failed":
			r := ev.Response
			final = &r
		case "error":
			detail = streamDetail{code: ev.Code, text: ev.Message} // 记录细节，供无终态时兜底。
		}
	}
	return final, detail, false
}
