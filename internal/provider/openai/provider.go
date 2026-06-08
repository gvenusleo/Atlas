package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

const chatCompletionsPath = "/chat/completions"

// Config 是创建 OpenAI-compatible provider 所需的连接配置。
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// Provider 调用 OpenAI-compatible Chat Completions API。
type Provider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// New 创建一个 OpenAI-compatible provider。
func New(config Config) (*Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("openai base url is required")
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("openai base url is invalid")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("openai api key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("openai model is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Provider{
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		apiKey:     config.APIKey,
		model:      config.Model,
		httpClient: httpClient,
	}, nil
}

// Chat 执行一次非流式聊天请求。
func (p *Provider) Chat(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	body, err := json.Marshal(p.buildRequest(req))
	if err != nil {
		return model.ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return model.ChatResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return model.ChatResponse{}, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return model.ChatResponse{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return model.ChatResponse{}, fmt.Errorf("chat completion failed: status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var apiResp chatResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return model.ChatResponse{}, err
	}
	if len(apiResp.Choices) == 0 {
		return model.ChatResponse{}, fmt.Errorf("chat completion returned no choices")
	}
	return toModelResponse(apiResp.Choices[0], apiResp.Usage), nil
}

func (p *Provider) buildRequest(req model.ChatRequest) chatRequest {
	apiReq := chatRequest{
		Model:       p.model,
		Messages:    toAPIMessages(req),
		Tools:       toAPITools(req.Tools),
		Temperature: req.Temperature,
	}
	if len(apiReq.Tools) == 0 {
		apiReq.Tools = nil
	}
	return apiReq
}

type chatRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Tools       []apiTool    `json:"tools,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
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

type chatResponse struct {
	Choices []choice `json:"choices"`
	Usage   apiUsage `json:"usage"`
}

type choice struct {
	Message      apiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func toAPIMessages(req model.ChatRequest) []apiMessage {
	messages := make([]apiMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, apiMessage{
			Role:    string(model.RoleSystem),
			Content: req.System,
		})
	}
	for _, msg := range req.Messages {
		messages = append(messages, toAPIMessage(msg))
	}
	return messages
}

func toAPIMessage(msg model.Message) apiMessage {
	apiMsg := apiMessage{
		Role:       string(msg.Role),
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
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

func toModelResponse(choice choice, usage apiUsage) model.ChatResponse {
	return model.ChatResponse{
		Content:    choice.Message.Content,
		ToolCalls:  toModelToolCalls(choice.Message.ToolCalls),
		StopReason: toStopReason(choice.FinishReason),
		RawFinish:  choice.FinishReason,
		Usage: model.Usage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			TotalTokens:  usage.TotalTokens,
		},
	}
}

func toModelToolCalls(calls []apiToolCall) []model.ToolCall {
	toolCalls := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, model.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return toolCalls
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
