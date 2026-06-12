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

	defaultMaxSteps               = 8
	defaultCompactionTriggerRatio = 0.8
	defaultTavilyBaseURL          = "https://api.tavily.com"
	defaultWeixinBaseURL          = "https://ilinkai.weixin.qq.com"
)

// Config 是 Atlas CLI 启动时需要的应用配置。
type Config struct {
	Provider ProviderConfig `json:"provider"`
	Agent    AgentConfig    `json:"agent"`
	Session  SessionConfig  `json:"session"`
	Services ServicesConfig `json:"services"`
}

// ProviderConfig 描述一个 OpenAI-compatible provider。
type ProviderConfig struct {
	BaseURL      string          `json:"base_url"`
	APIKey       string          `json:"api_key"`
	DefaultModel string          `json:"default_model"`
	Models       []ProviderModel `json:"models"`
}

// ProviderModel 描述一个可选模型。
type ProviderModel struct {
	Value         string `json:"value"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
}

// AgentConfig 描述 agent turn loop 的运行参数。
type AgentConfig struct {
	MaxSteps               int     `json:"max_steps"`
	Temperature            float64 `json:"temperature"`
	ReasoningEffort        string  `json:"reasoning_effort"`
	CompactionTriggerRatio float64 `json:"compaction_trigger_ratio"`
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
	DefaultCWD string `json:"default_cwd"`
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
	if c.Provider.BaseURL == "" {
		return fmt.Errorf("provider.base_url is required")
	}
	baseURL, err := url.Parse(c.Provider.BaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return fmt.Errorf("provider.base_url is invalid")
	}
	if c.Provider.APIKey == "" {
		return fmt.Errorf("provider.api_key is required")
	}
	if c.Provider.DefaultModel == "" {
		return fmt.Errorf("provider.default_model is required")
	}
	if len(c.Provider.Models) == 0 {
		return fmt.Errorf("provider.models is required")
	}
	seen := make(map[string]struct{}, len(c.Provider.Models))
	defaultFound := false
	for i, model := range c.Provider.Models {
		if strings.TrimSpace(model.Value) == "" {
			return fmt.Errorf("provider.models[%d].value is required", i)
		}
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("provider.models[%d].name is required", i)
		}
		if model.ContextWindow <= 0 {
			return fmt.Errorf("provider.models[%d].context_window must be positive", i)
		}
		if model.MaxTokens <= 0 {
			return fmt.Errorf("provider.models[%d].max_tokens must be positive", i)
		}
		if model.MaxTokens > model.ContextWindow {
			return fmt.Errorf("provider.models[%d].max_tokens must be less than or equal to context_window", i)
		}
		if _, ok := seen[model.Value]; ok {
			return fmt.Errorf("provider.models contains duplicate value %q", model.Value)
		}
		seen[model.Value] = struct{}{}
		if model.Value == c.Provider.DefaultModel {
			defaultFound = true
		}
	}
	if !defaultFound {
		return fmt.Errorf("provider.default_model %q is not in provider.models", c.Provider.DefaultModel)
	}
	if c.Agent.Temperature < 0 || c.Agent.Temperature > 2 {
		return fmt.Errorf("agent.temperature must be between 0 and 2")
	}
	if c.Agent.CompactionTriggerRatio <= 0 || c.Agent.CompactionTriggerRatio >= 1 {
		return fmt.Errorf("agent.compaction_trigger_ratio must be greater than 0 and less than 1")
	}
	if !validReasoningEffort(c.Agent.ReasoningEffort) {
		return fmt.Errorf("agent.reasoning_effort must be empty, high, or max")
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
	copy(models, p.Models)
	return models
}

func (c *Config) applyDefaults() {
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
}

func isHTTPURL(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

func validReasoningEffort(effort string) bool {
	switch effort {
	case "", "high", "max":
		return true
	default:
		return false
	}
}
