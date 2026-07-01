// Package config reads Atlas's local application configuration.
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

	// ProviderFormatChatCompletions indicates the Chat Completions API format.
	ProviderFormatChatCompletions = "chat_completions"
	// ProviderFormatResponses indicates the Responses API format.
	ProviderFormatResponses = "responses"

	// ModelInputFormatText indicates the model supports text input.
	ModelInputFormatText = "text"
	// ModelInputFormatImage indicates the model supports image input.
	ModelInputFormatImage = "image"

	defaultMaxSteps               = 20
	defaultCompactionTriggerRatio = 0.8
	defaultTavilyBaseURL          = "https://api.tavily.com"
	defaultWeixinBaseURL          = "https://ilinkai.weixin.qq.com"
	defaultWeixinCDNBaseURL       = "https://novac2c.cdn.weixin.qq.com/c2c"
)

// Config is the application configuration required at Atlas CLI startup.
type Config struct {
	DefaultModel string           `json:"default_model"`
	Providers    []ProviderConfig `json:"providers"`
	Agent        AgentConfig      `json:"agent"`
	Memory       MemoryConfig     `json:"memory"`
	Session      SessionConfig    `json:"session"`
	Services     ServicesConfig   `json:"services"`
}

// ProviderConfig describes a model API provider.
type ProviderConfig struct {
	Name    string          `json:"name"`
	Format  string          `json:"format"`
	BaseURL string          `json:"base_url"`
	APIKey  string          `json:"api_key"`
	Models  []ProviderModel `json:"models"`
}

// ProviderModel describes a selectable model.
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

// PromptCacheConfig describes the prompt cache configuration for a model.
type PromptCacheConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// ProviderReasoningEffort describes a reasoning effort option supported by a model.
type ProviderReasoningEffort struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// AgentConfig describes the runtime parameters for the agent turn loop.
type AgentConfig struct {
	MaxSteps               int     `json:"max_steps"`
	Temperature            float64 `json:"temperature"`
	CompactionTriggerRatio float64 `json:"compaction_trigger_ratio"`
}

// MemoryConfig describes the long-term memory extraction and retrieval configuration.
type MemoryConfig struct {
	Model string `json:"model"`
}

// SessionConfig describes the local session storage parameters.
type SessionConfig struct {
	DBPath string `json:"db_path"`
}

// ServicesConfig describes optional external services Atlas can integrate with.
type ServicesConfig struct {
	Tavily TavilyConfig `json:"tavily"`
	Weixin WeixinConfig `json:"weixin"`
	WS     WSConfig     `json:"ws"`
}

// TavilyConfig describes the Tavily search and web extraction service configuration.
type TavilyConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// WeixinConfig describes the WeChat remote control channel configuration.
type WeixinConfig struct {
	BaseURL    string `json:"base_url"`
	CDNBaseURL string `json:"cdn_base_url"`
}

// WSConfig describes the WebSocket channel configuration.
type WSConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// DefaultPath returns the Atlas configuration path under the current user's home directory.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// LoadDefault reads the configuration file from the default path.
func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return LoadFile(path)
}

// LoadFile reads and validates the configuration from the specified JSON file.
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

// Validate validates the configuration fields required for Atlas to run.
func (c Config) Validate() error {
	if err := c.validateProviders(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Memory.Model) != "" {
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

// validateProviders validates all provider configurations and checks that model values are globally unique across providers.
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

// ResolveModel finds the model with the specified value across all providers, returning the provider and model.
// When value is empty, returns the model corresponding to default_model.
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

// Validate validates a single provider configuration.
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

// validateModelInputFormats validates the model input format declarations.
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

// validateModelReasoningEfforts validates the model-level reasoning effort declarations.
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

// AllModels returns all models from all providers, each with its owning provider.
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

// ProviderModelInfo contains a model definition and its owning provider.
type ProviderModelInfo struct {
	Provider ProviderConfig
	Model    ProviderModel
}

// SupportsReasoningEffort returns whether the model declares support for the specified reasoning effort.
func (m ProviderModel) SupportsReasoningEffort(value string) bool {
	for _, effort := range m.ReasoningEfforts {
		if effort.Value == value {
			return true
		}
	}
	return false
}

// SupportsInputFormat returns whether the model declares support for the specified input format.
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
