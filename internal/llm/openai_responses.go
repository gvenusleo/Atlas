// Package llm 把 agent.StreamFn 落到具体的 LLM API 上。
// 适配器命名 = 厂商_wireAPI（如 OpenAIResponses），wire API 不同即为不同适配器。
package llm

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/gvenusleo/atlas/internal/agent"
)

// OpenAIResponsesStreamFn 返回基于 OpenAI Responses API 的 StreamFn；
// 失败按契约编码进返回消息：ctx 取消 → StopReasonCanceled，其余 → StopReasonError；
// 请求构造类错误在发起 HTTP 之前返回。
func OpenAIResponsesStreamFn(client openai.Client) agent.StreamFn {
	return func(ctx context.Context, req agent.StreamRequest, emit func(agent.Delta)) agent.AssistantMessage {
		params, err := requestOf(req)
		if err != nil {
			return agent.AssistantMessage{Model: req.Model, StopReason: agent.StopReasonError, ErrorMessage: err.Error()}
		}

		stream := client.Responses.NewStreaming(ctx, params)
		defer func() { _ = stream.Close() }()

		var final *responses.Response
		var errMsg string
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
					return agent.AssistantMessage{Model: req.Model, StopReason: agent.StopReasonError,
						ErrorMessage: fmt.Sprintf("工具参数增量对应的调用 %q 尚未声明", ev.ItemID)}
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
				errMsg = ev.Message // 记录细节，供无终态时兜底。
			}
		}

		if err := stream.Err(); err != nil {
			stop := agent.StopReasonError
			if errors.Is(err, context.Canceled) {
				stop = agent.StopReasonCanceled
			}
			return agent.AssistantMessage{Model: req.Model, StopReason: stop, ErrorMessage: err.Error()}
		}
		if final == nil {
			if errMsg != "" {
				return agent.AssistantMessage{Model: req.Model, StopReason: agent.StopReasonError, ErrorMessage: errMsg}
			}
			return agent.AssistantMessage{Model: req.Model, StopReason: agent.StopReasonError, ErrorMessage: "流结束但未收到终态事件"}
		}
		return messageOf(*final, req.Model)
	}
}

// requestOf 组装一次请求的参数；本地可判定的错误在这里全部拦下，不消耗网络调用。
func requestOf(req agent.StreamRequest) (responses.ResponseNewParams, error) {
	input, err := inputOf(req.Messages)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Tools: toolsOf(req.Tools),
	}
	switch req.ThinkingLevel {
	case "":
	case agent.ThinkingOff:
		params.Reasoning.Effort = shared.ReasoningEffortNone
	case agent.ThinkingMinimal, agent.ThinkingLow, agent.ThinkingMedium,
		agent.ThinkingHigh, agent.ThinkingXHigh, agent.ThinkingMax:
		params.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(req.ThinkingLevel),
			Summary: shared.ReasoningSummaryAuto,
		}
		params.Include = []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent}
	default:
		return responses.ResponseNewParams{}, fmt.Errorf("未知思考深度 %q", req.ThinkingLevel)
	}
	return params, nil
}

// pendingCall 保存单个工具调用的元信息与参数累积，仅用于实时快照。
type pendingCall struct {
	id, name string
	args     strings.Builder
}

// messageOf 按终态输出顺序构造完整消息，并映射结束原因与 token 用量；
// 推理项同时保留展示文本与原始 JSON，仅含加密数据的项也用于后续回传。
func messageOf(r responses.Response, model string) agent.AssistantMessage {
	msg := agent.AssistantMessage{Model: model, Usage: usageOf(r.Usage)}
	for _, item := range r.Output {
		switch v := item.AsAny().(type) {
		case responses.ResponseReasoningItem:
			text := reasoningText(v)
			if text != "" || v.RawJSON() != "" {
				msg.Content = append(msg.Content, agent.ThinkingContent{Thinking: text, Signature: v.RawJSON()})
			}
		case responses.ResponseOutputMessage:
			for _, p := range v.Content {
				switch t := p.AsAny().(type) {
				case responses.ResponseOutputText:
					msg.Content = append(msg.Content, agent.TextContent{Text: t.Text})
				case responses.ResponseOutputRefusal:
					msg.Content = append(msg.Content, agent.TextContent{Text: t.Refusal}) // 拒答回传，避免静默丢内容。
				}
			}
		case responses.ResponseFunctionToolCall:
			msg.Content = append(msg.Content, agent.ToolCall{
				ID:        v.CallID,
				Name:      v.Name,
				Arguments: jsontext.Value(v.Arguments),
			})
		}
	}
	switch r.Status {
	case responses.ResponseStatusCompleted:
		msg.StopReason = agent.StopReasonStop
		for _, c := range msg.Content {
			if _, ok := c.(agent.ToolCall); ok {
				msg.StopReason = agent.StopReasonToolUse
				break
			}
		}
	case responses.ResponseStatusIncomplete:
		if r.IncompleteDetails.Reason == "max_output_tokens" {
			msg.StopReason = agent.StopReasonMaxOutputTokens
		} else {
			msg.StopReason = agent.StopReasonError
			msg.ErrorMessage = "响应不完整：" + r.IncompleteDetails.Reason
		}
	case responses.ResponseStatusFailed:
		msg.StopReason = agent.StopReasonError
		msg.ErrorMessage = r.Error.Message
	case responses.ResponseStatusCancelled:
		msg.StopReason = agent.StopReasonCanceled
	default:
		msg.StopReason = agent.StopReasonError
		msg.ErrorMessage = "非终态或未知的响应状态：" + string(r.Status)
	}
	return msg
}

