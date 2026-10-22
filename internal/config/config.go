// Package config 读取 ~/.atlas/ 下的 settings.json 与 auth.json。
// settings.json 管接入配置（可展示、可备份），auth.json 管 API key（0600，永不共享）。
package config

import "slices"

// WireAPI 是请求/响应的 wire 协议方言，声明协议不代表已提供对应适配器。
type WireAPI string

const (
	WireAPIOpenAIResponses   WireAPI = "openai-responses"
	WireAPIOpenAICompletions WireAPI = "openai-completions"
)

// Settings 是 settings.json 的结构，不含任何凭据。
type Settings struct {
	Model     string              `json:"model,omitempty"` // 当前选用："provider/model-id"
	Providers map[string]Provider `json:"providers,omitempty"`
}

// Provider 是一家厂商的接入参数与其模型清单。
type Provider struct {
	WireAPI WireAPI          `json:"api,omitempty"`      // wire 协议方言
	BaseURL string           `json:"base_url,omitempty"` // 接入端点
	Models  map[string]Model `json:"models,omitempty"`   // 该 provider 的模型清单
}

// Model 是单个模型的元数据。
type Model struct {
	Name             string            `json:"name,omitempty"`          // 展示名，缺省用模型 id
	ContextWindow    int64             `json:"context_window,omitzero"` // 上下文窗口（token）
	MaxOutput        int64             `json:"max_output,omitzero"`     // 输出上限（token）
	Modalities       Modalities        `json:"modalities,omitzero"`     // 输入/输出模态（能力门控）
	Tools            *bool             `json:"tools,omitzero"`          // nil 视为支持
	Temperature      *bool             `json:"temperature,omitzero"`    // nil 视为支持
	Reasoning        bool              `json:"reasoning,omitzero"`      // 是否支持思考
	ReasoningOptions []ReasoningOption `json:"reasoning_options,omitempty"`
}

// ReasoningOption 是一种思考深度控制机制（对齐 models.dev 的 reasoning_options）。
type ReasoningOption struct {
	Type   ReasoningType `json:"type"`
	Values []string      `json:"values,omitempty"` // effort 档位（如 ["low","high"]）
	Min    int64         `json:"min,omitzero"`     // budget_tokens 下限
	Max    int64         `json:"max,omitzero"`     // budget_tokens 上限
}

// ReasoningType 是思考深度控制机制的类型取值。
type ReasoningType string

const (
	ReasoningTypeEffort       ReasoningType = "effort"        // 档位控制（values 给出支持档）
	ReasoningTypeToggle       ReasoningType = "toggle"        // 仅开关
	ReasoningTypeBudgetTokens ReasoningType = "budget_tokens" // token 预算（min/max）
)

// Modalities 是模型支持的输入/输出模态（对齐 models.dev 的 modalities）。
type Modalities struct {
	Input  []Modality `json:"input,omitempty"`
	Output []Modality `json:"output,omitempty"`
}

// AcceptsImages 报告模型是否声明了图片输入；没有声明就当作不支持。
func (m Modalities) AcceptsImages() bool {
	return slices.Contains(m.Input, ModalityImage)
}

// Modality 是输入/输出模态的取值。
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityPDF   Modality = "pdf"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
)
