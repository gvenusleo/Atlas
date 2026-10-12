package agent

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
)

// MarshalMessage 使用领域类型的字段定义，只补充角色和内容类型标记。
func MarshalMessage(message Message) ([]byte, error) {
	if err := ValidateMessage(message); err != nil {
		return nil, err
	}
	var value any
	switch m := message.(type) {
	case SystemMessage:
		value = struct {
			Role Role `json:"role"`
			SystemMessage
		}{Role: m.Role(), SystemMessage: m}
	case UserMessage:
		value = struct {
			Role Role `json:"role"`
			UserMessage
		}{Role: m.Role(), UserMessage: m}
	case AssistantMessage:
		value = struct {
			Role Role `json:"role"`
			AssistantMessage
		}{Role: m.Role(), AssistantMessage: m}
	case ToolResultMessage:
		value = struct {
			Role Role `json:"role"`
			ToolResultMessage
		}{Role: m.Role(), ToolResultMessage: m}
	}
	return json.Marshal(value, json.WithMarshalers(contentMarshalers))
}

// UnmarshalMessage 恢复值类型消息；未知字段和类型明确报错。
func UnmarshalMessage(data []byte) (Message, error) {
	var header struct {
		Role    Role          `json:"role"`
		Content jsontext.Kind `json:"content"`
	}
	if err := json.Unmarshal(data, &header, json.WithUnmarshalers(contentKindUnmarshalers)); err != nil {
		return nil, err
	}
	if header.Role == RoleSystem {
		if header.Content != '"' {
			return nil, errors.New("系统消息正文必须是字符串")
		}
	} else if header.Content != '[' {
		return nil, errors.New("消息内容必须是数组")
	}
	opts := []json.Options{json.RejectUnknownMembers(true), json.WithUnmarshalers(contentUnmarshalers)}
	var message Message
	switch header.Role {
	case RoleSystem:
		var value struct {
			Role Role `json:"role"`
			SystemMessage
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return nil, err
		}
		message = value.SystemMessage
	case RoleUser:
		var value struct {
			Role Role `json:"role"`
			UserMessage
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return nil, err
		}
		message = value.UserMessage
	case RoleAssistant:
		var value struct {
			Role Role `json:"role"`
			AssistantMessage
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return nil, err
		}
		message = value.AssistantMessage
	case RoleToolResult:
		var value struct {
			Role Role `json:"role"`
			ToolResultMessage
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return nil, err
		}
		message = value.ToolResultMessage
	default:
		return nil, fmt.Errorf("不支持消息角色 %q", header.Role)
	}
	if err := ValidateMessage(message); err != nil {
		return nil, err
	}
	return message, nil
}

// 分派消息类型时只记录正文的起始类型，不复制完整正文。
var contentKindUnmarshalers = json.UnmarshalFromFunc[*jsontext.Kind](func(dec *jsontext.Decoder, kind *jsontext.Kind) error {
	*kind = dec.PeekKind()
	return dec.SkipValue()
})

var contentMarshalers = json.MarshalToFunc[Content](func(enc *jsontext.Encoder, content Content) error {
	// json/v2 会将接口中的值取址后传入编码器；公共入口已验证值类型契约。
	switch c := content.(type) {
	case *TextContent:
		content = *c
	case *ThinkingContent:
		content = *c
	case *ImageContent:
		content = *c
	case *ToolCall:
		content = *c
	}
	var value any
	switch c := content.(type) {
	case TextContent:
		value = struct {
			Type string `json:"type"`
			TextContent
		}{Type: c.contentType(), TextContent: c}
	case ThinkingContent:
		value = struct {
			Type string `json:"type"`
			ThinkingContent
		}{Type: c.contentType(), ThinkingContent: c}
	case ImageContent:
		value = struct {
			Type string `json:"type"`
			ImageContent
		}{Type: c.contentType(), ImageContent: c}
	case ToolCall:
		value = struct {
			Type string `json:"type"`
			ToolCall
		}{Type: c.contentType(), ToolCall: c}
	default:
		return fmt.Errorf("不支持内容类型 %T", content)
	}
	// 替换当前 Content 编码器，防止嵌入领域类型的视图再次匹配 Content。
	return json.MarshalEncode(enc, value, json.WithMarshalers(argumentMarshalers))
})

