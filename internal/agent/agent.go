// Package agent 实现 Atlas 的 headless turn loop。
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/compact"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

const (
	defaultMaxSteps = 20
	// maxOverflowCompactions 限制单个 turn 内因 context-overflow 触发的自动压缩次数。
	// 压缩后仍 overflow 说明对话已无法恢复，直接返回错误。
	maxOverflowCompactions = 1
)

// Config 是创建 Agent 所需的依赖和运行参数。
type Config struct {
	Provider    model.Provider
	Tools       *tool.Registry
	Transcript  *transcript.Transcript
	System      string
	MaxSteps    int
	MaxTokens   int
	Temperature float64
	// ReasoningEffort 控制支持该参数的模型的思考深度。
	ReasoningEffort string
	// Compactor 在 context-overflow 时由 runtime 注入的压缩回调。
	// 为 nil 时 agent 不做 overflow 恢复，直接返回 provider 错误。
	Compactor func(ctx context.Context) error
	// OnAppend 在每次 transcript.Append 之后调用，用于实时持久化。
	// 为 nil 时不做任何操作。
	OnAppend func(msg model.Message)
	Observer Observer
}

// Agent 串联模型、工具和 transcript，执行一个 headless turn。
type Agent struct {
	provider        model.Provider
	tools           *tool.Registry
	transcript      *transcript.Transcript
	system          string
	maxSteps        int
	maxTokens       int
	temperature     float64
	reasoningEffort string
	compactor       func(ctx context.Context) error
	onAppend        func(model.Message)
	observer        Observer
}

// New 创建一个 Agent。
func New(config Config) (*Agent, error) {
	if config.Provider == nil {
		return nil, fmt.Errorf("agent provider is required")
	}
	tools := config.Tools
	if tools == nil {
		var err error
		tools, err = tool.NewRegistry()
		if err != nil {
			return nil, err
		}
	}
	trans := config.Transcript
	if trans == nil {
		trans = transcript.New()
	}
	maxSteps := config.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}

	return &Agent{
		provider:        config.Provider,
		tools:           tools,
		transcript:      trans,
		system:          config.System,
		maxSteps:        maxSteps,
		maxTokens:       config.MaxTokens,
		temperature:     config.Temperature,
		reasoningEffort: config.ReasoningEffort,
		compactor:       config.Compactor,
		onAppend:        config.OnAppend,
		observer:        config.Observer,
	}, nil
}

// appendMessage 将消息追加到 transcript 并触发 OnAppend 回调。
func (a *Agent) appendMessage(msg model.Message) {
	a.transcript.Append(msg)
	if a.onAppend != nil {
		a.onAppend(msg)
	}
}

// RunTurn 执行一次纯文本用户输入到最终 assistant 回复的循环。
func (a *Agent) RunTurn(ctx context.Context, prompt string) (string, error) {
	return a.RunTurnParts(ctx, []model.ContentPart{{Type: model.ContentPartText, Text: prompt}})
}

// RunTurnParts 执行一次结构化用户输入到最终 assistant 回复的循环。
func (a *Agent) RunTurnParts(ctx context.Context, parts []model.ContentPart) (string, error) {
	content := model.TextFromParts(parts)
	a.appendMessage(model.Message{
		Role:    model.RoleUser,
		Content: content,
		Parts:   normalizeContentParts(parts),
	})
	a.emit(Event{
		Type:    EventTurnStarted,
		Content: content,
	})

	overflowCompactions := 0
	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.provider.Stream(ctx, model.ChatRequest{
			System:          a.system,
			Messages:        a.transcript.Messages(),
			Tools:           a.tools.Definitions(),
			MaxTokens:       a.maxTokens,
			Temperature:     a.temperature,
			ReasoningEffort: a.reasoningEffort,
		}, func(event model.StreamEvent) error {
			switch {
			case event.Type == model.StreamTextDelta && event.Delta != "":
				a.emit(Event{
					Type:    EventModelDelta,
					Step:    step,
					Content: event.Delta,
				})
			case event.Type == model.StreamReasoningDelta && event.Delta != "":
				a.emit(Event{
					Type:    EventModelReasoningDelta,
					Step:    step,
					Content: event.Delta,
				})
			}
			return nil
		})
		if err != nil {
			// Context-overflow 恢复：当 provider 返回超限错误且注入了 compactor 时，
			// 触发一次自动压缩后重试当前 step。限制最多 maxOverflowCompactions 次。
			if compact.IsContextOverflow(err) && a.compactor != nil && overflowCompactions < maxOverflowCompactions {
				if compactErr := a.compactor(ctx); compactErr == nil {
					overflowCompactions++
					step-- // 重试当前 step
					continue
				}
			}
			a.emit(Event{
				Type: EventTurnFinished,
				Step: step,
				Err:  err,
			})
			return "", err
		}

		a.appendMessage(model.Message{
			Role:             model.RoleAssistant,
			Content:          resp.Content,
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
			Usage:            resp.Usage,
			ProviderItems:    resp.ProviderItems,
		})
		a.emit(Event{
			Type:    EventModelResponse,
			Step:    step,
			Content: resp.Content,
		})
		if len(resp.ToolCalls) == 0 {
			a.emit(Event{
				Type:    EventTurnFinished,
				Step:    step,
				Content: resp.Content,
			})
			return resp.Content, nil
		}

		for _, call := range resp.ToolCalls {
			a.emit(Event{
				Type:     EventToolStarted,
				Step:     step,
				ToolCall: call,
			})
			result, err := a.tools.Run(ctx, call)
			toolError := err != nil
			if err != nil {
				result.Content = toolErrorResult(result.Content, err)
			}
			a.appendMessage(model.Message{
				Role:         model.RoleTool,
				Content:      result.Content,
				ToolCallID:   call.ID,
				ToolMetadata: result.Metadata,
			})
			a.emit(Event{
				Type:         EventToolFinished,
				Step:         step,
				ToolCall:     call,
				ToolResult:   result.Content,
				ToolMetadata: result.Metadata,
				ToolError:    toolError,
			})
		}
	}

	err := fmt.Errorf("agent max steps exceeded: %d", a.maxSteps)
	a.emit(Event{
		Type: EventTurnFinished,
		Step: a.maxSteps,
		Err:  err,
	})
	return "", err
}

// normalizeContentParts 补齐图片默认 detail，并复制输入切片。
func normalizeContentParts(parts []model.ContentPart) []model.ContentPart {
	normalized := make([]model.ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type == "" {
			part.Type = model.ContentPartText
		}
		if part.Type == model.ContentPartImage && part.Detail == "" {
			part.Detail = model.ImageDetailAuto
		}
		normalized = append(normalized, part)
	}
	return normalized
}

func (a *Agent) emit(event Event) {
	if a.observer != nil {
		a.observer(event)
	}
}

func toolErrorResult(result string, err error) string {
	if result == "" {
		return err.Error()
	}
	if strings.Contains(result, err.Error()) {
		return result
	}
	return result + "\n" + err.Error()
}
