// Package config 读取 Atlas 的本机应用配置。
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirName  = ".atlas"
	configFileName = "config.json"

	// ProviderFormatChatCompletions 表示使用 Chat Completions API 格式。
	ProviderFormatChatCompletions = "chat_completions"
	// ProviderFormatResponses 表示使用 Responses API 格式。
	ProviderFormatResponses = "responses"

	// ModelInputFormatText 表示模型支持文本输入。
	ModelInputFormatText = "text"
	// ModelInputFormatImage 表示模型支持图片输入。
	ModelInputFormatImage = "image"

	defaultMaxSteps               = 20
	defaultCompactionTriggerRatio = 0.8
	defaultTavilyBaseURL          = "https://api.tavily.com"
	defaultWeixinBaseURL          = "https://ilinkai.weixin.qq.com"
	defaultWeixinCDNBaseURL       = "https://novac2c.cdn.weixin.qq.com/c2c"
)

// Config 是 Atlas CLI 启动时需要的应用配置。
type Config struct {
	DefaultModel string           `json:"default_model"`
	Providers    []ProviderConfig `json:"providers"`
	Agent        AgentConfig      `json:"agent"`
	Memory       MemoryConfig     `json:"memory"`
	Session      SessionConfig    `json:"session"`
	Services     ServicesConfig   `json:"services"`
}

// ProviderConfig 描述一个模型 API provider。
type ProviderConfig struct {
	Name    string          `json:"name"`
	Format  string          `json:"format"`
	BaseURL string          `json:"base_url"`
	APIKey  string          `json:"api_key"`
	Models  []ProviderModel `json:"models"`
}

// ProviderModel 描述一个可选模型。
type ProviderModel struct {
	Value            string                    `json:"value"`
	Name             string                    `json:"name"`
	Description      string                    `json:"description,omitempty"`
	ContextWindow    int                       `json:"context_window"`
	MaxTokens        int                       `json:"max_tokens"`
	InputFormats     []string                  `json:"input_formats"`
	PromptCache      PromptCacheConfig         `json:"prompt_cache,omitempty"`
	ReasoningEfforts []ProviderReasoningEffort `json:"reasoning_efforts,omitempty"`
}

// PromptCacheConfig 描述模型侧 prompt cache 的启用方式。
type PromptCacheConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// ProviderReasoningEffort 描述模型支持的一个 reasoning effort 选项。
type ProviderReasoningEffort struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// AgentConfig 描述 agent turn loop 的运行参数。
type AgentConfig struct {
	MaxSteps               int     `json:"max_steps"`
	Temperature            float64 `json:"temperature"`
	CompactionTriggerRatio float64 `json:"compaction_trigger_ratio"`
}

// MemoryConfig 描述长期记忆后台抽取和检索配置。
type MemoryConfig struct {
	Enabled *bool  `json:"enabled"`
	Model   string `json:"model"`
}

// IsEnabled 返回长期记忆是否启用；未配置时默认启用。
func (c MemoryConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// SessionConfig 描述本地会话存储参数。
type SessionConfig struct {
	DBPath string `json:"db_path"`
}

// ServicesConfig 描述 Atlas 可选接入的外部服务。
type ServicesConfig struct {
	Tavily TavilyConfig `json:"tavily"`
	Weixin WeixinConfig `json:"weixin"`
}

// TavilyConfig 描述 Tavily 搜索和网页提取服务配置。
type TavilyConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// WeixinConfig 描述微信远程控制通道配置。
type WeixinConfig struct {
	BaseURL    string `json:"base_url"`
	CDNBaseURL string `json:"cdn_base_url"`
}

// DefaultPath 返回当前用户主目录下的 Atlas 配置路径。
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// LoadDefault 从默认路径读取配置文件。
func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return LoadFile(path)
}

