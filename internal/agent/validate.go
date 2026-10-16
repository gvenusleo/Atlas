package agent

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateMessage 校验领域消息；pending 可用于流式快照，存储层另行拒绝保存。
func ValidateMessage(message Message) error {
	switch m := message.(type) {
	case SystemMessage:
		return validateSystem(m)
	case UserMessage:
		return validateContent(m.Role(), m.Content)
	case AssistantMessage:
		if err := validateAssistant(m); err != nil {
			return err
		}
		return validateContent(m.Role(), m.Content)
	case ToolResultMessage:
		if m.ToolCallID == "" || m.ToolName == "" {
			return errors.New("工具结果缺少调用 ID 或工具名称")
		}
		return validateContent(m.Role(), m.Content)
	default:
		return fmt.Errorf("不支持消息类型 %T", message)
	}
}

func validateSystem(m SystemMessage) error {
	switch m.Type {
	case SystemMessageTypePrompt:
		if strings.TrimSpace(m.Content) == "" {
			return errors.New("系统提示词快照不能为空")
		}
	case SystemMessageTypeNotice:
	case SystemMessageTypeCompaction:
		if strings.TrimSpace(m.Content) == "" {
			return errors.New("压缩记录的摘要不能为空")
		}
		if m.Kept < 0 {
			return errors.New("压缩记录保留的消息数不能为负")
		}
	default:
		return fmt.Errorf("未知系统消息类型 %q", m.Type)
	}
	return nil
}

func validateAssistant(m AssistantMessage) error {
	if m.Model == "" {
		return errors.New("助手消息缺少模型 ID")
	}
	switch m.StopReason {
	case StopReasonPending, StopReasonStop, StopReasonToolUse, StopReasonMaxOutputTokens,
		StopReasonError, StopReasonCanceled:
	default:
		return fmt.Errorf("未知助手结束状态 %q", m.StopReason)
	}
	return m.Usage.Validate()
}

// validateContent 检查内容块种类是否被角色允许。
func validateContent(role Role, content []Content) error {
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
