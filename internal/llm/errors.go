package llm

import (
	"crypto/tls"
	"errors"

	"github.com/openai/openai-go/v3"

	"github.com/gvenusleo/atlas/internal/agent"
)

// errorKind 判断读取流时的错误是否多半是暂时的。API 错误看状态码：请求超时、冲突、
// 限流与服务端故障可以重试，配额用尽和其他客户端错误不行；其余是传输或解码错误
// （连接中断、被截断的流），重试通常能恢复；证书校验失败是配置问题，重试也一样会失败。
func errorKind(err error) agent.ErrorKind {
	api, ok := errors.AsType[*openai.Error](err)
	if !ok {
		if _, bad := errors.AsType[*tls.CertificateVerificationError](err); bad {
			return ""
		}
		return agent.ErrorKindRetryable
	}
	switch {
	case api.Code == "insufficient_quota" || api.Type == "insufficient_quota":
		return ""
	case api.StatusCode == 408 || api.StatusCode == 409 || api.StatusCode == 429 ||
		api.StatusCode >= 500:
		return agent.ErrorKindRetryable
	}
	return ""
}

// codeKind 判断事件流或失败响应里的错误码是否是暂时性的。
func codeKind(code string) agent.ErrorKind {
	switch code {
	case "server_error", "rate_limit_exceeded":
		return agent.ErrorKindRetryable
	}
	return ""
}
