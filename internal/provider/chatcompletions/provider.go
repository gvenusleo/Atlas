// Package chatcompletions implements the Chat Completions format streaming model provider.
package chatcompletions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/provider"
	"github.com/liuyuxin/atlas/internal/version"
)

const chatCompletionsPath = "/chat/completions"

// Config holds the connection configuration for creating a Chat Completions provider.
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	PromptCacheEnabled bool
	HTTPClient         *http.Client
}

// Provider calls the Chat Completions API.
type Provider struct {
	baseURL            string
	apiKey             string
	model              string
	promptCacheEnabled bool
	httpClient         *http.Client
}

// New creates a Chat Completions provider.
func New(config Config) (*Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("chat completions base url is required")
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("chat completions base url is invalid")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("chat completions api key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("chat completions model is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Provider{
		baseURL:            strings.TrimRight(config.BaseURL, "/"),
		apiKey:             config.APIKey,
		model:              config.Model,
		promptCacheEnabled: config.PromptCacheEnabled,
		httpClient:         httpClient,
	}, nil
}

// Stream executes a streaming chat request and returns the accumulated complete response.
func (p *Provider) Stream(ctx context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	body, err := json.Marshal(p.buildRequest(req))
	if err != nil {
		return model.ChatResponse{}, err
	}

	sendFunc := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+chatCompletionsPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", "atlas/"+version.Current)
		return httpReq, nil
	}

	httpResp, err := provider.DoWithRetry(ctx, p.httpClient, sendFunc)
	if err != nil {
		return model.ChatResponse{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return model.ChatResponse{}, err
		}
		return model.ChatResponse{}, fmt.Errorf("chat completion failed: status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return parseStream(httpResp.Body, emit)
}

func (p *Provider) buildRequest(req model.ChatRequest) chatRequest {
	apiReq := chatRequest{
		Model:           p.model,
		Messages:        toAPIMessages(req),
		Tools:           toAPITools(req.Tools),
		MaxTokens:       req.MaxTokens,
		Temperature:     req.Temperature,
		ReasoningEffort: req.ReasoningEffort,
		ResponseFormat:  toAPIResponseFormat(req.ResponseFormat),
		Stream:          true,
	}
	if p.promptCacheEnabled && req.SessionID != "" {
		apiReq.PromptCacheKey = req.SessionID
	}
	if len(apiReq.Tools) == 0 {
		apiReq.Tools = nil
	}
	// Request the API to return usage statistics in the last chunk of the streaming response.
	// Some providers (e.g., Ark) do not return usage by default in streaming mode; this must be explicitly enabled.
	apiReq.StreamOptions = &streamOptions{IncludeUsage: true}
	return apiReq
}

type chatRequest struct {
	Model           string         `json:"model"`
	Messages        []apiMessage   `json:"messages"`
	Tools           []apiTool      `json:"tools,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Temperature     float64        `json:"temperature,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	ResponseFormat  *apiFormat     `json:"response_format,omitempty"`
	PromptCacheKey  string         `json:"prompt_cache_key,omitempty"`
	Stream          bool           `json:"stream"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
}

// streamOptions controls additional behavior of streaming responses.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type apiFormat struct {
	Type string `json:"type"`
}

type apiMessage struct {
	Role             string        `json:"role"`
	Content          any           `json:"content"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ToolCalls        []apiToolCall `json:"tool_calls,omitempty"`
}

type apiContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *apiImageURLContent `json:"image_url,omitempty"`
}

type apiImageURLContent struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type apiTool struct {
	Type     string      `json:"type"`
	Function apiFunction `json:"function"`
}

type apiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type apiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiUsage struct {
	PromptTokens             int                `json:"prompt_tokens"`
	CompletionTokens         int                `json:"completion_tokens"`
	TotalTokens              int                `json:"total_tokens"`
	PromptTokensDetails      promptTokenDetails `json:"prompt_tokens_details"`
	CacheReadInputTokens     int                `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int                `json:"cache_creation_input_tokens"`
	CacheWriteInputTokens    int                `json:"cache_write_input_tokens"`
	PromptCacheHitTokens     int                `json:"prompt_cache_hit_tokens"`
}

type promptTokenDetails struct {
	CachedTokens             int `json:"cached_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheWriteTokens         int `json:"cache_write_tokens"`
}

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
	Usage   apiUsage       `json:"usage"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type streamDelta struct {
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []streamToolCallDelta `json:"tool_calls"`
}

type streamToolCallDelta struct {
	Index    int                         `json:"index"`
	ID       string                      `json:"id"`
	Type     string                      `json:"type"`
	Function streamToolCallFunctionDelta `json:"function"`
}

type streamToolCallFunctionDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func toAPIMessages(req model.ChatRequest) []apiMessage {
	messages := make([]apiMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, apiMessage{
			Role:    string(model.RoleSystem),
			Content: toAPIContentParts([]model.ContentPart{{Type: model.ContentPartText, Text: req.System}}),
		})
	}
	for _, msg := range req.Messages {
		// Skip assistant messages with no content, no parts, and no tool calls.
		// These result from interrupted streams and cause API rejection because
		// the serialized content is an empty array, which some APIs treat as missing.
		if msg.Role == model.RoleAssistant && msg.Content == "" && len(msg.Parts) == 0 && len(msg.ToolCalls) == 0 {
			continue
		}
		messages = append(messages, toAPIMessage(msg))
	}
	return messages
}

func toAPIMessage(msg model.Message) apiMessage {
	apiMsg := apiMessage{
		Role:             string(msg.Role),
		ReasoningContent: msg.ReasoningContent,
		ToolCallID:       msg.ToolCallID,
	}
	if msg.Role == model.RoleTool {
		apiMsg.Content = msg.Content
	} else {
		apiMsg.Content = toAPIContentParts(model.MessageParts(msg))
	}
	if len(msg.ToolCalls) > 0 {
		apiMsg.ToolCalls = make([]apiToolCall, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			apiMsg.ToolCalls = append(apiMsg.ToolCalls, apiToolCall{
				ID:   call.ID,
				Type: "function",
				Function: apiToolFunction{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			})
		}
	}
	return apiMsg
}

func toAPIContentParts(parts []model.ContentPart) []apiContentPart {
	apiParts := make([]apiContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case model.ContentPartImage:
			if part.DataURL == "" {
				continue
			}
			detail := string(part.Detail)
			if detail == "" {
				detail = string(model.ImageDetailAuto)
			}
			apiParts = append(apiParts, apiContentPart{
				Type: "image_url",
				ImageURL: &apiImageURLContent{
					URL:    part.DataURL,
					Detail: detail,
				},
			})
		default:
			apiParts = append(apiParts, apiContentPart{
				Type: "text",
				Text: part.Text,
			})
		}
	}
	return apiParts
}

func toAPITools(tools []model.ToolDefinition) []apiTool {
	apiTools := make([]apiTool, 0, len(tools))
	for _, tool := range tools {
		apiTools = append(apiTools, apiTool{
			Type: "function",
			Function: apiFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return apiTools
}

func toAPIResponseFormat(format model.ResponseFormat) *apiFormat {
	if format == model.ResponseFormatJSONObject {
		return &apiFormat{Type: string(format)}
	}
	return nil
}

func parseStream(body io.Reader, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	var content strings.Builder
	var reasoningContent strings.Builder
	var finishReason string
	var usage apiUsage
	var sawChoice bool
	var toolCalls []toolCallAccumulator

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return model.ChatResponse{}, err
		}
		if line != "" {
			done, err := parseStreamLine(line, emit, &content, &reasoningContent, &toolCalls, &finishReason, &usage, &sawChoice)
			if err != nil {
				return model.ChatResponse{}, err
			}
			if done {
				break
			}
		}
		if err == io.EOF {
			break
		}
	}
	if !sawChoice {
		return model.ChatResponse{}, fmt.Errorf("chat completion stream returned no choices")
	}
	// An empty finish reason means the stream was interrupted before the model
	// finished generating (e.g. connection dropped mid-stream). Returning a
	// partial response would produce an assistant message with empty content,
	// which the API rejects on the next request.
	if finishReason == "" {
		return model.ChatResponse{}, fmt.Errorf("chat completion stream ended without finish reason")
	}
	return model.ChatResponse{
		Content:          content.String(),
		ReasoningContent: reasoningContent.String(),
		ToolCalls:        toModelToolCallsFromAccumulators(toolCalls),
		StopReason:       toStopReason(finishReason),
		RawFinish:        finishReason,
		Usage:            toModelUsage(usage),
	}, nil
}

func parseStreamLine(
	line string,
	emit func(model.StreamEvent) error,
	content *strings.Builder,
	reasoningContent *strings.Builder,
	toolCalls *[]toolCallAccumulator,
	finishReason *string,
	usage *apiUsage,
	sawChoice *bool,
) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data:") {
		return false, nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "[DONE]" {
		return true, nil
	}

	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false, err
	}
	if hasUsage(chunk.Usage) {
		*usage = chunk.Usage
	}
	if len(chunk.Choices) == 0 {
		return false, nil
	}
	*sawChoice = true
	choice := chunk.Choices[0]
	if choice.FinishReason != "" {
		*finishReason = choice.FinishReason
	}
	if choice.Delta.ReasoningContent != "" {
		reasoningContent.WriteString(choice.Delta.ReasoningContent)
		if emit != nil {
			if err := emit(model.StreamEvent{
				Type:  model.StreamReasoningDelta,
				Delta: choice.Delta.ReasoningContent,
			}); err != nil {
				return false, err
			}
		}
	}
	if choice.Delta.Content != "" {
		content.WriteString(choice.Delta.Content)
		if emit != nil {
			if err := emit(model.StreamEvent{
				Type:  model.StreamTextDelta,
				Delta: choice.Delta.Content,
			}); err != nil {
				return false, err
			}
		}
	}
	for _, delta := range choice.Delta.ToolCalls {
		appendToolCallDelta(toolCalls, delta)
	}
	return false, nil
}

type toolCallAccumulator struct {
	id        string
	name      string
	arguments string
}

func appendToolCallDelta(toolCalls *[]toolCallAccumulator, delta streamToolCallDelta) {
	for len(*toolCalls) <= delta.Index {
		*toolCalls = append(*toolCalls, toolCallAccumulator{})
	}
	call := &(*toolCalls)[delta.Index]
	if delta.ID != "" {
		call.id = delta.ID
	}
	if delta.Function.Name != "" {
		call.name += delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		call.arguments += delta.Function.Arguments
	}
}

func toModelToolCallsFromAccumulators(calls []toolCallAccumulator) []model.ToolCall {
	toolCalls := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.id == "" && call.name == "" && call.arguments == "" {
			continue
		}
		toolCalls = append(toolCalls, model.ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: call.arguments,
		})
	}
	return toolCalls
}

func toModelUsage(usage apiUsage) model.Usage {
	return model.Usage{
		InputTokens:           usage.PromptTokens,
		OutputTokens:          usage.CompletionTokens,
		TotalTokens:           usage.TotalTokens,
		CacheReadInputTokens:  max(usage.PromptTokensDetails.CachedTokens, usage.CacheReadInputTokens, usage.PromptCacheHitTokens),
		CacheWriteInputTokens: max(usage.CacheCreationInputTokens, usage.CacheWriteInputTokens, usage.PromptTokensDetails.CacheCreationInputTokens, usage.PromptTokensDetails.CacheWriteTokens),
	}
}

func hasUsage(usage apiUsage) bool {
	return usage.PromptTokens != 0 ||
		usage.CompletionTokens != 0 ||
		usage.TotalTokens != 0 ||
		usage.PromptTokensDetails.CachedTokens != 0 ||
		usage.CacheReadInputTokens != 0 ||
		usage.CacheCreationInputTokens != 0 ||
		usage.CacheWriteInputTokens != 0 ||
		usage.PromptCacheHitTokens != 0 ||
		usage.PromptTokensDetails.CacheCreationInputTokens != 0 ||
		usage.PromptTokensDetails.CacheWriteTokens != 0
}

func toStopReason(finishReason string) model.StopReason {
	switch finishReason {
	case "stop":
		return model.StopEndTurn
	case "tool_calls":
		return model.StopToolUse
	case "length":
		return model.StopMaxTokens
	default:
		return model.StopUnknown
	}
}
