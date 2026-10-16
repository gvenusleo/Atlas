package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
)

// saveTimeout 限制单条消息的保存耗时；保存不受运行取消影响。
const saveTimeout = 5 * time.Second

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
func (s *Session) Run(
	ctx context.Context, message agent.UserMessage, emit func(agent.Event),
) ([]agent.Message, error) {
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}
	if err := agent.ValidateMessage(message); err != nil {
		return nil, err
	}
	message = cloneMessage(message).(agent.UserMessage)
	return s.execute(ctx, emit, func(
		ctx context.Context, rc *agent.LoopContext, emit func(agent.Event),
	) []agent.Message {
		return agent.Run(ctx, []agent.Message{message}, rc, emit)
	})
}

// Compact 把较早的对话压缩成摘要，和 Run 一样独占会话；没有可压缩的内容时返回
// ErrNothingToCompact。
func (s *Session) Compact(ctx context.Context, emit func(agent.Event)) ([]agent.Message, error) {
	messages, err := s.execute(ctx, emit, agent.Compact)
	if err == nil && len(messages) == 0 {
		err = ErrNothingToCompact
	}
	return messages, err
}

// execute 独占会话并运行 drive，逐条保存它产生的消息；Run 与 Compact 共用。
func (s *Session) execute(
	ctx context.Context, emit func(agent.Event),
	drive func(context.Context, *agent.LoopContext, func(agent.Event)) []agent.Message,
) ([]agent.Message, error) {
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
	var saveErr error
	loopCompleted := false
	defer func() {
		if !loopCompleted {
			s.invalidate()
		}
	}()
	messages := drive(runCtx, &s.loop, func(event agent.Event) {
		if event.Type == agent.EventTypeMessageEnd && saveErr == nil {
			if saveErr = s.save(ctx, event.Message); saveErr != nil {
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
	return messages, lastError(messages)
}

// save 保存一条消息并推进已知序号；保存不受运行取消影响，但有独立超时。
func (s *Session) save(ctx context.Context, message agent.Message) error {
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer stop()
	seq, err := s.manager.store.AppendMessage(ctx, s.id, s.seq, message)
	if err != nil {
		return err
	}
	s.seq = seq
	if user, ok := message.(agent.UserMessage); ok {
		return s.autoName(ctx, user)
	}
	return nil
}

// lastError 按最后一条助手消息的结束原因返回运行错误。
func lastError(messages []agent.Message) error {
	for i := len(messages) - 1; i >= 0; i-- {
		last, ok := messages[i].(agent.AssistantMessage)
		if !ok {
			continue
		}
		switch last.StopReason {
		case agent.StopReasonError:
			return fmt.Errorf("模型调用失败: %s", last.ErrorMessage)
		case agent.StopReasonCanceled:
			return context.Canceled
		}
		return nil
	}
	return nil
}
