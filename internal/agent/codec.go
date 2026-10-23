package agent

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
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
	role, err := messageRole(data)
	if err != nil {
		return nil, err
	}
	opts := []json.Options{
		json.RejectUnknownMembers(true), json.WithUnmarshalers(contentUnmarshalers),
	}
	var message Message
	switch role {
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
		return nil, fmt.Errorf("不支持消息角色 %q", role)
	}
	if err := ValidateMessage(message); err != nil {
		return nil, err
	}
	return message, nil
}

// messageRole 读出消息的角色，并检查正文的形状：系统消息的正文是字符串，其余是数组。
func messageRole(data []byte) (Role, error) {
	var header struct {
		Role    Role          `json:"role"`
		Content jsontext.Kind `json:"content"`
	}
	err := json.Unmarshal(data, &header, json.WithUnmarshalers(contentKindUnmarshalers))
	if err != nil {
		return "", err
	}
	if header.Role == RoleSystem && header.Content != '"' {
		return "", errors.New("系统消息正文必须是字符串")
	}
	if header.Role != RoleSystem && header.Content != '[' {
		return "", errors.New("消息内容必须是数组")
	}
	return header.Role, nil
}

// 分派消息类型时只记录正文的起始类型，不复制完整正文。
var contentKindUnmarshalers = json.UnmarshalFromFunc(
	func(dec *jsontext.Decoder, kind *jsontext.Kind) error {
		*kind = dec.PeekKind()
		return dec.SkipValue()
	})

var contentMarshalers = json.MarshalToFunc(func(enc *jsontext.Encoder, content Content) error {
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
var argumentMarshalers = json.MarshalToFunc(
	func(enc *jsontext.Encoder, value jsontext.Value) error {
		return json.MarshalEncode(enc, string(value))
	})

var argumentUnmarshalers = json.UnmarshalFromFunc(
	func(dec *jsontext.Decoder, value *jsontext.Value) error {
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

var contentUnmarshalers = json.UnmarshalFromFunc(func(
	dec *jsontext.Decoder, content *Content,
) error {
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
	opts := []json.Options{
		json.RejectUnknownMembers(true), json.WithUnmarshalers(argumentUnmarshalers),
	}
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
