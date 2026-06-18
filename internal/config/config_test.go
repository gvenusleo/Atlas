package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := writeTestConfig(t, `{
		"active_provider": "deepseek",
		"providers": [
			{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"default_model": "deepseek-v4-flash",
				"models": [
					{
						"value": "deepseek-v4-flash",
						"name": "DeepSeek V4 Flash",
						"context_window": 1000000,
						"max_tokens": 384000,
						"input_formats": ["text"],
						"reasoning_efforts": [
							{"value": "high", "name": "High"},
							{"value": "max", "name": "Max", "description": "Maximum reasoning depth"}
						]
					},
					{"value": "deepseek-v4-pro", "name": "DeepSeek V4 Pro", "description": "pro model", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}
				]
			},
			{
				"name": "openai",
				"format": "responses",
				"base_url": "https://api.openai.com/v1",
				"api_key": "sk-openai",
				"default_model": "gpt-5",
				"models": [{"value": "gpt-5", "name": "GPT-5", "context_window": 400000, "max_tokens": 128000, "input_formats": ["text", "image"]}]
			}
		],
		"agent": {
			"max_steps": 3,
			"temperature": 0.2,
			"compaction_trigger_ratio": 0.7
		},
		"memory": {
			"enabled": false,
			"model": "deepseek-v4-pro"
		},
		"session": {
			"db_path": "/tmp/atlas.db"
		}
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.ActiveProvider != "deepseek" {
		t.Fatalf("ActiveProvider = %q", cfg.ActiveProvider)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers = %#v", cfg.Providers)
	}
	provider, err := cfg.ActiveProviderConfig()
	if err != nil {
		t.Fatalf("ActiveProviderConfig() error = %v", err)
	}
	if provider.Name != "deepseek" {
		t.Fatalf("provider name = %q", provider.Name)
	}
	if provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL = %q", provider.BaseURL)
	}
	if provider.Format != ProviderFormatChatCompletions {
		t.Fatalf("Format = %q", provider.Format)
	}
	if provider.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q", provider.APIKey)
	}
	if provider.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("DefaultModel = %q", provider.DefaultModel)
	}
	if len(provider.Models) != 2 {
		t.Fatalf("Models = %#v", provider.Models)
	}
	if provider.Models[0].ContextWindow != 1000000 {
		t.Fatalf("ContextWindow = %d", provider.Models[0].ContextWindow)
	}
	if provider.Models[0].MaxTokens != 384000 {
		t.Fatalf("MaxTokens = %d", provider.Models[0].MaxTokens)
	}
	if len(provider.Models[0].InputFormats) != 1 || provider.Models[0].InputFormats[0] != ModelInputFormatText {
		t.Fatalf("InputFormats = %#v", provider.Models[0].InputFormats)
	}
	if len(provider.Models[0].ReasoningEfforts) != 2 || provider.Models[0].ReasoningEfforts[1].Description != "Maximum reasoning depth" {
		t.Fatalf("ReasoningEfforts = %#v", provider.Models[0].ReasoningEfforts)
	}
	if cfg.Agent.MaxSteps != 3 {
		t.Fatalf("MaxSteps = %d", cfg.Agent.MaxSteps)
	}
	if cfg.Agent.Temperature != 0.2 {
		t.Fatalf("Temperature = %f", cfg.Agent.Temperature)
	}
	if cfg.Agent.CompactionTriggerRatio != 0.7 {
		t.Fatalf("CompactionTriggerRatio = %f", cfg.Agent.CompactionTriggerRatio)
	}
	if cfg.Memory.IsEnabled() {
		t.Fatal("Memory.IsEnabled() = true")
	}
	if cfg.Memory.Model != "deepseek-v4-pro" {
		t.Fatalf("Memory.Model = %q", cfg.Memory.Model)
	}
	if cfg.Session.DBPath != "/tmp/atlas.db" {
		t.Fatalf("Session.DBPath = %q", cfg.Session.DBPath)
	}
}

func TestLoadFileDefaults(t *testing.T) {
	base := `{
		"active_provider": "deepseek",
		"providers": [{
			"name": "deepseek",
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
			"default_model": "deepseek-v4-flash",
			"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
		}]
	}`

	t.Run("max_steps_and_compaction_ratio", func(t *testing.T) {
		cfg, err := LoadFile(writeTestConfig(t, base))
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		if cfg.Agent.MaxSteps != defaultMaxSteps {
			t.Fatalf("MaxSteps = %d", cfg.Agent.MaxSteps)
		}
		if cfg.Agent.CompactionTriggerRatio != defaultCompactionTriggerRatio {
			t.Fatalf("CompactionTriggerRatio = %f", cfg.Agent.CompactionTriggerRatio)
		}
		if !cfg.Memory.IsEnabled() {
			t.Fatal("Memory.IsEnabled() = false")
		}
		provider, err := cfg.ActiveProviderConfig()
		if err != nil {
			t.Fatalf("ActiveProviderConfig() error = %v", err)
		}
		if provider.Format != ProviderFormatChatCompletions {
			t.Fatalf("Provider.Format = %q", provider.Format)
		}
	})

	t.Run("disabled_memory_allows_unknown_model", func(t *testing.T) {
		content := base[:len(base)-1] + `,
		"memory": {"enabled": false, "model": "missing-model"}
	}`
		cfg, err := LoadFile(writeTestConfig(t, content))
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		if cfg.Memory.IsEnabled() {
			t.Fatal("Memory.IsEnabled() = true")
		}
	})

	t.Run("tavily_and_weixin_base_url", func(t *testing.T) {
		content := base[:len(base)-1] + `,
		"services": {"tavily": {"api_key": "tvly-test"}}
	}`
		cfg, err := LoadFile(writeTestConfig(t, content))
		if err != nil {
			t.Fatalf("LoadFile() error = %v", err)
		}
		if cfg.Services.Tavily.BaseURL != defaultTavilyBaseURL {
			t.Fatalf("Tavily.BaseURL = %q", cfg.Services.Tavily.BaseURL)
		}
		if cfg.Services.Tavily.APIKey != "tvly-test" {
			t.Fatalf("Tavily.APIKey = %q", cfg.Services.Tavily.APIKey)
		}
		if cfg.Services.Weixin.BaseURL != defaultWeixinBaseURL {
			t.Fatalf("Weixin.BaseURL = %q", cfg.Services.Weixin.BaseURL)
		}
		if cfg.Services.Weixin.CDNBaseURL != defaultWeixinCDNBaseURL {
			t.Fatalf("Weixin.CDNBaseURL = %q", cfg.Services.Weixin.CDNBaseURL)
		}
	})
}

func TestLoadFileRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing active provider", content: `{
			"active_provider": "",
			"providers": [{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"default_model": "deepseek-v4-flash",
				"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
			}]
		}`},
		{name: "missing provider", content: `{"active_provider": "deepseek"}`},
		{name: "active provider not configured", content: `{
			"active_provider": "missing",
			"providers": [{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"default_model": "deepseek-v4-flash",
				"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
			}]
		}`},
		{name: "missing provider name", content: validProviderConfigWith(`"name": "",`)},
		{name: "duplicate provider name", content: `{
			"active_provider": "deepseek",
			"providers": [
				{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "default_model": "deepseek-v4-flash", "models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
				{"name": "deepseek", "base_url": "https://api.example.com", "api_key": "sk-test", "default_model": "other", "models": [{"value": "other", "name": "Other", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
			]
		}`},
		{name: "invalid provider format", content: validProviderConfigWith(`"format": "openai",`)},
		{name: "missing base url", content: validProviderConfigWithout("base_url")},
		{name: "invalid base url", content: validProviderConfigWith(`"base_url": ":",`)},
		{name: "missing api key", content: validProviderConfigWithout("api_key")},
		{name: "missing default model", content: validProviderConfigWithout("default_model")},
		{name: "missing models", content: validProviderConfigWithout("models")},
		{name: "default model not configured", content: validProviderConfigWith(`"default_model": "deepseek-v4-pro",`)},
		{name: "duplicate model value", content: validProviderConfigWith(`"models": [
			{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]},
			{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash Copy", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}
		],`)},
		{name: "missing context window", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "max_tokens": 384000}],`)},
		{name: "missing max tokens", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000}],`)},
		{name: "max tokens exceed context window", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 1000001}],`)},
		{name: "missing input formats", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000}],`)},
		{name: "empty input format", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": [""]}],`)},
		{name: "unsupported input format", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text", "audio"]}],`)},
		{name: "input formats missing text", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["image"]}],`)},
		{name: "duplicate input format", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text", "text"]}],`)},
		{name: "missing reasoning effort value", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"], "reasoning_efforts": [{"name": "High"}]}],`)},
		{name: "missing reasoning effort name", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"], "reasoning_efforts": [{"value": "high"}]}],`)},
		{name: "duplicate reasoning effort value", content: validProviderConfigWith(`"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"], "reasoning_efforts": [{"value": "high", "name": "High"}, {"value": "high", "name": "High Copy"}]}],`)},
		{name: "invalid inactive provider", content: `{
			"active_provider": "deepseek",
			"providers": [
				{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "default_model": "deepseek-v4-flash", "models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
				{"name": "broken", "base_url": ":", "api_key": "sk-test", "default_model": "broken", "models": [{"value": "broken", "name": "Broken", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
			]
		}`},
		{name: "invalid temperature", content: validConfigWith(`"agent": {"temperature": 3},`)},
		{name: "invalid compaction trigger ratio", content: validConfigWith(`"agent": {"compaction_trigger_ratio": 1},`)},
		{name: "memory model not configured", content: validConfigWith(`"memory": {"model": "missing"},`)},
		{name: "invalid tavily base url", content: validConfigWith(`"services": {"tavily": {"base_url": ":", "api_key": "tvly-test"}},`)},
		{name: "unsupported tavily base url scheme", content: validConfigWith(`"services": {"tavily": {"base_url": "ftp://api.tavily.com", "api_key": "tvly-test"}},`)},
		{name: "custom tavily base url without api key", content: validConfigWith(`"services": {"tavily": {"base_url": "https://tavily.example.com"}},`)},
		{name: "invalid weixin base url", content: validConfigWith(`"services": {"weixin": {"base_url": ":"}},`)},
		{name: "unsupported weixin base url scheme", content: validConfigWith(`"services": {"weixin": {"base_url": "ftp://ilinkai.weixin.qq.com"}},`)},
		{name: "invalid weixin cdn base url", content: validConfigWith(`"services": {"weixin": {"cdn_base_url": ":"}},`)},
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

func TestLoadFileAllowsProviderResponsesFormat(t *testing.T) {
	path := writeTestConfig(t, `{
		"active_provider": "openai",
		"providers": [{
			"name": "openai",
			"format": "responses",
			"base_url": "https://api.openai.com/v1",
			"api_key": "sk-test",
			"default_model": "gpt-5",
			"models": [{"value": "gpt-5", "name": "GPT-5", "context_window": 400000, "max_tokens": 128000, "input_formats": ["text", "image"]}]
		}]
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	provider, err := cfg.ActiveProviderConfig()
	if err != nil {
		t.Fatalf("ActiveProviderConfig() error = %v", err)
	}
	if provider.Format != ProviderFormatResponses {
		t.Fatalf("Provider.Format = %q", provider.Format)
	}
}

func TestProviderConfigResolveModel(t *testing.T) {
	provider := ProviderConfig{
		DefaultModel: "default",
		Models: []ProviderModel{
			{Value: "default", Name: "Default", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
			{Value: "other", Name: "Other", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
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

func validConfigWith(extra string) string {
	return `{
		` + extra + `
		"active_provider": "deepseek",
		"providers": [{
			"name": "deepseek",
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
			"default_model": "deepseek-v4-flash",
			"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
		}]
	}`
}

func validProviderConfigWith(override string) string {
	fields := validProviderFields()
	for key := range fields {
		if strings.Contains(override, `"`+key+`"`) {
			delete(fields, key)
		}
	}
	return providerConfigFromFields(fields, override)
}

func validProviderConfigWithout(field string) string {
	fields := validProviderFields()
	delete(fields, field)
	return providerConfigFromFields(fields, "")
}

func validProviderFields() map[string]string {
	return map[string]string{
		"name":          `"name": "deepseek"`,
		"base_url":      `"base_url": "https://api.deepseek.com"`,
		"api_key":       `"api_key": "sk-test"`,
		"default_model": `"default_model": "deepseek-v4-flash"`,
		"models":        `"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]`,
	}
}

func providerConfigFromFields(fields map[string]string, extra string) string {
	order := []string{"name", "base_url", "api_key", "default_model", "models"}
	content := `{
		"active_provider": "deepseek",
		"providers": [{`
	first := true
	if strings.TrimSpace(extra) != "" {
		content += "\n" + strings.TrimRight(strings.TrimSpace(extra), ",")
		first = false
	}
	for _, key := range order {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if !first {
			content += ","
		}
		content += "\n" + value
		first = false
	}
	return content + `
		}]
	}`
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
