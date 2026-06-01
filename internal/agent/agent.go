package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/skills"
	"github.com/liuyuxin/atlas/internal/storage"
	"github.com/liuyuxin/atlas/internal/tool"
)

const defaultMaxSteps = 12

// Agent owns one serial turn loop over a durable session store.
type Agent struct {
	store    storage.Store
	provider model.Provider
	tools    *tool.Runtime
	prompts  prompt.Builder
	config   Config
	mu       sync.Mutex
}

// New constructs an agent from its core dependencies.
func New(store storage.Store, provider model.Provider, tools *tool.Runtime, config Config) *Agent {
	if config.MaxSteps <= 0 {
		config.MaxSteps = defaultMaxSteps
	}
	if strings.TrimSpace(config.Workdir) == "" {
		config.Workdir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(config.Workdir); err == nil {
		config.Workdir = abs
	}
	if len(config.SkillRoots) == 0 {
		config.SkillRoots = skills.DefaultRoots(config.Workdir)
	}
	return &Agent{
		store:    store,
		provider: provider,
		tools:    tools,
		prompts:  prompt.Builder{},
		config:   config,
	}
}

// CreateSession creates a durable local session.
func (a *Agent) CreateSession(ctx context.Context, title string) (storage.Session, error) {
	select {
	case <-ctx.Done():
		return storage.Session{}, ctx.Err()
	default:
	}
	now := time.Now()
	session := storage.Session{
		ID:        newID("ses"),
		Title:     fallbackTitle(title),
		Workdir:   a.config.Workdir,
		Model:     a.config.Model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.store.CreateSession(session); err != nil {
		return storage.Session{}, err
	}
	return session, nil
}

// RunTurn appends user input, runs the model/tool loop, and streams events.
func (a *Agent) RunTurn(ctx context.Context, sessionID string, userInput string) (<-chan Event, <-chan error) {
	events := make(chan Event, 32)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		if err := a.runTurn(ctx, sessionID, userInput, events); err != nil {
			emit(events, Event{Type: EventError, SessionID: sessionID, Text: err.Error(), Error: true})
			errs <- err
		}
	}()
	return events, errs
}

// runTurn executes one complete user turn, including follow-up tool steps.
func (a *Agent) runTurn(ctx context.Context, sessionID string, userInput string, events chan<- Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(userInput) == "" {
		return fmt.Errorf("user input is empty")
	}
	session, err := a.store.GetSession(sessionID)
	if err != nil {
		return err
	}
	if err := a.store.AddMessage(storage.Message{
		SessionID: session.ID,
		Role:      string(model.RoleUser),
		Content:   userInput,
	}); err != nil {
		return err
	}

	emit(events, Event{Type: EventTurnStarted, SessionID: session.ID})
	for step := 0; step < a.config.MaxSteps; step++ {
		result, err := a.runModelStep(ctx, session, userInput, events)
		if err != nil {
			return err
		}
		if err := a.persistAssistant(session.ID, result); err != nil {
			return err
		}
		if len(result.ToolCalls) == 0 {
			emit(events, Event{Type: EventTurnFinished, SessionID: session.ID})
			return nil
		}
		for _, call := range result.ToolCalls {
			if err := a.executeToolCall(ctx, session.ID, call, events); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("max steps reached (%d)", a.config.MaxSteps)
}

// runModelStep performs one provider call and assembles streamed output.
func (a *Agent) runModelStep(ctx context.Context, session storage.Session, userInput string, events chan<- Event) (model.AssistantResult, error) {
	messages, err := a.store.Messages(session.ID, 0)
	if err != nil {
		return model.AssistantResult{}, err
	}
	extra := a.skillPromptContext(userInput)
	system, modelMessages := a.prompts.Build(session, messages, extra)
	req := model.ChatRequest{
		Model:       session.Model,
		System:      system,
		Messages:    modelMessages,
		Tools:       a.modelToolDefinitions(),
		Temperature: a.config.Temperature,
	}
	stream, errs := a.provider.StreamChat(ctx, req)
	var result model.AssistantResult
	for stream != nil || errs != nil {
		select {
		case <-ctx.Done():
			return model.AssistantResult{}, ctx.Err()
		case event, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			if event.TextDelta != "" {
				result.Content += event.TextDelta
				emit(events, Event{Type: EventTextDelta, SessionID: session.ID, Text: event.TextDelta})
			}
			if event.ToolCall != nil {
				result.ToolCalls = append(result.ToolCalls, *event.ToolCall)
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return model.AssistantResult{}, err
			}
		}
	}
	return result, nil
}

// skillPromptContext builds transient skills context for one user turn.
func (a *Agent) skillPromptContext(userInput string) prompt.ExtraContext {
	catalog := skills.Load(a.config.SkillRoots)
	context := skills.BuildPromptContext(catalog, userInput)
	return prompt.ExtraContext{
		AvailableSkills: context.Available,
		SkillBlocks:     skills.RenderInjections(context.Injected, context.Warnings),
	}
}

// persistAssistant writes assistant text and structured tool calls to storage.
func (a *Agent) persistAssistant(sessionID string, result model.AssistantResult) error {
	toolCalls := ""
	if len(result.ToolCalls) > 0 {
		data, err := json.Marshal(result.ToolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCalls = string(data)
	}
	return a.store.AddMessage(storage.Message{
		SessionID: sessionID,
		Role:      string(model.RoleAssistant),
		Content:   result.Content,
		ToolCalls: toolCalls,
	})
}

// executeToolCall runs a local tool and persists the result message.
func (a *Agent) executeToolCall(ctx context.Context, sessionID string, call model.ToolCall, events chan<- Event) error {
	emit(events, Event{Type: EventToolStarted, SessionID: sessionID, ToolName: call.Name, ToolCallID: call.ID})
	result := a.tools.Execute(ctx, call.Name, call.Arguments)
	if err := a.store.AddMessage(storage.Message{
		SessionID:  sessionID,
		Role:       string(model.RoleTool),
		Content:    result.Content,
		ToolCallID: call.ID,
	}); err != nil {
		return err
	}
	emit(events, Event{
		Type:       EventToolFinished,
		SessionID:  sessionID,
		ToolName:   call.Name,
		ToolCallID: call.ID,
		Text:       result.Content,
		Error:      result.Error,
	})
	return nil
}

// modelToolDefinitions adapts tool runtime definitions to model definitions.
func (a *Agent) modelToolDefinitions() []model.ToolDefinition {
	defs := a.tools.Definitions()
	out := make([]model.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		out = append(out, model.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}
	return out
}

// emit timestamps an event before sending it to consumers.
func emit(events chan<- Event, event Event) {
	event.CreatedAt = time.Now()
	events <- event
}

// fallbackTitle normalizes empty session titles.
func fallbackTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "New session"
	}
	return title
}

// newID creates a compact random identifier with a stable prefix.
func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}
