package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestChatSendsOpenAICompatibleRequest(t *testing.T) {
	var gotAuth string
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != chatCompletionsPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {"role": "assistant", "content": "done"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5}
		}`))
	}))
	defer server.Close()

	provider, err := New(Config{
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Model:   "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := provider.Chat(context.Background(), model.ChatRequest{
		System: "system prompt",
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "hi"},
		},
		Tools: []model.ToolDefinition{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotReq.Model != "deepseek-v4-flash" {
		t.Fatalf("Model = %q", gotReq.Model)
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %d", len(gotReq.Messages))
	}
	if gotReq.Messages[0].Role != "system" || gotReq.Messages[0].Content != "system prompt" {
		t.Fatalf("system message = %#v", gotReq.Messages[0])
	}
	if gotReq.Messages[1].Role != "user" || gotReq.Messages[1].Content != "hi" {
		t.Fatalf("user message = %#v", gotReq.Messages[1])
	}
	if len(gotReq.Tools) != 1 || gotReq.Tools[0].Function.Name != "read_file" {
		t.Fatalf("tools = %#v", gotReq.Tools)
	}
	if gotReq.Temperature != 0.2 {
		t.Fatalf("temperature = %f", gotReq.Temperature)
	}
	if resp.Content != "done" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("Usage.TotalTokens = %d", resp.Usage.TotalTokens)
	}
}

func TestChatSendsToolMessages(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {"role": "assistant", "content": "done"},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Chat(context.Background(), model.ChatRequest{
		Messages: []model.Message{
			{
				Role:    model.RoleAssistant,
				Content: "reading",
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
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %d", len(gotReq.Messages))
	}
	assistant := gotReq.Messages[0]
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", assistant.ToolCalls)
	}
	if assistant.ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool call id = %q", assistant.ToolCalls[0].ID)
	}
	if assistant.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool call name = %q", assistant.ToolCalls[0].Function.Name)
	}
	if assistant.ToolCalls[0].Function.Arguments != `{"path":"README.md"}` {
		t.Fatalf("tool call arguments = %q", assistant.ToolCalls[0].Function.Arguments)
	}
	tool := gotReq.Messages[1]
	if tool.Role != "tool" || tool.ToolCallID != "call-1" || tool.Content != "content" {
		t.Fatalf("tool message = %#v", tool)
	}
}

func TestChatParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "",
					"tool_calls": [{
						"id": "call-1",
						"type": "function",
						"function": {
							"name": "read_file",
							"arguments": "{\"path\":\"README.md\"}"
						}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	resp, err := provider.Chat(context.Background(), model.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
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
}

func TestChatReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Chat(context.Background(), model.ChatRequest{}); err == nil {
		t.Fatal("Chat() error = nil")
	}
}

func TestChatRejectsNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Chat(context.Background(), model.ChatRequest{}); err == nil {
		t.Fatal("Chat() error = nil")
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
		Model:   "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
