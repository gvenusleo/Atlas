package compact

import "strings"

// contextKeywords 是与上下文/输入直接相关的关键词。
// 单独出现这些词不足以判定 overflow，需要配合 limitKeywords 组合匹配。
var contextKeywords = []string{
	"context",
	"input",
	"messages",
	"items",
	"tokens",
}

// limitKeywords 是表示数量/长度限制的词。
// 单独出现这些词不足以判定 overflow（如 "maximum of 5 attachments"）。
var limitKeywords = []string{
	"maximum of",
	"maximum length",
	"maximum number of",
	"too long",
	"too many",
}

// exactPatterns 是自身已足够明确、无需组合匹配的 overflow 模式。
var exactPatterns = []string{
	"context length",
	"context window",
	"context_limit",
}

// IsContextOverflow 判断错误是否表示上下文超限（token 或 input item 数量）。
// 用于 agent 循环在 provider 返回 400 时决定是否触发自动压缩恢复。
//
// 匹配策略分两层：
//  1. 精确模式：错误消息包含 "context length"、"context window"、"context_limit" 等自身明确的模式。
//  2. 组合模式：错误消息同时包含一个 limitKeywords 词和一个 contextKeywords 词，
//     例如 "Maximum of 1000 items allowed in input" 同时命中 "maximum of" + "items"/"input"。
//     这避免了 "maximum of 5 attachments" 等无关 400 误触发压缩。
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// 精确模式
	for _, pattern := range exactPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	// 组合模式：limitKeyword + contextKeyword
	hasLimit := false
	for _, kw := range limitKeywords {
		if strings.Contains(msg, kw) {
			hasLimit = true
			break
		}
	}
	if !hasLimit {
		return false
	}
	for _, kw := range contextKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
