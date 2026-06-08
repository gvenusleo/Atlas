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
			"model": "deepseek-v4-flash"
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
	if cfg.Provider.Model != "deepseek-v4-flash" {
		t.Fatalf("Model = %q", cfg.Provider.Model)
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
			"model": "deepseek-v4-flash"
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
				"provider": {"api_key": "sk-test", "model": "deepseek-v4-flash"}
			}`,
		},
		{
			name: "invalid base url",
			content: `{
				"provider": {"base_url": ":", "api_key": "sk-test", "model": "deepseek-v4-flash"}
			}`,
		},
		{
			name: "missing api key",
			content: `{
				"provider": {"base_url": "https://api.deepseek.com", "model": "deepseek-v4-flash"}
			}`,
		},
		{
			name: "missing model",
			content: `{
				"provider": {"base_url": "https://api.deepseek.com", "api_key": "sk-test"}
			}`,
		},
		{
			name: "invalid temperature",
			content: `{
				"provider": {"base_url": "https://api.deepseek.com", "api_key": "sk-test", "model": "deepseek-v4-flash"},
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
