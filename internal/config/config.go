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
	ActiveProvider string           `json:"active_provider"`
	Providers      []ProviderConfig `json:"providers"`
	Agent          AgentConfig      `json:"agent"`
	Memory         MemoryConfig     `json:"memory"`
	Session        SessionConfig    `json:"session"`
	Services       ServicesConfig   `json:"services"`
}

// ProviderConfig 描述一个模型 API provider。
type ProviderConfig struct {
	Name         string          `json:"name"`
	Format       string          `json:"format"`
	BaseURL      string          `json:"base_url"`
	APIKey       string          `json:"api_key"`
	DefaultModel string          `json:"default_model"`
	Models       []ProviderModel `json:"models"`
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
	active, err := c.ActiveProviderConfig()
	if err != nil {
		return err
	}
	if c.Memory.IsEnabled() && strings.TrimSpace(c.Memory.Model) != "" {
		if _, err := active.ResolveModel(c.Memory.Model); err != nil {
			return fmt.Errorf("memory.model %q is not in active provider models", c.Memory.Model)
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

// ActiveProviderConfig 返回当前启用的 provider 配置。
func (c Config) ActiveProviderConfig() (ProviderConfig, error) {
	if len(c.Providers) == 0 {
		return ProviderConfig{}, fmt.Errorf("providers is required")
	}
	name := strings.TrimSpace(c.ActiveProvider)
	if name == "" {
		return ProviderConfig{}, fmt.Errorf("active_provider is required")
	}
	seen := make(map[string]struct{}, len(c.Providers))
	var active ProviderConfig
	activeFound := false
	for i, provider := range c.Providers {
		providerName := strings.TrimSpace(provider.Name)
		if providerName == "" {
			return ProviderConfig{}, fmt.Errorf("providers[%d].name is required", i)
		}
		if _, ok := seen[providerName]; ok {
			return ProviderConfig{}, fmt.Errorf("providers contains duplicate name %q", providerName)
		}
		seen[providerName] = struct{}{}
		if err := provider.Validate(i); err != nil {
			return ProviderConfig{}, err
		}
		if providerName == name {
			active = provider
			activeFound = true
		}
	}
	if activeFound {
		return active, nil
	}
	return ProviderConfig{}, fmt.Errorf("active_provider %q is not in providers", name)
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
	if p.DefaultModel == "" {
		return fmt.Errorf("%s.default_model is required", prefix)
	}
	if len(p.Models) == 0 {
		return fmt.Errorf("%s.models is required", prefix)
	}
	seen := make(map[string]struct{}, len(p.Models))
	defaultFound := false
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
		if model.Value == p.DefaultModel {
			defaultFound = true
		}
	}
	if !defaultFound {
		return fmt.Errorf("%s.default_model %q is not in providers[%d].models", prefix, p.DefaultModel, index)
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

// ResolveModel 返回指定 value 对应的模型；value 为空时返回默认模型。
func (p ProviderConfig) ResolveModel(value string) (ProviderModel, error) {
	if strings.TrimSpace(value) == "" {
		value = p.DefaultModel
	}
	for _, model := range p.Models {
		if model.Value == value {
			return model, nil
		}
	}
	return ProviderModel{}, fmt.Errorf("provider model %q is not configured", value)
}

// ModelOptions 返回模型列表副本。
func (p ProviderConfig) ModelOptions() []ProviderModel {
	models := make([]ProviderModel, len(p.Models))
	for i, model := range p.Models {
		models[i] = model
		models[i].InputFormats = append([]string(nil), model.InputFormats...)
		models[i].ReasoningEfforts = append([]ProviderReasoningEffort(nil), model.ReasoningEfforts...)
	}
	return models
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
