package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/version"
)

func TestStreamSendsChatCompletionsRequest(t *testing.T) {
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
		writeSSE(w,
			`{"choices":[{"delta":{"content":"done"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		)
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

	var deltas []string
	resp, err := provider.Stream(context.Background(), model.ChatRequest{
		System: "system prompt",
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "hi"},
		},
		Tools: []model.ToolDefinition{{
			Name:        "test_tool",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
		MaxTokens:       384000,
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
	if gotReq.Model != "deepseek-v4-flash" {
		t.Fatalf("Model = %q", gotReq.Model)
	}
	if !gotReq.Stream {
		t.Fatal("stream = false")
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %d", len(gotReq.Messages))
	}
	if gotReq.Messages[0].Role != "system" {
		t.Fatalf("system message = %#v", gotReq.Messages[0])
	}
	assertContentParts(t, gotReq.Messages[0].Content, []apiContentPart{{Type: "text", Text: "system prompt"}})
	if gotReq.Messages[1].Role != "user" {
		t.Fatalf("user message = %#v", gotReq.Messages[1])
	}
	assertContentParts(t, gotReq.Messages[1].Content, []apiContentPart{{Type: "text", Text: "hi"}})
	if len(gotReq.Tools) != 1 || gotReq.Tools[0].Function.Name != "test_tool" {
		t.Fatalf("tools = %#v", gotReq.Tools)
	}
	if gotReq.Temperature != 0.2 {
		t.Fatalf("temperature = %f", gotReq.Temperature)
	}
	if gotReq.MaxTokens != 384000 {
		t.Fatalf("max tokens = %d", gotReq.MaxTokens)
	}
	if gotReq.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", gotReq.ReasoningEffort)
	}
	if gotReq.ResponseFormat != nil {
		t.Fatalf("response format = %#v", gotReq.ResponseFormat)
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

func TestStreamSendsPromptCacheKeyWhenEnabled(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider, err := New(Config{
		BaseURL:            server.URL,
		APIKey:             "sk-test",
		Model:              "deepseek-v4-flash",
		PromptCacheEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Stream(context.Background(), model.ChatRequest{SessionID: "sess-1"}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotReq.PromptCacheKey != "sess-1" {
		t.Fatalf("prompt cache key = %q", gotReq.PromptCacheKey)
	}
}

func TestStreamOmitsPromptCacheKeyByDefault(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{SessionID: "sess-1"}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, ok := raw["prompt_cache_key"]; ok {
		t.Fatalf("prompt_cache_key present: %#v", raw)
	}
}

// TestStreamSendsIncludeUsage verifies that streaming requests always include stream_options.include_usage.
func TestStreamSendsIncludeUsage(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotReq.StreamOptions == nil || !gotReq.StreamOptions.IncludeUsage {
		t.Fatalf("stream_options.include_usage not set: %+v", gotReq.StreamOptions)
	}
}

func TestStreamSendsToolMessages(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`)
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
					Name:      "test_tool",
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
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %d", len(gotReq.Messages))
	}
	assistant := gotReq.Messages[0]
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", assistant.ToolCalls)
	}
	if assistant.ReasoningContent != "need file" {
		t.Fatalf("reasoning content = %q", assistant.ReasoningContent)
	}
	if assistant.ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool call id = %q", assistant.ToolCalls[0].ID)
	}
	if assistant.ToolCalls[0].Function.Name != "test_tool" {
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

func TestStreamParsesCacheUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"choices":[{"delta":{"content":"done"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":3,"total_tokens":23,"prompt_tokens_details":{"cached_tokens":12,"cache_creation_input_tokens":5}}}`,
		)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	resp, err := provider.Stream(context.Background(), model.ChatRequest{}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if resp.Usage.CacheReadInputTokens != 12 {
		t.Fatalf("CacheReadInputTokens = %d", resp.Usage.CacheReadInputTokens)
	}
	if resp.Usage.CacheWriteInputTokens != 5 {
		t.Fatalf("CacheWriteInputTokens = %d", resp.Usage.CacheWriteInputTokens)
	}
}

func TestStreamSendsImageContentPart(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: "describe",
			Parts: []model.ContentPart{
				{Type: model.ContentPartText, Text: "describe"},
				{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,aGVsbG8=", Detail: model.ImageDetailHigh},
			},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	assertContentParts(t, gotReq.Messages[0].Content, []apiContentPart{
		{Type: "text", Text: "describe"},
		{Type: "image_url", ImageURL: &apiImageURLContent{URL: "data:image/png;base64,aGVsbG8=", Detail: "high"}},
	})
}

func TestStreamSendsJSONResponseFormat(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"{}"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages:       []model.Message{{Role: model.RoleUser, Content: "json"}},
		ResponseFormat: model.ResponseFormatJSONObject,
	}, nil); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotReq.ResponseFormat == nil || gotReq.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", gotReq.ResponseFormat)
	}
}

func TestStreamParsesReasoningDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"choices":[{"delta":{"reasoning_content":"think "},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"reasoning_content":"more","content":"done"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
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

func TestStreamParsesToolCallDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"test_tool","arguments":"{\"path\""}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"README.md\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
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
	if resp.ToolCalls[0].ID != "call-1" || resp.ToolCalls[0].Name != "test_tool" {
		t.Fatalf("ToolCall = %#v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Arguments != `{"path":"README.md"}` {
		t.Fatalf("Arguments = %q", resp.ToolCalls[0].Arguments)
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

func TestStreamRejectsNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, `{"choices":[]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{}, nil); err == nil {
		t.Fatal("Stream() error = nil")
	}
}

// TestStreamRejectsMissingFinishReason verifies that a stream interrupted before
// the model finishes (no finish_reason) returns an error instead of a partial response.
func TestStreamRejectsMissingFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send reasoning deltas but never send a finish_reason, then [DONE].
		writeSSE(w, `{"choices":[{"delta":{"reasoning_content":"thinking"},"finish_reason":null}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	if _, err := provider.Stream(context.Background(), model.ChatRequest{}, nil); err == nil {
		t.Fatal("Stream() error = nil, want error for missing finish reason")
	}
}

// TestStreamSkipsEmptyAssistantMessage verifies that assistant messages with no
// content, no parts, and no tool calls are skipped when building the request,
// preventing an empty content array that some APIs reject as missing.
func TestStreamSkipsEmptyAssistantMessage(t *testing.T) {
	var gotReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := newTestProvider(t, server.URL)
	_, err := provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "first"},
			// Simulate an assistant message left by an interrupted stream.
			{Role: model.RoleAssistant, ReasoningContent: "partial thinking"},
			{Role: model.RoleUser, Content: "second"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	// The empty assistant message should be skipped, leaving: system? no, user, user.
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %d, want 2 (empty assistant skipped)", len(gotReq.Messages))
	}
	if gotReq.Messages[0].Role != "user" || gotReq.Messages[0].Content == nil {
		t.Fatalf("message 0 = %#v", gotReq.Messages[0])
	}
	if gotReq.Messages[1].Role != "user" || gotReq.Messages[1].Content == nil {
		t.Fatalf("message 1 = %#v", gotReq.Messages[1])
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

func assertContentParts(t *testing.T, content any, want []apiContentPart) {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	var got []apiContentPart
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("content is not parts array: %s", raw)
	}
	if len(got) != len(want) {
		t.Fatalf("content parts = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Text != want[i].Text {
			t.Fatalf("content parts = %#v, want %#v", got, want)
		}
		if want[i].ImageURL == nil {
			if got[i].ImageURL != nil {
				t.Fatalf("content parts = %#v, want %#v", got, want)
			}
			continue
		}
		if got[i].ImageURL == nil || *got[i].ImageURL != *want[i].ImageURL {
			t.Fatalf("content parts = %#v, want %#v", got, want)
		}
	}
}

func TestStreamSendsCustomUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		writeSSE(w,
			`{"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)
	}))
	defer server.Close()

	provider, err := New(Config{
		BaseURL:   server.URL,
		APIKey:    "sk-test",
		Model:     "m",
		UserAgent: "my-app/2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}, nil)
	if gotUA != "my-app/2.0" {
		t.Fatalf("User-Agent = %q, want %q", gotUA, "my-app/2.0")
	}
}

func TestStreamSendsDefaultUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		writeSSE(w,
			`{"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)
	}))
	defer server.Close()

	provider, err := New(Config{
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Model:   "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = provider.Stream(context.Background(), model.ChatRequest{
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}, nil)
	if gotUA != "atlas/"+version.Current {
		t.Fatalf("User-Agent = %q, want %q", gotUA, "atlas/"+version.Current)
	}
}

func writeSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		_, _ = w.Write([]byte("data: " + event + "\n\n"))
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}
