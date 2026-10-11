package session

import (
	"database/sql"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

// encodedMessage 只包含 SQL 参数，不定义另一套消息或内容字段。
type encodedMessage struct {
	role      agent.Role
	timestamp time.Time
	stop      agent.StopReason
	isError   bool
	body      string
	usage     sql.NullString
}

// encodeMessage 提取查询列并拆出 Usage，不修改调用方消息，拒绝保存流式占位消息。
func encodeMessage(message agent.Message) (encodedMessage, error) {
	var encoded encodedMessage
	switch m := message.(type) {
	case agent.SystemMessage:
		encoded.timestamp = m.Timestamp
	case agent.UserMessage:
		encoded.timestamp = m.Timestamp
	case agent.AssistantMessage:
		if m.StopReason == agent.StopReasonPending {
			return encodedMessage{}, errors.New("不能保存未定稿的助手消息")
		}
		if err := m.Usage.Validate(); err != nil {
			return encodedMessage{}, err
		}
		encoded.timestamp, encoded.stop = m.Timestamp, m.StopReason
		encoded.isError = m.StopReason == agent.StopReasonError
		data, err := json.Marshal(m.Usage)
		if err != nil {
			return encodedMessage{}, err
		}
		encoded.usage = sql.NullString{String: string(data), Valid: true}
		// 只修改值副本，正文省略 Usage；独立列保存唯一一份用量。
		m.Usage = agent.Usage{}
		message = m
	case agent.ToolResultMessage:
		encoded.timestamp, encoded.isError = m.Timestamp, m.IsError
	}
	data, err := agent.MarshalMessage(message)
	if err != nil {
		return encodedMessage{}, err
	}
	encoded.role, encoded.body = message.Role(), string(data)
	return encoded, nil
}

// decodeMessage 用统一编码恢复消息并合并独立 Usage，拒绝角色或用量不一致的记录。
func decodeMessage(body string, usage sql.NullString) (agent.Message, error) {
	message, err := agent.UnmarshalMessage([]byte(body))
	if err != nil {
		return nil, err
	}
	assistant, isAssistant := message.(agent.AssistantMessage)
	if isAssistant != usage.Valid {
		return nil, errors.New("消息角色与 Usage 字段不一致")
	}
	if isAssistant {
		if assistant.StopReason == agent.StopReasonPending {
			return nil, errors.New("存储中含未定稿的助手消息")
		}
		if assistant.Usage != (agent.Usage{}) {
			return nil, errors.New("消息正文重复保存了 Usage")
		}
		if err := json.Unmarshal([]byte(usage.String), &assistant.Usage, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if err := assistant.Usage.Validate(); err != nil {
			return nil, err
		}
		message = assistant
	}
	return message, nil
}
