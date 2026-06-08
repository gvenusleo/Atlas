package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
}

// ProviderConfig 描述一个 OpenAI-compatible provider。
type ProviderConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// AgentConfig 描述 agent turn loop 的运行参数。
type AgentConfig struct {
	MaxSteps    int     `json:"max_steps"`
	Temperature float64 `json:"temperature"`
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

// Validate 校验配置中第一版必须明确提供的字段。
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
	if c.Provider.Model == "" {
		return fmt.Errorf("provider.model is required")
	}
	if c.Agent.Temperature < 0 || c.Agent.Temperature > 2 {
		return fmt.Errorf("agent.temperature must be between 0 and 2")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Agent.MaxSteps <= 0 {
		c.Agent.MaxSteps = defaultMaxSteps
	}
}
