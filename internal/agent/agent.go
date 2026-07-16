// Package agent implements the Atlas headless turn loop.
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
	// maxOverflowCompactions limits the number of auto-compactions triggered by context-overflow within a single turn.
	// If overflow persists after compaction, the conversation is unrecoverable and the error is returned directly.
	maxOverflowCompactions = 1
)

// Config holds the dependencies and runtime parameters for creating an Agent.
type Config struct {
	Provider    model.Provider
	Tools       *tool.Registry
	Transcript  *transcript.Transcript
	System      string
	MaxSteps    int
	MaxTokens   int
	Temperature float64
	// ReasoningEffort controls the thinking depth for models that support this parameter.
	ReasoningEffort string
	// Compactor is the compaction callback injected by the runtime on context-overflow.
	// When nil, the agent does not perform overflow recovery and returns the provider error directly.
	Compactor func(ctx context.Context) error
	// OnAppend is called after each transcript.Append, used for real-time persistence.
	// When nil, no action is taken.
	OnAppend func(msg model.Message)
	Observer Observer
}

// Agent wires together the model, tools, and transcript to execute a headless turn.
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

// New creates an Agent.
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

// appendMessage appends a message to the transcript and triggers the OnAppend callback.
func (a *Agent) appendMessage(msg model.Message) {
	a.transcript.Append(msg)
	if a.onAppend != nil {
		a.onAppend(msg)
	}
}

// RunTurn executes a loop from plain-text user input to a final assistant reply.
func (a *Agent) RunTurn(ctx context.Context, prompt string) (string, error) {
	return a.RunTurnParts(ctx, []model.ContentPart{{Type: model.ContentPartText, Text: prompt}})
}

// RunTurnParts executes a loop from structured user input to a final assistant reply.
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
			// Context-overflow recovery: when the provider returns an overflow error and a compactor is injected,
			// trigger one auto-compaction and retry the current step. Limited to at most maxOverflowCompactions times.
			if compact.IsContextOverflow(err) && a.compactor != nil && overflowCompactions < maxOverflowCompactions {
				if compactErr := a.compactor(ctx); compactErr == nil {
					overflowCompactions++
					step-- // retry current step
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
			result.Metadata.Error = toolError
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

// normalizeContentParts fills in default image detail and copies the input slice.
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
