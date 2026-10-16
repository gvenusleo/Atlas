package llm

import (
	"crypto/tls"
	"errors"
	"regexp"

	"github.com/openai/openai-go/v3"

	"github.com/gvenusleo/atlas/internal/agent"
)

// overflowText 匹配兼容各家 API 的“上下文超限”错误文字：错误码因厂商而异，文字更通用。
var overflowText = regexp.MustCompile(`(?i)context[_ ]length|context window|maximum context|` +
	`(prompt|input) is too long|request_too_large`)

// errorKind 判断读取流时的错误属于哪一类。API 错误看状态码：请求超出上下文窗口要先压缩；
// 请求超时、冲突、限流与服务端故障可以重试；配额用尽和其他客户端错误不行。
// 其余是传输或解码错误（连接中断、被截断的流），重试通常能恢复；
// 证书校验失败是配置问题，重试也一样会失败。
func errorKind(err error) agent.ErrorKind {
	api, ok := errors.AsType[*openai.Error](err)
	if !ok {
		if _, bad := errors.AsType[*tls.CertificateVerificationError](err); bad {
			return ""
		}
		return agent.ErrorKindRetryable
	}
	switch {
	case isOverflow(api):
		return agent.ErrorKindContextOverflow
	case api.Code == "insufficient_quota" || api.Type == "insufficient_quota":
		return ""
	case api.StatusCode == 408 || api.StatusCode == 409 || api.StatusCode == 429 ||
		api.StatusCode >= 500:
		return agent.ErrorKindRetryable
	}
	return ""
}

// isOverflow 报告 API 错误是否是请求超出了上下文窗口：只看客户端错误，免得把限流误判进来。
func isOverflow(api *openai.Error) bool {
	if api.Code == "context_length_exceeded" {
		return true
	}
	switch api.StatusCode {
	case 400, 413, 422:
		return overflowText.MatchString(api.Code + " " + api.Message)
	}
	return false
}

// codeKind 判断事件流或失败响应里的错误码属于哪一类。
func codeKind(code string) agent.ErrorKind {
	switch code {
	case "server_error", "rate_limit_exceeded":
		return agent.ErrorKindRetryable
	case "context_length_exceeded":
		return agent.ErrorKindContextOverflow
	}
	return ""
}
