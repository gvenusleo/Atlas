package compact

import (
	"errors"
	"testing"
)

func TestIsContextOverflow(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// --- 真实 overflow 场景 ---
		{
			name: "openai responses max items in input",
			err:  errors.New(`responses request failed: status 400: {"error":{"code":"InvalidParameter","message":"The parameter input specified in the request are not valid: Invalid input: Maximum of 1000 items allowed in input."}}`),
			want: true,
		},
		{
			name: "azure messages exceeds maximum length",
			err:  errors.New(`chat completion failed: status 400: $.messages exceeds the maximum length of 2048 items`),
			want: true,
		},
		{
			name: "context length exceeded",
			err:  errors.New("context length exceeded, please reduce the number of messages"),
			want: true,
		},
		{
			name: "context window exceeded",
			err:  errors.New("This model's maximum context window is 128000 tokens"),
			want: true,
		},
		{
			name: "context_limit_exceeded",
			err:  errors.New("context_limit_exceeded: your request exceeded the limit"),
			want: true,
		},
		{
			name: "maximum number of input tokens",
			err:  errors.New("You exceeded the maximum number of input tokens"),
			want: true,
		},
		{
			name: "messages array too long",
			err:  errors.New("$.messages array is too long"),
			want: true,
		},
		{
			name: "too many input tokens",
			err:  errors.New("too many input tokens in the request"),
			want: true,
		},

		// --- 非 overflow 场景（不应误触发压缩） ---
		{
			name: "network error",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "auth error",
			err:  errors.New("status 401: unauthorized"),
			want: false,
		},
		{
			name: "rate limit",
			err:  errors.New("status 429: too many requests"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic 400",
			err:  errors.New("status 400: bad request"),
			want: false,
		},
		{
			name: "maximum of 5 attachments",
			err:  errors.New("maximum of 5 attachments allowed"),
			want: false,
		},
		{
			name: "parameter value too long",
			err:  errors.New("parameter value is too long"),
			want: false,
		},
		{
			name: "maximum length of parameter name",
			err:  errors.New("field name exceeds maximum length of 64 characters"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContextOverflow(tt.err); got != tt.want {
				t.Errorf("IsContextOverflow() = %v, want %v", got, tt.want)
			}
		})
	}
}
