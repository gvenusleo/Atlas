package responses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestStreamSendsResponsesRequest(t *testing.T) {
	var gotAuth string
	var gotReq responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != responsesPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w,
			`{"type":"response.output_text.delta","delta":"done"}`,
			`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		)
	}))
	defer server.Close()

	provider, err := New(Config{
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Model:   "gpt-5",
	})
	if err != nil {
		t.Fatal(err)
	}

	var deltas []string
	resp, err := provider.Stream(context.Background(), model.ChatRequest{
		System: "system prompt",
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "hi"},
		},
		Tools: []model.ToolDefinition{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
		MaxTokens:       4096,
		Temperature:     0.2,
		ReasoningEffort: "high",
	}, func(event model.StreamEvent) error {
		deltas = append(deltas, event.Delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotReq.Model != "gpt-5" {
		t.Fatalf("Model = %q", gotReq.Model)
	}
	if !gotReq.Stream {
		t.Fatal("stream = false")
	}
	if gotReq.Instructions != "system prompt" {
		t.Fatalf("Instructions = %q", gotReq.Instructions)
	}
	if len(gotReq.Input) != 1 || gotReq.Input[0].Role != "user" || gotReq.Input[0].Content != "hi" {
		t.Fatalf("input = %#v", gotReq.Input)
	}
	if len(gotReq.Tools) != 1 || gotReq.Tools[0].Name != "read_file" || gotReq.Tools[0].Type != "function" {
		t.Fatalf("tools = %#v", gotReq.Tools)
	}
	if gotReq.Tools[0].Strict {
		t.Fatalf("tool strict = true")
	}
	if gotReq.MaxOutputTokens != 4096 {
		t.Fatalf("max output tokens = %d", gotReq.MaxOutputTokens)
	}
	if gotReq.Temperature != 0.2 {
		t.Fatalf("temperature = %f", gotReq.Temperature)
	}
	if gotReq.Reasoning == nil || gotReq.Reasoning.Effort != "high" {
		t.Fatalf("reasoning = %#v", gotReq.Reasoning)
	}
	if gotReq.Text != nil {
		t.Fatalf("text = %#v", gotReq.Text)
	}
	if resp.Content != "done" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if len(deltas) != 1 || deltas[0] != "done" {
		t.Fatalf("deltas = %#v", deltas)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("Usage.TotalTokens = %d", resp.Usage.TotalTokens)
	}
}

func TestStreamSendsFunctionCallOutput(t *testing.T) {
	var gotReq responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{
			{
				Role:             model.RoleAssistant,
				Content:          "reading",
				ReasoningContent: "need file",
				ToolCalls: []model.ToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				}},
			},
			{
				Role:       model.RoleTool,
				Content:    "content",
				ToolCallID: "call-1",
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(gotReq.Input) != 3 {
		t.Fatalf("input = %#v", gotReq.Input)
	}
	call := gotReq.Input[1]
	if call.Type != "function_call" || call.CallID != "call-1" || call.Name != "read_file" || call.Arguments != `{"path":"README.md"}` {
		t.Fatalf("function call input = %#v", call)
	}
	output := gotReq.Input[2]
	if output.Type != "function_call_output" || output.CallID != "call-1" || output.Output != "content" {
		t.Fatalf("function call output = %#v", output)
	}
}

func TestStreamSendsProviderItems(t *testing.T) {
	var gotReq struct {
		Input []json.RawMessage `json:"input"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"type":"response.completed","response":{"status":"completed"}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{
			{
				Role: model.RoleAssistant,
				ProviderItems: []model.ProviderItem{{
					Type: "responses",
					JSON: `{"type":"reasoning","id":"rs_1","summary":[]}`,
				}},
			},
			{Role: model.RoleTool, ToolCallID: "call-1", Content: "ok"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(gotReq.Input) != 2 {
		t.Fatalf("input = %#v", gotReq.Input)
	}
	if string(gotReq.Input[0]) != `{"type":"reasoning","id":"rs_1","summary":[]}` {
		t.Fatalf("raw input = %s", gotReq.Input[0])
	}
	var output inputItem
	if err := json.Unmarshal(gotReq.Input[1], &output); err != nil {
		t.Fatal(err)
	}
	if output.Type != "function_call_output" || output.Output != "ok" {
		t.Fatalf("tool output = %#v", output)
	}
}

func TestStreamSkipsReasoningOnlyAssistantMessage(t *testing.T) {
	var gotReq responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"type":"response.completed","response":{"status":"completed"}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{
			{
				Role:             model.RoleAssistant,
				ReasoningContent: "need file",
				ToolCalls: []model.ToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				}},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(gotReq.Input) != 1 {
		t.Fatalf("input = %#v", gotReq.Input)
	}
	if gotReq.Input[0].Type != "function_call" || gotReq.Input[0].CallID != "call-1" {
		t.Fatalf("input = %#v", gotReq.Input)
	}
}

func TestStreamSendsJSONResponseFormat(t *testing.T) {
	var gotReq responsesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"type":"response.output_text.delta","delta":"{}"}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages:       []model.Message{{Role: model.RoleUser, Content: "json"}},
		ResponseFormat: model.ResponseFormatJSONObject,
	}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotReq.Text == nil || gotReq.Text.Format.Type != "json_object" {
		t.Fatalf("text = %#v", gotReq.Text)
	}
}

func TestStreamParsesReasoningDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"type":"response.reasoning_summary_text.delta","delta":"think "}`,
			`{"type":"response.reasoning_summary_text.delta","delta":"more"}`,
			`{"type":"response.output_text.delta","delta":"done"}`,
			`{"type":"response.completed","response":{"status":"completed"}}`,
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	var events []model.StreamEvent
	resp, err := provider.Stream(context.Background(), model.ChatRequest{}, func(event model.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp.ReasoningContent != "think more" {
		t.Fatalf("ReasoningContent = %q", resp.ReasoningContent)
	}
	if resp.Content != "done" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != model.StreamReasoningDelta || events[0].Delta != "think " {
		t.Fatalf("event 0 = %#v", events[0])
	}
	if events[1].Type != model.StreamReasoningDelta || events[1].Delta != "more" {
		t.Fatalf("event 1 = %#v", events[1])
	}
	if events[2].Type != model.StreamTextDelta || events[2].Delta != "done" {
		t.Fatalf("event 2 = %#v", events[2])
	}
}

func TestStreamParsesFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call-1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`,
			`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	resp, err := provider.Stream(context.Background(), model.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp.StopReason != model.StopToolUse {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call-1" || resp.ToolCalls[0].Name != "read_file" {
		t.Fatalf("ToolCall = %#v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Arguments != `{"path":"README.md"}` {
		t.Fatalf("Arguments = %q", resp.ToolCalls[0].Arguments)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("Usage.TotalTokens = %d", resp.Usage.TotalTokens)
	}
}

func TestStreamParsesCompletedOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	resp, err := provider.Stream(context.Background(), model.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}
}

func TestStreamStoresProviderItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"rs_1","summary":[]},{"type":"function_call","call_id":"call-1","name":"read_file","arguments":"{}"}]}}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	resp, err := provider.Stream(context.Background(), model.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(resp.ProviderItems) != 2 {
		t.Fatalf("ProviderItems = %#v", resp.ProviderItems)
	}
	if resp.ProviderItems[0].Type != "responses" || resp.ProviderItems[0].JSON == "" {
		t.Fatalf("ProviderItems[0] = %#v", resp.ProviderItems[0])
	}
	if resp.ProviderItems[0].JSON != `{"type":"reasoning","id":"rs_1","summary":[]}` {
		t.Fatalf("ProviderItems[0].JSON = %s", resp.ProviderItems[0].JSON)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call-1" {
		t.Fatalf("ToolCalls = %#v", resp.ToolCalls)
	}
}

func TestStreamReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{}, nil); err == nil {
		t.Fatal("Stream() error = nil")
	}
}

func TestStreamRejectsNoEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{}, nil); err == nil {
		t.Fatal("Stream() error = nil")
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "missing base url", config: Config{APIKey: "sk-test", Model: "m"}},
		{name: "invalid base url", config: Config{BaseURL: ":", APIKey: "sk-test", Model: "m"}},
		{name: "missing api key", config: Config{BaseURL: "https://api.example.com", Model: "m"}},
		{name: "missing model", config: Config{BaseURL: "https://api.example.com", APIKey: "sk-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.config); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func newTestProvider(t *testing.T, baseURL string) *Provider {
	t.Helper()

	provider, err := New(Config{
		BaseURL: baseURL,
		APIKey:  "sk-test",
		Model:   "gpt-5",
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func writeSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		_, _ = w.Write([]byte("data: " + event + "\n\n"))
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}
