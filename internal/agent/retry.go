package agent

import (
	"context"
	"time"
)

// RetryPolicy 是暂时性失败的重试策略：最多重试 Max 次，第 n 次重试前等待 Base·2^(n-1)。
type RetryPolicy struct {
	Max  int
	Base time.Duration
}

// retryPolicy 是模型调用的重试策略：重试 4 次，依次等待 2、4、8、16 秒，共约半分钟。
// 测试里改小。
var retryPolicy = RetryPolicy{Max: 4, Base: 2 * time.Second}

// RetryInfo 是 EventRetry 的载荷。
type RetryInfo struct {
	Attempt int           // 第几次重试，从 1 起
	Max     int           // 最多重试几次
	Delay   time.Duration // 这次重试前的等待
	Error   string        // 触发重试的错误
}

// delay 返回第 attempt 次重试前的等待。
func (p RetryPolicy) delay(attempt int) time.Duration {
	return p.Base << (attempt - 1)
}

// sleep 等待 d；ctx 先取消时提前返回 false。
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
