package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
)

func TestRunRequiresPrompt(t *testing.T) {
	err := run(context.Background(), nil)
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if !strings.Contains(err.Error(), "usage: atlas <prompt>") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunWithDependenciesPassesDefaultSystemPrompt(t *testing.T) {
	provider := &recordingProvider{
		response: model.ChatResponse{Content: "ok"},
	}
	var stdout bytes.Buffer

	err := runWithDependencies(context.Background(), []string{"hello"}, runDependencies{
		loadConfig: func() (config.Config, error) {
			return config.Config{
				Provider: config.ProviderConfig{
					BaseURL: "https://api.example.com",
					APIKey:  "sk-test",
					Model:   "test-model",
				},
				Agent: config.AgentConfig{
					MaxSteps:    4,
					Temperature: 0.2,
				},
			}, nil
		},
		newProvider: func(config.ProviderConfig) (model.Provider, error) {
			return provider, nil
		},
		getwd: func() (string, error) { return "/tmp/atlas-work", nil },
		loadInstructions: func(string) ([]prompt.InstructionFile, error) {
			return []prompt.InstructionFile{
				{Path: "/tmp/atlas-work/AGENTS.md", Content: "project rules"},
			}, nil
		},
		now:    func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
		stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("runWithDependencies() error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "ok" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(provider.request.System, "You are Atlas") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if !strings.Contains(provider.request.System, "Working directory: /tmp/atlas-work") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if !strings.Contains(provider.request.System, "project rules") {
		t.Fatalf("system prompt = %q", provider.request.System)
	}
	if provider.request.Temperature != 0.2 {
		t.Fatalf("temperature = %f", provider.request.Temperature)
	}
	if len(provider.request.Tools) != 5 {
		t.Fatalf("tools = %d", len(provider.request.Tools))
	}
}

func TestPrintEventWritesToolStatus(t *testing.T) {
	var out bytes.Buffer
	observer := printEvent(&out)

	observer(agentEventToolStarted("read_file"))
	observer(agentEventToolFinished("read_file", true))

	got := out.String()
	if !strings.Contains(got, "[tool] read_file") {
		t.Fatalf("output = %q", got)
	}
	if !strings.Contains(got, "[tool failed] read_file") {
		t.Fatalf("output = %q", got)
	}
}

func agentEventToolStarted(name string) agent.Event {
	return agent.Event{
		Type:     agent.EventToolStarted,
		ToolCall: model.ToolCall{Name: name},
	}
}

func agentEventToolFinished(name string, failed bool) agent.Event {
	return agent.Event{
		Type:      agent.EventToolFinished,
		ToolCall:  model.ToolCall{Name: name},
		ToolError: failed,
	}
}

type recordingProvider struct {
	request  model.ChatRequest
	response model.ChatResponse
}

func (p *recordingProvider) Chat(_ context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	p.request = req
	return p.response, nil
}