// LoadFile 从指定 JSON 文件读取并校验配置。
func LoadFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate 校验 Atlas 运行所需的配置字段。
func (c Config) Validate() error {
	if err := c.validateProviders(); err != nil {
		return err
	}
	if c.Memory.IsEnabled() && strings.TrimSpace(c.Memory.Model) != "" {
		if _, _, err := c.ResolveModel(c.Memory.Model); err != nil {
			return fmt.Errorf("memory.model %q is not in any provider models", c.Memory.Model)
		}
	}
	if c.Agent.Temperature < 0 || c.Agent.Temperature > 2 {
		return fmt.Errorf("agent.temperature must be between 0 and 2")
	}
	if c.Agent.CompactionTriggerRatio <= 0 || c.Agent.CompactionTriggerRatio >= 1 {
		return fmt.Errorf("agent.compaction_trigger_ratio must be greater than 0 and less than 1")
	}
	if c.Services.Tavily.APIKey != "" {
		tavilyURL, err := url.Parse(c.Services.Tavily.BaseURL)
		if err != nil || tavilyURL.Scheme == "" || tavilyURL.Host == "" || !isHTTPURL(tavilyURL) {
			return fmt.Errorf("services.tavily.base_url is invalid")
		}
	} else if c.Services.Tavily.BaseURL != "" && c.Services.Tavily.BaseURL != defaultTavilyBaseURL {
		return fmt.Errorf("services.tavily.api_key is required when services.tavily.base_url is set")
	}
	if c.Services.Weixin.BaseURL != "" {
		weixinURL, err := url.Parse(c.Services.Weixin.BaseURL)
		if err != nil || weixinURL.Scheme == "" || weixinURL.Host == "" || !isHTTPURL(weixinURL) {
			return fmt.Errorf("services.weixin.base_url is invalid")
		}
	}
	if c.Services.Weixin.CDNBaseURL != "" {
		cdnURL, err := url.Parse(c.Services.Weixin.CDNBaseURL)
		if err != nil || cdnURL.Scheme == "" || cdnURL.Host == "" || !isHTTPURL(cdnURL) {
			return fmt.Errorf("services.weixin.cdn_base_url is invalid")
		}
	}
	return nil
}

// validateProviders 校验所有 provider 配置，并检查 model value 跨 provider 全局唯一。
func (c Config) validateProviders() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("providers is required")
	}
	if strings.TrimSpace(c.DefaultModel) == "" {
		return fmt.Errorf("default_model is required")
	}
	seenProviders := make(map[string]struct{}, len(c.Providers))
	seenModels := make(map[string]string) // model value -> provider name
	defaultFound := false
	for i, provider := range c.Providers {
		providerName := strings.TrimSpace(provider.Name)
		if providerName == "" {
			return fmt.Errorf("providers[%d].name is required", i)
		}
		if _, ok := seenProviders[providerName]; ok {
			return fmt.Errorf("providers contains duplicate name %q", providerName)
		}
		seenProviders[providerName] = struct{}{}
		if err := provider.Validate(i); err != nil {
			return err
		}
		for _, model := range provider.Models {
			if existing, ok := seenModels[model.Value]; ok {
				return fmt.Errorf("model value %q is duplicated in providers %q and %q", model.Value, existing, providerName)
			}
			seenModels[model.Value] = providerName
			if model.Value == c.DefaultModel {
				defaultFound = true
			}
		}
	}
	if !defaultFound {
		return fmt.Errorf("default_model %q is not in any provider models", c.DefaultModel)
	}
	return nil
}

// ResolveModel 在所有 provider 中查找指定 value 的模型，返回 provider 和模型。
// value 为空时返回 default_model 对应的模型。
func (c Config) ResolveModel(value string) (ProviderConfig, ProviderModel, error) {
	if strings.TrimSpace(value) == "" {
		value = c.DefaultModel
	}
	for _, provider := range c.Providers {
		for _, model := range provider.Models {
			if model.Value == value {
				return provider, model, nil
			}
		}
	}
	return ProviderConfig{}, ProviderModel{}, fmt.Errorf("model %q is not configured in any provider", value)
}

// Validate 校验单个 provider 配置。
func (p ProviderConfig) Validate(index int) error {
	prefix := fmt.Sprintf("providers[%d]", index)
	if p.Format != "" && !validProviderFormat(p.Format) {
		return fmt.Errorf("%s.format must be chat_completions or responses", prefix)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("%s.base_url is required", prefix)
	}
	baseURL, err := url.Parse(p.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return fmt.Errorf("%s.base_url is invalid", prefix)
	}
	if p.APIKey == "" {
		return fmt.Errorf("%s.api_key is required", prefix)
	}
	if len(p.Models) == 0 {
		return fmt.Errorf("%s.models is required", prefix)
	}
	seen := make(map[string]struct{}, len(p.Models))
	for i, model := range p.Models {
		if strings.TrimSpace(model.Value) == "" {
			return fmt.Errorf("%s.models[%d].value is required", prefix, i)
		}
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("%s.models[%d].name is required", prefix, i)
		}
		if model.ContextWindow <= 0 {
			return fmt.Errorf("%s.models[%d].context_window must be positive", prefix, i)
		}
		if model.MaxTokens <= 0 {
			return fmt.Errorf("%s.models[%d].max_tokens must be positive", prefix, i)
		}
		if model.MaxTokens > model.ContextWindow {
			return fmt.Errorf("%s.models[%d].max_tokens must be less than or equal to context_window", prefix, i)
		}
		if err := validateModelInputFormats(prefix, i, model); err != nil {
			return err
		}
		if err := validateModelReasoningEfforts(prefix, i, model); err != nil {
			return err
		}
		if _, ok := seen[model.Value]; ok {
			return fmt.Errorf("%s.models contains duplicate value %q", prefix, model.Value)
		}
		seen[model.Value] = struct{}{}
	}
	return nil
}

