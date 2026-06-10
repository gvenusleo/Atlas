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

	defaultMaxSteps = 8
)

// Config 是 Atlas CLI 启动时需要的应用配置。
type Config struct {
	Provider ProviderConfig `json:"provider"`
	Agent    AgentConfig    `json:"agent"`
	Session  SessionConfig  `json:"session"`
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
	ContextLength int    `json:"context_length,omitempty"`
}

// AgentConfig 描述 agent turn loop 的运行参数。
type AgentConfig struct {
	MaxSteps    int     `json:"max_steps"`
	Temperature float64 `json:"temperature"`
}

// SessionConfig 描述本地会话存储参数。
type SessionConfig struct {
	DBPath string `json:"db_path"`
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
		if model.ContextLength < 0 {
			return fmt.Errorf("provider.models[%d].context_length must be non-negative", i)
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
}
