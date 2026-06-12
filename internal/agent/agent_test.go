package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

type fakeProvider struct {
	responses []model.ChatResponse
	events    [][]model.StreamEvent
	err       error
	requests  []model.ChatRequest
}

func (p *fakeProvider) Stream(_ context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return model.ChatResponse{}, p.err
	}
	if len(p.responses) == 0 {
		return model.ChatResponse{}, errors.New("unexpected stream call")
	}
	if len(p.events) > 0 {
		for _, event := range p.events[0] {
			if err := emit(event); err != nil {
				return model.ChatResponse{}, err
			}
		}
		p.events = p.events[1:]
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

type fakeTool struct {
	definition model.ToolDefinition
	result     string
	err        error
}

func (t fakeTool) Definition() model.ToolDefinition {
	return t.definition
}

func (t fakeTool) Run(_ context.Context, _ string) (string, error) {
	return t.result, t.err
}

func TestRunTurnTextResponse(t *testing.T) {
	provider := &fakeProvider{
		responses: []model.ChatResponse{{Content: "hello"}},
	}
	agent, err := New(Config{
		Provider:        provider,
		System:          "system",
		MaxTokens:       384000,
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := agent.RunTurn(context.Background(), "hi")
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("RunTurn() = %q, want %q", got, "hello")
	}
	if len(provider.requests) != 1 {
		t.Fatalf("stream calls = %d, want 1", len(provider.requests))
	}
	if provider.requests[0].System != "system" {
		t.Fatalf("request system = %q, want %q", provider.requests[0].System, "system")
	}
	if provider.requests[0].MaxTokens != 384000 {
		t.Fatalf("max tokens = %d", provider.requests[0].MaxTokens)
	}
	if provider.requests[0].ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", provider.requests[0].ReasoningEffort)
	}
}

func TestRunTurnToolThenFinalResponse(t *testing.T) {
	registry, err := tool.NewRegistry(fakeTool{
		definition: model.ToolDefinition{Name: "fake"},
		result:     "tool result",
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ReasoningContent: "need tool", ToolCalls: []model.ToolCall{{ID: "call_1", Name: "fake", Arguments: `{}`}}},
			{Content: "done"},
		},
	}
	agent, err := New(Config{Provider: provider, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := agent.RunTurn(context.Background(), "use tool")
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if got != "done" {
		t.Fatalf("RunTurn() = %q, want %q", got, "done")
	}
	if len(provider.requests) != 2 {
		t.Fatalf("stream calls = %d, want 2", len(provider.requests))
	}
	lastMessages := provider.requests[1].Messages
	last := lastMessages[len(lastMessages)-1]
	if last.Role != model.RoleTool || last.Content != "tool result" || last.ToolCallID != "call_1" {
		t.Fatalf("last message = %#v", last)
	}
	if lastMessages[1].ReasoningContent != "need tool" {
		t.Fatalf("assistant reasoning content = %q", lastMessages[1].ReasoningContent)
	}
}

func TestRunTurnEmitsEventsInOrder(t *testing.T) {
	registry, err := tool.NewRegistry(fakeTool{
		definition: model.ToolDefinition{Name: "fake"},
		result:     "tool result",
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "fake", Arguments: `{}`}}},
			{Content: "done"},
		},
	}
	var events []Event
	agent, err := New(Config{
		Provider: provider,
		Tools:    registry,
		Observer: func(event Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := agent.RunTurn(context.Background(), "use tool")
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if got != "done" {
		t.Fatalf("RunTurn() = %q", got)
	}

	wantTypes := []EventType{
		EventTurnStarted,
		EventModelResponse,
		EventToolStarted,
		EventToolFinished,
		EventModelResponse,
		EventTurnFinished,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("events = %#v", events)
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Fatalf("event %d type = %q, want %q", i, events[i].Type, want)
		}
	}
	if events[2].ToolCall.Name != "fake" {
		t.Fatalf("tool started = %#v", events[2])
	}
	if events[3].ToolResult != "tool result" || events[3].ToolError {
		t.Fatalf("tool finished = %#v", events[3])
	}
	if events[5].Content != "done" {
		t.Fatalf("turn finished = %#v", events[5])
	}
}

func TestRunTurnEmitsModelDeltas(t *testing.T) {
	provider := &fakeProvider{
		events: [][]model.StreamEvent{{
			{Type: model.StreamReasoningDelta, Delta: "think"},
			{Type: model.StreamTextDelta, Delta: "hel"},
			{Type: model.StreamTextDelta, Delta: "lo"},
		}},
		responses: []model.ChatResponse{{Content: "hello"}},
	}
	var events []Event
	agent, err := New(Config{
		Provider: provider,
		Observer: func(event Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := agent.RunTurn(context.Background(), "hi")
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("RunTurn() = %q, want %q", got, "hello")
	}
	var deltas []string
	var reasoningDeltas []string
	for _, event := range events {
		if event.Type == EventModelDelta {
			deltas = append(deltas, event.Content)
		}
		if event.Type == EventModelReasoningDelta {
			reasoningDeltas = append(reasoningDeltas, event.Content)
		}
	}
	if len(deltas) != 2 || deltas[0] != "hel" || deltas[1] != "lo" {
		t.Fatalf("deltas = %#v", deltas)
	}
	if len(reasoningDeltas) != 1 || reasoningDeltas[0] != "think" {
		t.Fatalf("reasoning deltas = %#v", reasoningDeltas)
	}
}

func TestRunTurnEmitsToolErrorEvent(t *testing.T) {
	registry, err := tool.NewRegistry(fakeTool{
		definition: model.ToolDefinition{Name: "fake"},
		err:        errors.New("tool failed"),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "fake", Arguments: `{}`}}},
			{Content: "done"},
		},
	}
	var events []Event
	agent, err := New(Config{
		Provider: provider,
		Tools:    registry,
		Observer: func(event Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "use tool"); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	var found bool
	for _, event := range events {
		if event.Type == EventToolFinished {
			found = true
			if !event.ToolError || event.ToolResult != "tool failed" {
				t.Fatalf("tool event = %#v", event)
			}
		}
	}
	if !found {
		t.Fatalf("events = %#v", events)
	}
}

func TestRunTurnKeepsToolResultWhenToolReturnsError(t *testing.T) {
	registry, err := tool.NewRegistry(fakeTool{
		definition: model.ToolDefinition{Name: "fake"},
		result:     "partial output",
		err:        errors.New("tool failed"),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "fake", Arguments: `{}`}}},
			{Content: "done"},
		},
	}
	var events []Event
	agent, err := New(Config{
		Provider: provider,
		Tools:    registry,
		Observer: func(event Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "use tool"); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	messages := provider.requests[1].Messages
	last := messages[len(messages)-1]
	if last.Content != "partial output\ntool failed" {
		t.Fatalf("tool result content = %q, want partial output and error", last.Content)
	}
	for _, event := range events {
		if event.Type == EventToolFinished && (!event.ToolError || event.ToolResult != "partial output\ntool failed") {
			t.Fatalf("tool event = %#v", event)
		}
	}
}

func TestRunTurnUnknownToolIsVisibleToModel(t *testing.T) {
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "missing", Arguments: `{}`}}},
			{Content: "recovered"},
		},
	}
	agent, err := New(Config{Provider: provider})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "use missing"); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	messages := provider.requests[1].Messages
	last := messages[len(messages)-1]
	if last.Role != model.RoleTool || last.ToolCallID != "call_1" {
		t.Fatalf("last message = %#v", last)
	}
	if last.Content == "" {
		t.Fatal("tool error content is empty")
	}
}

