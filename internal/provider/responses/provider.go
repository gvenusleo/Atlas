// Package responses implements the Responses API format streaming model provider.
package responses

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

const responsesPath = "/responses"

// Config holds the connection configuration for creating a Responses provider.
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	UserAgent          string
	PromptCacheEnabled bool
	HTTPClient         *http.Client
}

// Provider calls the Responses API.
type Provider struct {
	baseURL            string
	apiKey             string
	model              string
	userAgent          string
	promptCacheEnabled bool
	httpClient         *http.Client
}

// New creates a Responses provider.
func New(config Config) (*Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("responses base url is required")
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("responses base url is invalid")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("responses api key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("responses model is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "atlas/" + version.Current
	}
	return &Provider{
		baseURL:            strings.TrimRight(config.BaseURL, "/"),
		apiKey:             config.APIKey,
		model:              config.Model,
		userAgent:          userAgent,
		promptCacheEnabled: config.PromptCacheEnabled,
		httpClient:         httpClient,
	}, nil
}

// Stream executes a streaming Responses request and returns the accumulated complete response.
func (p *Provider) Stream(ctx context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	body, err := json.Marshal(p.buildRequest(req))
	if err != nil {
		return model.ChatResponse{}, err
	}

	sendFunc := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+responsesPath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", p.userAgent)
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
		return model.ChatResponse{}, fmt.Errorf("responses request failed: status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return parseStream(httpResp.Body, emit)
}

func (p *Provider) buildRequest(req model.ChatRequest) responsesRequest {
	apiReq := responsesRequest{
		Model:           p.model,
		Instructions:    req.System,
		Input:           toInputItems(req.Messages),
		Tools:           toAPITools(req.Tools),
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		Reasoning:       toAPIReasoning(req.ReasoningEffort),
		Text:            toAPIText(req.ResponseFormat),
		Stream:          true,
	}
	if p.promptCacheEnabled && req.SessionID != "" {
		apiReq.PromptCacheKey = req.SessionID
	}
	if len(apiReq.Tools) == 0 {
		apiReq.Tools = nil
	}
	return apiReq
}

type responsesRequest struct {
	Model           string      `json:"model"`
	Instructions    string      `json:"instructions,omitempty"`
	Input           []inputItem `json:"input"`
	Tools           []apiTool   `json:"tools,omitempty"`
	MaxOutputTokens int         `json:"max_output_tokens,omitempty"`
	Temperature     float64     `json:"temperature,omitempty"`
	Reasoning       *reasoning  `json:"reasoning,omitempty"`
	Text            *apiText    `json:"text,omitempty"`
	PromptCacheKey  string      `json:"prompt_cache_key,omitempty"`
	Stream          bool        `json:"stream"`
}

type inputItem struct {
	Raw       json.RawMessage `json:"-"`
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   any             `json:"content,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
}

type inputContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func (i inputItem) MarshalJSON() ([]byte, error) {
	if len(i.Raw) > 0 {
		return i.Raw, nil
	}
	type alias inputItem
	return json.Marshal(alias(i))
}

type apiTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

type reasoning struct {
	Effort string `json:"effort"`
}

type apiText struct {
	Format apiTextFormat `json:"format"`
}

type apiTextFormat struct {
	Type string `json:"type"`
}

type streamEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Response *apiResponse    `json:"response"`
	Item     *apiOutputItem  `json:"item"`
	Usage    apiUsage        `json:"usage"`
	Error    *responsesError `json:"error"`
}

type apiResponse struct {
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
	Usage  apiUsage        `json:"usage"`
}

type apiOutputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Text      string `json:"text"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type apiUsage struct {
	InputTokens        int               `json:"input_tokens"`
	OutputTokens       int               `json:"output_tokens"`
	TotalTokens        int               `json:"total_tokens"`
	InputTokensDetails inputTokenDetails `json:"input_tokens_details"`
}

type inputTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesError struct {
	Message string `json:"message"`
}

func toInputItems(messages []model.Message) []inputItem {
	items := make([]inputItem, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case model.RoleAssistant:
			if len(msg.ProviderItems) > 0 {
				items = append(items, toProviderInputItems(msg.ProviderItems)...)
				continue
			}
			if msg.Content != "" || len(msg.Parts) > 0 {
				items = append(items, inputItem{
					Role:    string(model.RoleAssistant),
					Content: toInputContentParts(model.MessageParts(msg)),
				})
			}
			for _, call := range msg.ToolCalls {
				items = append(items, inputItem{
					Type:      "function_call",
					CallID:    call.ID,
					Name:      call.Name,
					Arguments: call.Arguments,
				})
			}
		case model.RoleTool:
			items = append(items, inputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
		default:
			items = append(items, inputItem{
				Role:    string(msg.Role),
				Content: toInputContentParts(model.MessageParts(msg)),
			})
		}
	}
	return items
}

func toInputContentParts(parts []model.ContentPart) []inputContentPart {
	inputParts := make([]inputContentPart, 0, len(parts))
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
			inputParts = append(inputParts, inputContentPart{
				Type:     "input_image",
				ImageURL: part.DataURL,
				Detail:   detail,
			})
		default:
			inputParts = append(inputParts, inputContentPart{
				Type: "input_text",
				Text: part.Text,
			})
		}
	}
	return inputParts
}

func toProviderInputItems(providerItems []model.ProviderItem) []inputItem {
	items := make([]inputItem, 0, len(providerItems))
	for _, item := range providerItems {
		if item.Type != "responses" || item.JSON == "" {
			continue
		}
		items = append(items, inputItem{Raw: json.RawMessage(item.JSON)})
	}
	return items
}

