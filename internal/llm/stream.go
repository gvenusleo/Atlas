package llm

import (
	"fmt"

	"github.com/gvenusleo/atlas/internal/agent"
)

// OpenAIResponsesAPI 是 OpenAI Responses 协议在设置里的名字。
const OpenAIResponsesAPI = "openai-responses"

// Supports 报告协议是否已实现。核心只通过 Supports 与 NewStream 使用提供方，
// 不需要知道有哪些提供方。
func Supports(api string) bool {
	return api == OpenAIResponsesAPI
}

// NewStream 按提供方的协议创建流函数；协议尚未实现时返回错误。
func NewStream(api, baseURL, apiKey, modelID string, images bool) (agent.StreamFn, error) {
	if !Supports(api) {
		return nil, fmt.Errorf("协议 %q 尚未实现", api)
	}
	return OpenAIResponses(baseURL, apiKey, modelID, images), nil
}