// reasoningText 取终态推理文本：content 优先，为空时取 summary。
func reasoningText(item responses.ResponseReasoningItem) string {
	content := make([]string, 0, len(item.Content))
	for _, c := range item.Content {
		if c.Text != "" {
			content = append(content, c.Text)
		}
	}
	if text := strings.Join(content, "\n\n"); text != "" {
		return text
	}
	summary := make([]string, 0, len(item.Summary))
	for _, stm := range item.Summary {
		if stm.Text != "" {
			summary = append(summary, stm.Text)
		}
	}
	return strings.Join(summary, "\n\n")
}

// usageOf 归一化 token 口径：InputTokens 是未命中缓存的净输入（假定 provider 的
// input_tokens 含缓存读写项）；TotalTokens 由四项相加得出。
func usageOf(u responses.ResponseUsage) agent.Usage {
	read := u.InputTokensDetails.CachedTokens
	write := u.InputTokensDetails.CacheWriteTokens
	input := max(0, u.InputTokens-read-write)
	total := input + read + write + u.OutputTokens
	return agent.Usage{
		InputTokens:      input,
		CacheReadTokens:  read,
		CacheWriteTokens: write,
		OutputTokens:     u.OutputTokens,
		TotalTokens:      total,
	}
}

// ---- 消息 → input items ----

// inputOf 把消息历史全量重放为 input items，assistant 推理项按原顺序回传；
// 图片允许出现在 user 消息与工具结果；assistant 图片报错而非丢弃。
func inputOf(msgs []agent.Message) (responses.ResponseInputParam, error) {
	var items responses.ResponseInputParam
	for _, m := range msgs {
		switch v := m.(type) {
		case agent.SystemMessage:
			items = append(items, easyContent(responses.EasyInputMessageRoleSystem,
				responses.EasyInputMessageContentUnionParam{OfString: openai.String(v.Content)}))
		case agent.UserMessage:
			content, err := messageContent(v.Content)
			if err != nil {
				return nil, fmt.Errorf("user 消息: %w", err)
			}
			items = append(items, easyContent(responses.EasyInputMessageRoleUser, content))
		case agent.AssistantMessage:
			for _, c := range v.Content {
				switch p := c.(type) {
				case agent.ThinkingContent:
					if p.Signature == "" {
						return nil, errors.New("assistant 思考内容缺少原始推理项，无法回传")
					}
					var reasoning responses.ResponseReasoningItem
					if err := json.Unmarshal([]byte(p.Signature), &reasoning); err != nil {
						return nil, fmt.Errorf("assistant 推理项解析失败: %w", err)
					}
					if reasoning.ID == "" || reasoning.Type != "reasoning" {
						return nil, errors.New("assistant 推理项必须包含 id 且 type 为 reasoning")
					}
					// ToParam 使用原始 JSON 回传，保留推理标识、加密数据及其他原始字段。
					items = append(items, responses.ResponseInputItemUnionParam{OfReasoning: new(reasoning.ToParam())})
				case agent.TextContent:
					if p.Text != "" {
						items = append(items, easyContent(responses.EasyInputMessageRoleAssistant,
							responses.EasyInputMessageContentUnionParam{OfString: openai.String(p.Text)}))
					}
				case agent.ToolCall:
					items = append(items, responses.ResponseInputItemUnionParam{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							CallID:    p.ID,
							Name:      p.Name,
							Arguments: string(p.Arguments),
						},
					})
				case agent.ImageContent:
					return nil, errors.New("assistant 消息里的图片无法在 Responses 输入侧表达")
				default:
					return nil, fmt.Errorf("assistant 消息里的未知内容块 %T", c)
				}
			}
		case agent.ToolResultMessage:
			output, err := toolOutput(v.Content)
			if err != nil {
				return nil, fmt.Errorf("工具 %s 的结果: %w", v.ToolName, err)
			}
			items = append(items, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: openai.String(v.ToolCallID),
					Output: output,
				},
			})
		default:
			return nil, fmt.Errorf("未知消息类型 %T", m)
		}
	}
	return items, nil
}

// part 是内容块的中间表示，供消息内容与工具结果两种 wire 形状复用同一套遍历与校验。
type part struct {
	kind  partKind
	text  string
	image agent.ImageContent
}

// partKind 区分 user 消息与工具结果允许包含的文本、图片。
type partKind int

const (
	partText partKind = iota
	partImage
)

