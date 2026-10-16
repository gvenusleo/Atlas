package llm

import (
	"encoding/base64"
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

// inputItem 是 Responses 的输入项。
type inputItem = responses.ResponseInputItemUnionParam

// requestOf 组装一次请求的参数；本地可判定的错误在这里全部拦下，不消耗网络调用。
func requestOf(req agent.StreamRequest) (responses.ResponseNewParams, error) {
	input, err := inputOf(req.SystemPrompt, req.Messages)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Tools: toolsOf(req.Tools),
	}
	switch level := req.ThinkingLevel; {
	case level == "":
	case level == agent.ThinkingOff:
		params.Reasoning.Effort = shared.ReasoningEffortNone
	case level.Valid():
		params.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(level),
			Summary: shared.ReasoningSummaryAuto,
		}
		params.Include = []responses.ResponseIncludable{
			responses.ResponseIncludableReasoningEncryptedContent,
		}
	default:
		return responses.ResponseNewParams{}, fmt.Errorf("未知思考深度 %q", level)
	}
	return params, nil
}

// inputOf 先放入本次提示词，再转换请求历史；缺失工具结果临时补齐，失败或取消的响应跳过。
// 历史中的 system 只允许通知与压缩记录，提示词快照与未知类型报错。
// 图片允许出现在 user 消息与工具结果；assistant 图片报错而非丢弃。
func inputOf(systemPrompt string, msgs []agent.Message) (responses.ResponseInputParam, error) {
	var items responses.ResponseInputParam
	if systemPrompt != "" {
		items = append(items, textItem(responses.EasyInputMessageRoleSystem, systemPrompt))
	}
	for _, m := range messagesForRequest(msgs) {
		converted, err := itemsOf(m)
		if err != nil {
			return nil, err
		}
		items = append(items, converted...)
	}
	return items, nil
}

// itemsOf 把一条历史消息转换为零到多个输入项。
func itemsOf(m agent.Message) ([]inputItem, error) {
	switch v := m.(type) {
	case agent.SystemMessage:
		switch v.Type {
		case agent.SystemMessageTypeNotice:
			return []inputItem{textItem(responses.EasyInputMessageRoleSystem, v.Content)}, nil
		case agent.SystemMessageTypeCompaction:
			// 摘要以用户消息的身份回放，模型把它当作此前对话的交代而不是指令。
			text := "此前的对话已压缩成下面的摘要：\n\n<summary>\n" + v.Content + "\n</summary>"
			return []inputItem{textItem(responses.EasyInputMessageRoleUser, text)}, nil
		case agent.SystemMessageTypePrompt:
			return nil, errors.New("消息历史不能包含 system_prompt 提示词快照，" +
				"请通过 SystemPrompt 传入本次提示词")
		default:
			return nil, fmt.Errorf("消息历史中的未知 system 消息类型 %q", v.Type)
		}
	case agent.UserMessage:
		content, err := messageContent(v.Content)
		if err != nil {
			return nil, fmt.Errorf("user 消息: %w", err)
		}
		return []inputItem{easyContent(responses.EasyInputMessageRoleUser, content)}, nil
	case agent.AssistantMessage:
		return assistantItems(v)
	case agent.ToolResultMessage:
		output, err := toolOutput(v.Content)
		if err != nil {
			return nil, fmt.Errorf("工具 %s 的结果: %w", v.ToolName, err)
		}
		return []inputItem{{
			OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				CallID: openai.String(v.ToolCallID),
				Output: output,
			},
		}}, nil
	default:
		return nil, fmt.Errorf("未知消息类型 %T", m)
	}
}

// assistantItems 把助手内容块按原顺序转换为推理项、文本消息与工具调用。
func assistantItems(m agent.AssistantMessage) ([]inputItem, error) {
	var items []inputItem
	for _, c := range m.Content {
		switch p := c.(type) {
		case agent.ThinkingContent:
			item, err := reasoningItem(p)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		case agent.TextContent:
			if p.Text != "" {
				items = append(items, textItem(responses.EasyInputMessageRoleAssistant, p.Text))
			}
		case agent.ToolCall:
			items = append(items, inputItem{
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
	return items, nil
}

// reasoningItem 由保存的原始推理项恢复回传输入，保留推理标识、加密数据及其他原始字段。
func reasoningItem(p agent.ThinkingContent) (inputItem, error) {
	if p.Signature == "" {
		return inputItem{}, errors.New("assistant 思考内容缺少原始推理项，无法回传")
	}
	var reasoning responses.ResponseReasoningItem
	if err := json.Unmarshal([]byte(p.Signature), &reasoning); err != nil {
		return inputItem{}, fmt.Errorf("assistant 推理项解析失败: %w", err)
	}
	if reasoning.ID == "" || reasoning.Type != "reasoning" {
		return inputItem{}, errors.New("assistant 推理项必须包含 id 且 type 为 reasoning")
	}
	return inputItem{OfReasoning: new(reasoning.ToParam())}, nil
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
		return responses.EasyInputMessageContentUnionParam{
			OfString: openai.String(joinText(ps)),
		}, nil
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
func toolOutput(
	content []agent.Content,
) (responses.ResponseInputItemFunctionCallOutputOutputUnionParam, error) {
	type output = responses.ResponseInputItemFunctionCallOutputOutputUnionParam
	ps, err := parts(content)
	if err != nil {
		return output{}, err
	}
	if !hasImage(ps) {
		return output{OfString: openai.String(joinText(ps))}, nil
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
	return output{OfResponseFunctionCallOutputItemArray: list}, nil
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

// textItem 把纯文本包装为指定角色的 Responses 消息输入项。
func textItem(role responses.EasyInputMessageRole, text string) inputItem {
	return easyContent(role, responses.EasyInputMessageContentUnionParam{
		OfString: openai.String(text),
	})
}

// easyContent 将指定角色与内容包装为 Responses 的简单消息输入项。
func easyContent(
	role responses.EasyInputMessageRole, content responses.EasyInputMessageContentUnionParam,
) inputItem {
	return inputItem{OfMessage: &responses.EasyInputMessageParam{Role: role, Content: content}}
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