func toAPITools(tools []model.ToolDefinition) []apiTool {
	apiTools := make([]apiTool, 0, len(tools))
	for _, tool := range tools {
		apiTools = append(apiTools, apiTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Strict:      false,
		})
	}
	return apiTools
}

func toAPIReasoning(effort string) *reasoning {
	switch effort {
	case "":
		return nil
	case "max":
		return &reasoning{Effort: "high"}
	default:
		return &reasoning{Effort: effort}
	}
}

func toAPIText(format model.ResponseFormat) *apiText {
	if format == model.ResponseFormatJSONObject {
		return &apiText{Format: apiTextFormat{Type: string(format)}}
	}
	return nil
}

func parseStream(body io.Reader, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	var content strings.Builder
	var reasoningContent strings.Builder
	var status string
	var sawEvent bool
	var toolCalls []model.ToolCall
	var usage apiUsage
	var providerItems []model.ProviderItem

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return model.ChatResponse{}, err
		}
		if line != "" {
			done, err := parseStreamLine(line, emit, &content, &reasoningContent, &toolCalls, &status, &usage, &providerItems, &sawEvent)
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
	if !sawEvent {
		return model.ChatResponse{}, fmt.Errorf("responses stream returned no events")
	}
	// An empty status means response.completed was never received — the stream
	// was interrupted before the model finished. Returning a partial response
	// would produce an assistant message with empty content.
	if status == "" {
		return model.ChatResponse{}, fmt.Errorf("responses stream ended without completion")
	}
	return model.ChatResponse{
		Content:          content.String(),
		ReasoningContent: reasoningContent.String(),
		ToolCalls:        toolCalls,
		StopReason:       responsesStopReason(status, toolCalls),
		RawFinish:        status,
		Usage:            toModelUsage(usage),
		ProviderItems:    providerItems,
	}, nil
}

func parseStreamLine(
	line string,
	emit func(model.StreamEvent) error,
	content *strings.Builder,
	reasoningContent *strings.Builder,
	toolCalls *[]model.ToolCall,
	status *string,
	usage *apiUsage,
	providerItems *[]model.ProviderItem,
	sawEvent *bool,
) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data:") {
		return false, nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "[DONE]" {
		return true, nil
	}

	var event streamEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return false, err
	}
	*sawEvent = true
	if event.Error != nil {
		return false, fmt.Errorf("responses stream error: %s", event.Error.Message)
	}
	switch event.Type {
	case "response.output_text.delta":
		content.WriteString(event.Delta)
		if emit != nil {
			if err := emit(model.StreamEvent{Type: model.StreamTextDelta, Delta: event.Delta}); err != nil {
				return false, err
			}
		}
	case "response.reasoning_summary_text.delta":
		reasoningContent.WriteString(event.Delta)
		if emit != nil {
			if err := emit(model.StreamEvent{Type: model.StreamReasoningDelta, Delta: event.Delta}); err != nil {
				return false, err
			}
		}
	case "response.output_item.done":
		if event.Item != nil && event.Item.Type == "function_call" {
			*toolCalls = append(*toolCalls, toModelToolCall(*event.Item))
		}
	case "response.completed":
		if event.Response != nil {
			*status = event.Response.Status
			if hasUsage(event.Response.Usage) {
				*usage = event.Response.Usage
			}
			if len(*toolCalls) == 0 {
				*toolCalls = appendOutputToolCalls((*toolCalls)[:0], outputItems(event.Response.Output))
			}
			if content.Len() == 0 {
				content.WriteString(outputText(outputItems(event.Response.Output)))
			}
			*providerItems = toProviderItems(event.Response.Output)
		}
		if hasUsage(event.Usage) {
			*usage = event.Usage
		}
	}
	return false, nil
}

func toModelToolCall(item apiOutputItem) model.ToolCall {
	return model.ToolCall{
		ID:        item.CallID,
		Name:      item.Name,
		Arguments: item.Arguments,
	}
}

func appendOutputToolCalls(calls []model.ToolCall, output []apiOutputItem) []model.ToolCall {
	for _, item := range output {
		if item.Type == "function_call" {
			calls = append(calls, toModelToolCall(item))
		}
	}
	return calls
}

func toProviderItems(output json.RawMessage) []model.ProviderItem {
	if len(output) == 0 {
		return nil
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(output, &rawItems); err != nil {
		return nil
	}
	items := make([]model.ProviderItem, 0, len(rawItems))
	for _, item := range rawItems {
		items = append(items, model.ProviderItem{
			Type: "responses",
			JSON: string(item),
		})
	}
	return items
}

func outputItems(output json.RawMessage) []apiOutputItem {
	if len(output) == 0 {
		return nil
	}
	var items []apiOutputItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil
	}
	return items
}

func outputText(output []apiOutputItem) string {
	var builder strings.Builder
	for _, item := range output {
		if item.Type != "message" {
			continue
		}
		if item.Text != "" {
			builder.WriteString(item.Text)
		}
		for _, content := range item.Content {
			if content.Text != "" {
				builder.WriteString(content.Text)
			}
		}
	}
	return builder.String()
}

func hasUsage(usage apiUsage) bool {
	return usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 || usage.InputTokensDetails.CachedTokens != 0
}

func toModelUsage(usage apiUsage) model.Usage {
	return model.Usage{
		InputTokens:          usage.InputTokens,
		OutputTokens:         usage.OutputTokens,
		TotalTokens:          usage.TotalTokens,
		CacheReadInputTokens: usage.InputTokensDetails.CachedTokens,
	}
}

func responsesStopReason(status string, toolCalls []model.ToolCall) model.StopReason {
	if len(toolCalls) > 0 {
		return model.StopToolUse
	}
	switch status {
	case "completed":
		return model.StopEndTurn
	case "incomplete":
		return model.StopMaxTokens
	default:
		return model.StopUnknown
	}
}