// 原始参数可能是截断 JSON；编码为字符串才能无损保存整个消息。
var argumentMarshalers = json.MarshalToFunc[jsontext.Value](func(enc *jsontext.Encoder, value jsontext.Value) error {
	return json.MarshalEncode(enc, string(value))
})

var argumentUnmarshalers = json.UnmarshalFromFunc[*jsontext.Value](func(dec *jsontext.Decoder, value *jsontext.Value) error {
	if dec.PeekKind() != '"' {
		return errors.New("工具参数必须以字符串编码")
	}
	var text string
	if err := json.UnmarshalDecode(dec, &text); err != nil {
		return err
	}
	*value = jsontext.Value(text)
	return nil
})

var contentUnmarshalers = json.UnmarshalFromFunc[*Content](func(dec *jsontext.Decoder, content *Content) error {
	data, err := dec.ReadValue()
	if err != nil {
		return err
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	opts := []json.Options{json.RejectUnknownMembers(true), json.WithUnmarshalers(argumentUnmarshalers)}
	switch header.Type {
	case "text":
		var value struct {
			Type string `json:"type"`
			TextContent
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return err
		}
		*content = value.TextContent
	case "thinking":
		var value struct {
			Type string `json:"type"`
			ThinkingContent
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return err
		}
		*content = value.ThinkingContent
	case "image":
		var value struct {
			Type string `json:"type"`
			ImageContent
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return err
		}
		*content = value.ImageContent
	case "tool_call":
		var value struct {
			Type string `json:"type"`
			ToolCall
		}
		if err := json.Unmarshal(data, &value, opts...); err != nil {
			return err
		}
		*content = value.ToolCall
	default:
		return fmt.Errorf("不支持内容类型 %q", header.Type)
	}
	return nil
})

// ValidateMessage 校验领域消息；pending 可用于流式快照，存储层另行拒绝保存。
func ValidateMessage(message Message) error {
	var content []Content
	var role Role
	switch m := message.(type) {
	case SystemMessage:
		switch m.Type {
		case SystemMessageTypePrompt:
			if strings.TrimSpace(m.Content) == "" {
				return errors.New("系统提示词快照不能为空")
			}
		case SystemMessageTypeNotice:
		default:
			return fmt.Errorf("未知系统消息类型 %q", m.Type)
		}
		return nil
	case UserMessage:
		role, content = m.Role(), m.Content
	case AssistantMessage:
		if m.Model == "" {
			return errors.New("助手消息缺少模型 ID")
		}
		switch m.StopReason {
		case StopReasonPending, StopReasonStop, StopReasonToolUse, StopReasonMaxOutputTokens,
			StopReasonError, StopReasonCanceled:
		default:
			return fmt.Errorf("未知助手结束状态 %q", m.StopReason)
		}
		if err := m.Usage.Validate(); err != nil {
			return err
		}
		role, content = m.Role(), m.Content
	case ToolResultMessage:
		if m.ToolCallID == "" || m.ToolName == "" {
			return errors.New("工具结果缺少调用 ID 或工具名称")
		}
		role, content = m.Role(), m.Content
	default:
		return fmt.Errorf("不支持消息类型 %T", message)
	}
	for _, block := range content {
		switch c := block.(type) {
		case TextContent:
		case ImageContent:
			if role == RoleAssistant {
				return errors.New("助手消息不支持图片内容")
			}
		case ThinkingContent:
			if role != RoleAssistant {
				return fmt.Errorf("角色 %q 不支持思考内容", role)
			}
		case ToolCall:
			if role != RoleAssistant {
				return fmt.Errorf("角色 %q 不支持工具调用", role)
			}
			if c.ID == "" || c.Name == "" {
				return errors.New("工具调用缺少 ID 或名称")
			}
		default:
			return fmt.Errorf("不支持内容类型 %T", block)
		}
	}
	return nil
}
