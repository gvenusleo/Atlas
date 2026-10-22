package agent

import (
	"context"
	"testing"
	"time"
)

// 暂时性失败按策略重试，失败的尝试整个作废、不入历史；不可重试的失败、重试用尽
// 与退避期间取消各自收尾。
func TestRetry(t *testing.T) {
	failure := func(kind ErrorKind) AssistantMessage {
		return AssistantMessage{StopReason: StopReasonError, ErrorMessage: "boom", ErrorKind: kind}
	}
	retryable := failure(ErrorKindRetryable)
	ok := AssistantMessage{StopReason: StopReasonStop, Content: []Content{TextContent{Text: "好"}}}
	type script = []AssistantMessage // 每次调用依次返回

	for _, tt := range []struct {
		name           string
		script         script
		max            int
		cancelOnRetry  bool
		want           StopReason
		calls, retries int
	}{
		{"失败两次后成功", script{retryable, retryable, ok}, 4, false, StopReasonStop, 3, 2},
		{"不可重试的立即收尾", script{failure("")}, 4, false, StopReasonError, 1, 0},
		{"重试用尽", script{retryable, retryable, retryable}, 2, false, StopReasonError, 3, 2},
		{"退避期间取消", script{retryable, ok}, 4, true, StopReasonCanceled, 1, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls, retries := 0, 0
			saved := retryPolicy
			retryPolicy = RetryPolicy{Max: tt.max, Base: time.Millisecond}
			t.Cleanup(func() { retryPolicy = saved })
			in := Input{Model: Model{Ref: "m",
				Stream: func(context.Context, StreamRequest, func(Delta)) AssistantMessage {
					calls++
					return tt.script[calls-1]
				}}}
			prompt := UserMessage{Content: []Content{TextContent{Text: "你好"}}}
			messages := Run(ctx, in, prompt, func(e Event) {
				if e.Type == EventRetry {
					retries++
					if tt.cancelOnRetry {
						cancel()
					}
				}
			})

			if len(messages) != 2 {
				t.Fatalf("新消息 = %d 条，期望 2（提示与最终响应），失败的尝试不能入历史",
					len(messages))
			}
			if got := messages[1].(AssistantMessage).StopReason; got != tt.want {
				t.Errorf("结束原因 = %q，期望 %q", got, tt.want)
			}
			if calls != tt.calls || retries != tt.retries {
				t.Errorf("调用 %d 次、重试事件 %d 次，期望 %d 与 %d",
					calls, retries, tt.calls, tt.retries)
			}
		})
	}
}
