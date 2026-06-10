package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := writeTestConfig(t, `{
		"provider": {
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
			"default_model": "deepseek-v4-flash",
			"models": [
				{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_length": 64000},
				{"value": "deepseek-v4-pro", "name": "DeepSeek V4 Pro", "description": "pro model"}
			]
		},
		"agent": {
			"max_steps": 3,
			"temperature": 0.2
		},
		"session": {
			"db_path": "/tmp/atlas.db"
		}
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL = %q", cfg.Provider.BaseURL)
	}
	if cfg.Provider.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q", cfg.Provider.APIKey)
	}
	if cfg.Provider.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("DefaultModel = %q", cfg.Provider.DefaultModel)
	}
	if len(cfg.Provider.Models) != 2 {
		t.Fatalf("Models = %#v", cfg.Provider.Models)
	}
	if cfg.Provider.Models[0].ContextLength != 64000 {
		t.Fatalf("ContextLength = %d", cfg.Provider.Models[0].ContextLength)
	}
	if cfg.Agent.MaxSteps != 3 {
		t.Fatalf("MaxSteps = %d", cfg.Agent.MaxSteps)
	}
	if cfg.Agent.Temperature != 0.2 {
		t.Fatalf("Temperature = %f", cfg.Agent.Temperature)
	}
	if cfg.Session.DBPath != "/tmp/atlas.db" {
		t.Fatalf("Session.DBPath = %q", cfg.Session.DBPath)
	}
}

func TestLoadFileDefaultsMaxSteps(t *testing.T) {
	path := writeTestConfig(t, `{
		"provider": {
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
			"default_model": "deepseek-v4-flash",
			"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
		}
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Agent.MaxSteps != defaultMaxSteps {
		t.Fatalf("MaxSteps = %d", cfg.Agent.MaxSteps)
	}
}

func TestLoadFileRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "missing base url",
			content: `{
				"provider": {
					"api_key": "sk-test",
					"default_model": "deepseek-v4-flash",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				}
			}`,
		},
		{
			name: "invalid base url",
			content: `{
				"provider": {
					"base_url": ":",
					"api_key": "sk-test",
					"default_model": "deepseek-v4-flash",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				}
			}`,
		},
		{
			name: "missing api key",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"default_model": "deepseek-v4-flash",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				}
			}`,
		},
		{
			name: "missing default model",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"api_key": "sk-test",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				}
			}`,
		},
		{
			name: "missing models",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"api_key": "sk-test",
					"default_model": "deepseek-v4-flash"
				}
			}`,
		},
		{
			name: "default model not configured",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"api_key": "sk-test",
					"default_model": "deepseek-v4-pro",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				}
			}`,
		},
		{
			name: "duplicate model value",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"api_key": "sk-test",
					"default_model": "deepseek-v4-flash",
					"models": [
						{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"},
						{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash Copy"}
					]
				}
			}`,
		},
		{
			name: "invalid temperature",
			content: `{
				"provider": {
					"base_url": "https://api.deepseek.com",
					"api_key": "sk-test",
					"default_model": "deepseek-v4-flash",
					"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash"}]
				},
				"agent": {"temperature": 3}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.content)
			if _, err := LoadFile(path); err == nil {
				t.Fatal("LoadFile() error = nil")
			}
		})
	}
}

func TestProviderConfigResolveModel(t *testing.T) {
	provider := ProviderConfig{
		DefaultModel: "default",
		Models: []ProviderModel{
			{Value: "default", Name: "Default"},
			{Value: "other", Name: "Other"},
		},
	}

	got, err := provider.ResolveModel("")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if got.Value != "default" {
		t.Fatalf("model = %#v", got)
	}

	got, err = provider.ResolveModel("other")
	if err != nil {
		t.Fatalf("ResolveModel(other) error = %v", err)
	}
	if got.Name != "Other" {
		t.Fatalf("model = %#v", got)
	}

	if _, err := provider.ResolveModel("missing"); err == nil {
		t.Fatal("ResolveModel(missing) error = nil")
	}
}

func TestLoadFileRejectsInvalidJSON(t *testing.T) {
	path := writeTestConfig(t, `{`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("LoadFile() error = nil")
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
