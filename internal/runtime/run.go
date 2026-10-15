package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

// Cancel 发出当前 Run 的取消信号，不等待收尾；空闲时无操作。
func (s *Session) Cancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

// WaitIdle 等待调用时已经接受的会话操作结束，不取消或等待未来的操作。
// 不得在本会话 Run 的事件回调中调用。
func (s *Session) WaitIdle(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run 同步执行并交付事件。回调可 Cancel 或 State，不得重入修改或等待方法。
// 接收的消息、事件与返回值均不共享内部历史的可变数据。
func (s *Session) Run(ctx context.Context, message agent.UserMessage, emit func(agent.Event)) ([]agent.Message, error) {
	ctx, end, err := s.manager.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	runCtx, cancel := context.WithCancel(ctx)
	if err := s.start(false, cancel); err != nil {
		cancel()
		return nil, err
	}
	defer func() {
		cancel()
		s.finish()
	}()
	if err := runCtx.Err(); err != nil {
		return nil, err
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}
	if err := agent.ValidateMessage(message); err != nil {
		return nil, err
	}
	message = cloneMessage(message).(agent.UserMessage)
	var saveErr error
	loopCompleted := false
	defer func() {
		if !loopCompleted {
			s.invalidate()
		}
	}()
	messages := agent.Run(runCtx, []agent.Message{message}, &s.loop, func(event agent.Event) {
		if event.Type == agent.EventTypeMessageEnd && saveErr == nil {
			saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			var seq int64
			seq, saveErr = s.manager.store.AppendMessage(saveCtx, s.id, s.seq, event.Message)
			stop()
			if saveErr == nil {
				s.seq = seq
			}
			if saveErr != nil {
				s.invalidate()
				cancel()
			}
		}
		if emit != nil {
			emit(cloneEvent(event))
		}
	})
	messages = cloneMessages(messages)
	loopCompleted = true
	if saveErr != nil {
		return messages, errors.Join(ErrNeedsReload, saveErr)
	}
	if err := runCtx.Err(); err != nil {
		return messages, err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if last, ok := messages[i].(agent.AssistantMessage); ok {
			switch last.StopReason {
			case agent.StopReasonError:
				return messages, fmt.Errorf("模型调用失败: %s", last.ErrorMessage)
			case agent.StopReasonCanceled:
				return messages, context.Canceled
			}
			break
		}
	}
	return messages, nil
}
