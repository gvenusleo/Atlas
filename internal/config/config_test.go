package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := writeTestConfig(t, `{
		"default_model": "deepseek-v4-flash",
		"providers": [
			{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"models": [
						{
							"value": "deepseek-v4-flash",
							"name": "DeepSeek V4 Flash",
							"context_window": 1000000,
							"max_tokens": 384000,
							"input_formats": ["text"],
							"prompt_cache": {"enabled": true},
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
				"models": [{"value": "gpt-5", "name": "GPT-5", "context_window": 400000, "max_tokens": 128000, "input_formats": ["text", "image"]}]
			}
		],
		"agent": {
			"max_steps": 3,
			"temperature": 0.2,
			"compaction_trigger_ratio": 0.7
		},
			"session": {
				"db_path": "/tmp/atlas.db"
			},
			"services": {
				"ws": {"host": "0.0.0.0", "port": 8765, "token": "secret"}
			}
		}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("DefaultModel = %q", cfg.DefaultModel)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers = %#v", cfg.Providers)
	}
	provider, _, err := cfg.ResolveModel("deepseek-v4-flash")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
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
	if !provider.Models[0].PromptCache.Enabled {
		t.Fatalf("PromptCache = %#v", provider.Models[0].PromptCache)
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
	if cfg.Session.DBPath != "/tmp/atlas.db" {
		t.Fatalf("Session.DBPath = %q", cfg.Session.DBPath)
	}
	if cfg.Services.WS.Token != "secret" {
		t.Fatalf("WS.Token = %q", cfg.Services.WS.Token)
	}
}

func TestLoadFileDefaults(t *testing.T) {
	base := `{
		"default_model": "deepseek-v4-flash",
		"providers": [{
			"name": "deepseek",
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
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
		provider, _, err := cfg.ResolveModel("deepseek-v4-flash")
		if err != nil {
			t.Fatalf("ResolveModel() error = %v", err)
		}
		if provider.Format != ProviderFormatChatCompletions {
			t.Fatalf("Provider.Format = %q", provider.Format)
		}
	})

	t.Run("tavily_base_url", func(t *testing.T) {
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
	})
}

func TestLoadFileRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing default model", content: `{
			"providers": [{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
			}]
		}`},
		{name: "missing provider", content: `{"default_model": "deepseek-v4-flash"}`},
		{name: "default model not configured", content: `{
			"default_model": "missing",
			"providers": [{
				"name": "deepseek",
				"base_url": "https://api.deepseek.com",
				"api_key": "sk-test",
				"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]
			}]
		}`},
		{name: "missing provider name", content: validProviderConfigWith(`"name": "",`)},
		{name: "duplicate provider name", content: `{
			"default_model": "deepseek-v4-flash",
			"providers": [
				{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
				{"name": "deepseek", "base_url": "https://api.example.com", "api_key": "sk-test", "models": [{"value": "other", "name": "Other", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
			]
		}`},
		{name: "ambiguous default model across providers", content: `{
			"default_model": "shared-model",
			"providers": [
				{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "models": [{"value": "shared-model", "name": "Shared", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
				{"name": "openai", "base_url": "https://api.openai.com/v1", "api_key": "sk-test", "models": [{"value": "shared-model", "name": "Shared Copy", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
			]
		}`},
		{name: "invalid provider format", content: validProviderConfigWith(`"format": "openai",`)},
		{name: "missing base url", content: validProviderConfigWithout("base_url")},
		{name: "invalid base url", content: validProviderConfigWith(`"base_url": ":",`)},
		{name: "missing api key", content: validProviderConfigWithout("api_key")},
		{name: "missing models", content: validProviderConfigWithout("models")},
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
		{name: "invalid provider", content: `{
			"default_model": "deepseek-v4-flash",
			"providers": [
				{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
				{"name": "broken", "base_url": ":", "api_key": "sk-test", "models": [{"value": "broken", "name": "Broken", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
			]
		}`},
		{name: "invalid temperature", content: validConfigWith(`"agent": {"temperature": 3},`)},
		{name: "invalid compaction trigger ratio", content: validConfigWith(`"agent": {"compaction_trigger_ratio": 1},`)},
		{name: "invalid tavily base url", content: validConfigWith(`"services": {"tavily": {"base_url": ":", "api_key": "tvly-test"}},`)},
		{name: "unsupported tavily base url scheme", content: validConfigWith(`"services": {"tavily": {"base_url": "ftp://api.tavily.com", "api_key": "tvly-test"}},`)},
		{name: "custom tavily base url without api key", content: validConfigWith(`"services": {"tavily": {"base_url": "https://tavily.example.com"}},`)},
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
		"default_model": "gpt-5",
		"providers": [{
			"name": "openai",
			"format": "responses",
			"base_url": "https://api.openai.com/v1",
			"api_key": "sk-test",
			"models": [{"value": "gpt-5", "name": "GPT-5", "context_window": 400000, "max_tokens": 128000, "input_formats": ["text", "image"]}]
		}]
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	provider, _, err := cfg.ResolveModel("gpt-5")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if provider.Format != ProviderFormatResponses {
		t.Fatalf("Provider.Format = %q", provider.Format)
	}
}

func TestConfigResolveModel(t *testing.T) {
	cfg := Config{
		DefaultModel: "default",
		Providers: []ProviderConfig{
			{
				Name: "provider-a",
				Models: []ProviderModel{
					{Value: "default", Name: "Default", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
				},
			},
			{
				Name: "provider-b",
				Models: []ProviderModel{
					{Value: "other", Name: "Other", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
				},
			},
		},
	}

	provider, got, err := cfg.ResolveModel("")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if got.Value != "default" {
		t.Fatalf("model = %#v", got)
	}
	if provider.Name != "provider-a" {
		t.Fatalf("provider = %q", provider.Name)
	}

	provider, got, err = cfg.ResolveModel("other")
	if err != nil {
		t.Fatalf("ResolveModel(other) error = %v", err)
	}
	if got.Name != "Other" {
		t.Fatalf("model = %#v", got)
	}
	if provider.Name != "provider-b" {
		t.Fatalf("provider = %q", provider.Name)
	}

	if _, _, err := cfg.ResolveModel("missing"); err == nil {
		t.Fatal("ResolveModel(missing) error = nil")
	}
}

func TestResolveModelCompoundFormat(t *testing.T) {
	cfg := Config{
		DefaultModel: "provider-a/shared",
		Providers: []ProviderConfig{
			{
				Name: "provider-a",
				Models: []ProviderModel{
					{Value: "shared", Name: "A Shared", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
				},
			},
			{
				Name: "provider-b",
				Models: []ProviderModel{
					{Value: "shared", Name: "B Shared", ContextWindow: 1000000, MaxTokens: 384000, InputFormats: []string{ModelInputFormatText}},
				},
			},
		},
	}

	// default_model uses provider/model format and resolves unambiguously.
	provider, got, err := cfg.ResolveModel("")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if provider.Name != "provider-a" || got.Name != "A Shared" {
		t.Fatalf("provider = %q, model = %#v", provider.Name, got)
	}

	// Explicit provider/model resolves to the correct provider.
	provider, got, err = cfg.ResolveModel("provider-b/shared")
	if err != nil {
		t.Fatalf("ResolveModel(provider-b/shared) error = %v", err)
	}
	if provider.Name != "provider-b" || got.Name != "B Shared" {
		t.Fatalf("provider = %q, model = %#v", provider.Name, got)
	}

	// Bare value is ambiguous across two providers.
	_, _, err = cfg.ResolveModel("shared")
	if err == nil || !strings.Contains(err.Error(), "multiple providers") {
		t.Fatalf("ResolveModel(shared) error = %v", err)
	}

	// Unknown provider in compound format.
	_, _, err = cfg.ResolveModel("unknown/shared")
	if err == nil || !strings.Contains(err.Error(), "provider \"unknown\" not found") {
		t.Fatalf("ResolveModel(unknown/shared) error = %v", err)
	}

	// Known provider, unknown model in compound format.
	_, _, err = cfg.ResolveModel("provider-a/missing")
	if err == nil || !strings.Contains(err.Error(), "not found in provider") {
		t.Fatalf("ResolveModel(provider-a/missing) error = %v", err)
	}
}

func TestLoadFileAllowsDuplicateModelAcrossProviders(t *testing.T) {
	path := writeTestConfig(t, `{
		"default_model": "deepseek/shared-model",
		"providers": [
			{"name": "deepseek", "base_url": "https://api.deepseek.com", "api_key": "sk-test", "models": [{"value": "shared-model", "name": "Shared", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]},
			{"name": "openai", "base_url": "https://api.openai.com/v1", "api_key": "sk-test", "models": [{"value": "shared-model", "name": "Shared Copy", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]}
		]
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	provider, model, err := cfg.ResolveModel("openai/shared-model")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if provider.Name != "openai" || model.Value != "shared-model" {
		t.Fatalf("provider = %q, model = %q", provider.Name, model.Value)
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
		"default_model": "deepseek-v4-flash",
		"providers": [{
			"name": "deepseek",
			"base_url": "https://api.deepseek.com",
			"api_key": "sk-test",
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
		"name":     `"name": "deepseek"`,
		"base_url": `"base_url": "https://api.deepseek.com"`,
		"api_key":  `"api_key": "sk-test"`,
		"models":   `"models": [{"value": "deepseek-v4-flash", "name": "DeepSeek V4 Flash", "context_window": 1000000, "max_tokens": 384000, "input_formats": ["text"]}]`,
	}
}

func providerConfigFromFields(fields map[string]string, extra string) string {
	order := []string{"name", "base_url", "api_key", "models"}
	content := `{
		"default_model": "deepseek-v4-flash",
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
