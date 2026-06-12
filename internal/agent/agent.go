package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

const defaultMaxSteps = 8

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
	Observer        Observer
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
		observer:        config.Observer,
	}, nil
}

// RunTurn 执行一次用户输入到最终 assistant 回复的循环。
func (a *Agent) RunTurn(ctx context.Context, prompt string) (string, error) {
	a.transcript.Append(model.Message{
		Role:    model.RoleUser,
		Content: prompt,
	})
	a.emit(Event{
		Type:    EventTurnStarted,
		Content: prompt,
	})

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
			a.emit(Event{
				Type: EventTurnFinished,
				Step: step,
				Err:  err,
			})
			return "", err
		}

		a.transcript.Append(model.Message{
			Role:             model.RoleAssistant,
			Content:          resp.Content,
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
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
				result = toolErrorResult(result, err)
			}
			a.transcript.Append(model.Message{
				Role:       model.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
			a.emit(Event{
				Type:       EventToolFinished,
				Step:       step,
				ToolCall:   call,
				ToolResult: result,
				ToolError:  toolError,
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
