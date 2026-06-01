package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	defaultModel   = "deepseek-v4-flash"
)

// Client streams responses from DeepSeek's OpenAI-compatible chat API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Config contains DeepSeek provider settings.
type Config struct {
	APIKey  string
	BaseURL string
}

// New creates a DeepSeek client. If APIKey is empty, DEEPSEEK_API_KEY is used.
func New(config Config) *Client {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 0,
		},
	}
}

// DefaultModel returns the initial model Atlas uses for DeepSeek.
func DefaultModel() string {
	return defaultModel
}

// StreamChat streams a single DeepSeek chat completion request.
func (c *Client) StreamChat(ctx context.Context, req model.ChatRequest) (<-chan model.StreamEvent, <-chan error) {
	events := make(chan model.StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)
		if err := c.stream(ctx, req, events); err != nil {
			errs <- err
		}
	}()

	return events, errs
}

// stream performs the HTTP request and forwards decoded SSE events.
func (c *Client) stream(ctx context.Context, req model.ChatRequest, events chan<- model.StreamEvent) error {
	if c.apiKey == "" {
		return fmt.Errorf("DEEPSEEK_API_KEY is required")
	}
	body, err := json.Marshal(c.requestBody(req))
	if err != nil {
		return fmt.Errorf("encode deepseek request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create deepseek request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		return fmt.Errorf("deepseek status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return readSSE(resp.Body, events)
}

// requestBody converts Atlas chat input to DeepSeek's request shape.
func (c *Client) requestBody(req model.ChatRequest) map[string]any {
	modelName := req.Model
	if strings.TrimSpace(modelName) == "" {
		modelName = defaultModel
	}
	body := map[string]any{
		"model":    modelName,
		"messages": encodeMessages(req.System, req.Messages),
		"stream":   true,
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(req.Tools) > 0 {
		body["tools"] = encodeTools(req.Tools)
		body["tool_choice"] = "auto"
	}
	return body
}

// encodeMessages converts Atlas messages to OpenAI-compatible chat messages.
func encodeMessages(system string, messages []model.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages)+1)
	if strings.TrimSpace(system) != "" {
		out = append(out, map[string]any{
			"role":    string(model.RoleSystem),
			"content": system,
		})
	}
	for _, message := range messages {
		item := map[string]any{
			"role":    string(message.Role),
			"content": message.Content,
		}
		if message.ToolCallID != "" {
			item["tool_call_id"] = message.ToolCallID
		}
		if len(message.ToolCalls) > 0 {
			item["tool_calls"] = encodeToolCalls(message.ToolCalls)
		}
		out = append(out, item)
	}
	return out
}

// encodeToolCalls converts stored assistant tool calls back to provider format.
func encodeToolCalls(calls []model.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, map[string]any{
			"id":   call.ID,
			"type": "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Arguments,
			},
		})
	}
	return out
}

// encodeTools exposes Atlas tool definitions as function tools.
func encodeTools(tools []model.ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return out
}

// readSSE decodes DeepSeek's server-sent event stream.
func readSSE(body io.Reader, events chan<- model.StreamEvent) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	acc := newToolAccumulator()
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			acc.flush(events)
			events <- model.StreamEvent{Done: true}
			return nil
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("decode deepseek stream: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				events <- model.StreamEvent{TextDelta: choice.Delta.Content}
			}
			for _, call := range choice.Delta.ToolCalls {
				if flushed := acc.add(call); flushed != nil {
					events <- model.StreamEvent{ToolCall: flushed}
				}
			}
			if choice.FinishReason == "tool_calls" {
				acc.flush(events)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read deepseek stream: %w", err)
	}
	acc.flush(events)
	events <- model.StreamEvent{Done: true}
	return nil
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string          `json:"content"`
			ToolCalls []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type partialToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type toolAccumulator struct {
	byIndex map[int]*partialToolCall
}

func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{byIndex: make(map[int]*partialToolCall)}
}

// add applies one streaming tool-call delta.
func (a *toolAccumulator) add(delta toolCallDelta) *model.ToolCall {
	call := a.byIndex[delta.Index]
	if call == nil {
		call = &partialToolCall{}
		a.byIndex[delta.Index] = call
	}
	if delta.ID != "" {
		call.id = delta.ID
	}
	if delta.Function.Name != "" {
		call.name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		call.arguments.WriteString(delta.Function.Arguments)
	}
	return nil
}

// flush emits complete tool calls in provider index order.
func (a *toolAccumulator) flush(events chan<- model.StreamEvent) {
	indexes := make([]int, 0, len(a.byIndex))
	for index := range a.byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		call := a.byIndex[index]
		if call.name == "" {
			continue
		}
		events <- model.StreamEvent{ToolCall: &model.ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: call.arguments.String(),
		}}
		delete(a.byIndex, index)
	}
}

// compile-time guard for the provider interface.
var _ model.Provider = (*Client)(nil)