func TestRunTurnToolResultOrder(t *testing.T) {
	registry, err := tool.NewRegistry(
		fakeTool{definition: model.ToolDefinition{Name: "first"}, result: "one"},
		fakeTool{definition: model.ToolDefinition{Name: "second"}, result: "two"},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{
				{ID: "a", Name: "first", Arguments: `{}`},
				{ID: "b", Name: "second", Arguments: `{}`},
			}},
			{Content: "done"},
		},
	}
	agent, err := New(Config{Provider: provider, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "use tools"); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	messages := provider.requests[1].Messages
	if messages[len(messages)-2].Content != "one" || messages[len(messages)-1].Content != "two" {
		t.Fatalf("tool result order = %#v", messages[len(messages)-2:])
	}
}

func TestRunTurnMaxSteps(t *testing.T) {
	provider := &fakeProvider{
		responses: []model.ChatResponse{
			{ToolCalls: []model.ToolCall{{ID: "call_1", Name: "missing", Arguments: `{}`}}},
		},
	}
	agent, err := New(Config{Provider: provider, MaxSteps: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "loop"); err == nil {
		t.Fatal("RunTurn() error = nil, want max steps error")
	}
}

func TestRunTurnProviderError(t *testing.T) {
	want := errors.New("provider failed")
	var events []Event
	agent, err := New(Config{
		Provider: &fakeProvider{err: want},
		Observer: func(event Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = agent.RunTurn(context.Background(), "hi")
	if !errors.Is(err, want) {
		t.Fatalf("RunTurn() error = %v, want %v", err, want)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[1].Type != EventTurnFinished || !errors.Is(events[1].Err, want) {
		t.Fatalf("finish event = %#v", events[1])
	}
}

func TestRunTurnContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	agent, err := New(Config{Provider: &cancelProvider{}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = agent.RunTurn(ctx, "hi")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunTurn() error = %v, want %v", err, context.Canceled)
	}
}

func TestNewRequiresProvider(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New() error = nil, want provider error")
	}
}

func TestNewUsesProvidedTranscript(t *testing.T) {
	trans := transcript.New()
	provider := &fakeProvider{
		responses: []model.ChatResponse{{Content: "hello"}},
	}
	agent, err := New(Config{Provider: provider, Transcript: trans})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := agent.RunTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(trans.Messages()) != 2 {
		t.Fatalf("transcript messages = %d, want 2", len(trans.Messages()))
	}
}

type cancelProvider struct{}

func (cancelProvider) Stream(ctx context.Context, _ model.ChatRequest, _ func(model.StreamEvent) error) (model.ChatResponse, error) {
	return model.ChatResponse{}, ctx.Err()
}