// parts 按原始顺序提取内容块；无法在 wire 上表达的形态一律报错，绝不静默丢弃。
func parts(content []agent.Content) ([]part, error) {
	out := make([]part, 0, len(content))
	for _, c := range content {
		switch v := c.(type) {
		case agent.TextContent:
			out = append(out, part{kind: partText, text: v.Text})
		case agent.ImageContent:
			if err := validateImage(v); err != nil {
				return nil, err
			}
			out = append(out, part{kind: partImage, image: v})
		case agent.ToolCall:
			return nil, errors.New("只有 assistant 消息可以包含 ToolCall")
		case agent.ThinkingContent:
			return nil, errors.New("只有 assistant 消息可以包含思考内容")
		default:
			return nil, fmt.Errorf("未知内容块 %T", c)
		}
	}
	return out, nil
}

// messageContent 映射消息内容：纯文本走字符串快路径，含图片时用保序的内容数组。
func messageContent(content []agent.Content) (responses.EasyInputMessageContentUnionParam, error) {
	ps, err := parts(content)
	if err != nil {
		return responses.EasyInputMessageContentUnionParam{}, err
	}
	if !hasImage(ps) {
		return responses.EasyInputMessageContentUnionParam{OfString: openai.String(joinText(ps))}, nil
	}
	list := make(responses.ResponseInputMessageContentListParam, 0, len(ps))
	for _, p := range ps {
		switch {
		case p.kind == partImage:
			list = append(list, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					Detail:   responses.ResponseInputImageDetailAuto,
					ImageURL: openai.String(dataURL(p.image)),
				},
			})
		case p.text != "":
			list = append(list, responses.ResponseInputContentUnionParam{
				OfInputText: &responses.ResponseInputTextParam{Text: p.text},
			})
		}
	}
	return responses.EasyInputMessageContentUnionParam{OfInputItemContentList: list}, nil
}

// toolOutput 映射工具结果内容：纯文本走字符串快路径，含图片时用保序的内容数组。
func toolOutput(content []agent.Content) (responses.ResponseInputItemFunctionCallOutputOutputUnionParam, error) {
	ps, err := parts(content)
	if err != nil {
		return responses.ResponseInputItemFunctionCallOutputOutputUnionParam{}, err
	}
	if !hasImage(ps) {
		return responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: openai.String(joinText(ps))}, nil
	}
	list := make(responses.ResponseFunctionCallOutputItemListParam, 0, len(ps))
	for _, p := range ps {
		switch {
		case p.kind == partImage:
			list = append(list, responses.ResponseFunctionCallOutputItemUnionParam{
				OfInputImage: &responses.ResponseInputImageContentParam{
					Detail:   responses.ResponseInputImageContentDetailAuto,
					ImageURL: openai.String(dataURL(p.image)),
				},
			})
		case p.text != "":
			list = append(list, responses.ResponseFunctionCallOutputItemUnionParam{
				OfInputText: &responses.ResponseInputTextContentParam{Text: p.text},
			})
		}
	}
	return responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
		OfResponseFunctionCallOutputItemArray: list,
	}, nil
}

// validateImage 检查图片 MIME 非空及 base64 可解码；不验证图像格式或模型能力。
func validateImage(img agent.ImageContent) error {
	if img.MIMEType == "" {
		return errors.New("图像缺少 MIME 类型")
	}
	// 流式解码器可能接受跨块拼接的填充片段；只允许末尾出现至多两个填充符。
	if padding := strings.IndexByte(img.Data, '='); padding >= 0 {
		suffix := img.Data[padding:]
		if strings.Trim(suffix, "=\r\n") != "" || strings.Count(suffix, "=") > 2 {
			return errors.New("图像数据不是合法 base64: 填充符后仍有数据或填充过多")
		}
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(img.Data))
	if _, err := io.Copy(io.Discard, decoder); err != nil {
		return fmt.Errorf("图像数据不是合法 base64: %w", err)
	}
	return nil
}

// dataURL 将已校验的图片包装为 Responses 接受的内嵌数据 URL。
func dataURL(img agent.ImageContent) string {
	return "data:" + img.MIMEType + ";base64," + img.Data
}

// joinText 按原顺序连接文本块，不插入额外分隔符。
func joinText(ps []part) string {
	var b strings.Builder
	for _, p := range ps {
		if p.kind == partText {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// hasImage 判断是否需要使用包含图片的 wire 内容数组。
func hasImage(ps []part) bool {
	for _, p := range ps {
		if p.kind == partImage {
			return true
		}
	}
	return false
}

// easyContent 将指定角色与内容包装为 Responses 的简单消息输入项。
func easyContent(role responses.EasyInputMessageRole, content responses.EasyInputMessageContentUnionParam) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{
		Role:    role,
		Content: content,
	}}
}

// toolsOf 映射工具声明；未声明参数时使用无参对象 schema。
func toolsOf(tools []agent.Tool) []responses.ToolUnionParam {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  params,
		}})
	}
	return out
}
