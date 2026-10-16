package llm

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/openai/openai-go/v3"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 暂时性的失败标为可重试：限流、服务端故障、连接中断；
// 配额用尽、证书校验失败与客户端错误不重试。
func TestErrorKind(t *testing.T) {
	api := func(status int, code string) error {
		return &openai.Error{StatusCode: status, Code: code}
	}
	retry := agent.ErrorKindRetryable
	for _, tt := range []struct {
		name string
		err  error
		want agent.ErrorKind
	}{
		{"限流", api(429, "rate_limit_exceeded"), retry},
		{"服务端故障", api(503, ""), retry},
		{"被包装的 API 错误", fmt.Errorf("读取流: %w", api(502, "")), retry},
		{"连接中断", fmt.Errorf("读取流: %w", io.ErrUnexpectedEOF), retry},
		{"证书校验失败", fmt.Errorf("读取流: %w", &tls.CertificateVerificationError{
			Err: errors.New("x509: unknown authority")}), ""},
		{"配额用尽", api(429, "insufficient_quota"), ""},
		{"参数错误", api(400, "invalid_request_error"), ""},
		{"鉴权失败", api(401, ""), ""},
	} {
		if got := errorKind(tt.err); got != tt.want {
			t.Errorf("%s：errorKind = %q，期望 %q", tt.name, got, tt.want)
		}
	}
	for code, want := range map[string]agent.ErrorKind{
		"server_error": retry, "rate_limit_exceeded": retry, "invalid_prompt": "", "": "",
	} {
		if got := codeKind(code); got != want {
			t.Errorf("codeKind(%q) = %q，期望 %q", code, got, want)
		}
	}
}