// validateModelInputFormats 校验模型输入格式声明。
func validateModelInputFormats(prefix string, modelIndex int, model ProviderModel) error {
	modelPrefix := fmt.Sprintf("%s.models[%d]", prefix, modelIndex)
	if len(model.InputFormats) == 0 {
		return fmt.Errorf("%s.input_formats is required", modelPrefix)
	}
	seen := make(map[string]struct{}, len(model.InputFormats))
	hasText := false
	for i, format := range model.InputFormats {
		value := strings.TrimSpace(format)
		if value == "" {
			return fmt.Errorf("%s.input_formats[%d] is required", modelPrefix, i)
		}
		switch value {
		case ModelInputFormatText:
			hasText = true
		case ModelInputFormatImage:
		default:
			return fmt.Errorf("%s.input_formats[%d] must be text or image", modelPrefix, i)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s.input_formats contains duplicate value %q", modelPrefix, value)
		}
		seen[value] = struct{}{}
	}
	if !hasText {
		return fmt.Errorf("%s.input_formats must include text", modelPrefix)
	}
	return nil
}

// validateModelReasoningEfforts 校验模型级 reasoning effort 声明。
func validateModelReasoningEfforts(prefix string, modelIndex int, model ProviderModel) error {
	modelPrefix := fmt.Sprintf("%s.models[%d]", prefix, modelIndex)
	seen := make(map[string]struct{}, len(model.ReasoningEfforts))
	for i, effort := range model.ReasoningEfforts {
		effortPrefix := fmt.Sprintf("%s.reasoning_efforts[%d]", modelPrefix, i)
		value := strings.TrimSpace(effort.Value)
		if value == "" {
			return fmt.Errorf("%s.value is required", effortPrefix)
		}
		if strings.TrimSpace(effort.Name) == "" {
			return fmt.Errorf("%s.name is required", effortPrefix)
		}
		if _, ok := seen[effort.Value]; ok {
			return fmt.Errorf("%s.reasoning_efforts contains duplicate value %q", modelPrefix, effort.Value)
		}
		seen[effort.Value] = struct{}{}
	}
	return nil
}

// AllModels 返回所有 provider 的所有模型，附带其所属 provider。
func (c Config) AllModels() []ProviderModelInfo {
	var result []ProviderModelInfo
	for _, provider := range c.Providers {
		for _, model := range provider.Models {
			result = append(result, ProviderModelInfo{
				Provider: provider,
				Model:    model,
			})
		}
	}
	return result
}

// ProviderModelInfo 包含模型定义及其所属 provider。
type ProviderModelInfo struct {
	Provider ProviderConfig
	Model    ProviderModel
}

// SupportsReasoningEffort 返回模型是否声明支持指定 reasoning effort。
func (m ProviderModel) SupportsReasoningEffort(value string) bool {
	for _, effort := range m.ReasoningEfforts {
		if effort.Value == value {
			return true
		}
	}
	return false
}

// SupportsInputFormat 返回模型是否声明支持指定输入格式。
func (m ProviderModel) SupportsInputFormat(value string) bool {
	for _, format := range m.InputFormats {
		if format == value {
			return true
		}
	}
	return false
}

func (c *Config) applyDefaults() {
	for i := range c.Providers {
		if c.Providers[i].Format == "" {
			c.Providers[i].Format = ProviderFormatChatCompletions
		}
	}
	if c.Agent.MaxSteps <= 0 {
		c.Agent.MaxSteps = defaultMaxSteps
	}
	if c.Agent.CompactionTriggerRatio == 0 {
		c.Agent.CompactionTriggerRatio = defaultCompactionTriggerRatio
	}
	if c.Services.Tavily.BaseURL == "" {
		c.Services.Tavily.BaseURL = defaultTavilyBaseURL
	}
	if c.Services.Weixin.BaseURL == "" {
		c.Services.Weixin.BaseURL = defaultWeixinBaseURL
	}
	if c.Services.Weixin.CDNBaseURL == "" {
		c.Services.Weixin.CDNBaseURL = defaultWeixinCDNBaseURL
	}
}

func isHTTPURL(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

func validProviderFormat(format string) bool {
	switch format {
	case ProviderFormatChatCompletions, ProviderFormatResponses:
		return true
	default:
		return false
	}
}
