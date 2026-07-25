package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
)

func TestNewCommandStartsFreshSessionInCurrentDirectory(t *testing.T) {
	cwd := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatalf("session.Open() error = %v", err)
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	oldMessages := []model.Message{
		model.TextMessage(model.RoleUser, "Old prompt"),
		model.TextMessage(model.RoleAssistant, "Old answer"),
	}
	if err := store.SaveTranscript(context.Background(), "current", cwd, oldMessages); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Store.Close() error = %v", err)
	}

	provider := &tuiRecordingProvider{response: model.ChatResponse{Content: "New answer"}}
	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{
				DefaultModel: "provider/model-a",
				Providers: []config.ProviderConfig{{
					Name: "provider",
					Models: []config.ProviderModel{{
						Value: "model-a", ContextWindow: 1000, MaxTokens: 100,
						InputFormats:     []string{config.ModelInputFormatText},
						ReasoningEfforts: []config.ProviderReasoningEffort{{Value: "high", Name: "High"}},
					}},
				}},
				Agent:   config.AgentConfig{MaxSteps: 2},
				Session: config.SessionConfig{DBPath: dbPath},
			}, nil
		},
		NewProvider: func(config.ProviderConfig, config.ProviderModel) (model.Provider, error) {
			return provider, nil
		},
		NewSessionID: func(time.Time) (string, error) { return "fresh", nil },
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	m := New(Options{Runtime: rt, SessionID: "current", CWD: cwd})
	m.loading = false
	m.showWelcome = false
	m.messages = []*chatMessage{newUserMessage("Visible old prompt")}
	m.current = newAssistantMessage()
	m.messages = append(m.messages, m.current)
	m.contextTokens = 790
	m.contextWindow = 1000
	m.modelValue = "provider/model-a"
	m.modelName = "Model A"
	m.reasoningEffort = "high"
	m.skillsLoaded = true
	m.skillCount = 1
	m.slashPopup.setSkills([]runtime.SkillSummary{{Name: "think", Description: "Plan work"}})
	m.input.SetValue("/new")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.sessionID != "" || len(m.messages) != 0 || m.current != nil || !m.showWelcome || m.contextTokens != 0 {
		t.Fatalf("new session state: cmd=%v id=%q messages=%d current=%v welcome=%t tokens=%d", cmd, m.sessionID, len(m.messages), m.current, m.showWelcome, m.contextTokens)
	}
	if m.cwd != cwd || m.modelValue != "provider/model-a" || m.modelName != "Model A" || m.reasoningEffort != "high" || m.contextWindow != 1000 {
		t.Fatalf("preserved state: cwd=%q model=%q/%q effort=%q window=%d", m.cwd, m.modelValue, m.modelName, m.reasoningEffort, m.contextWindow)
	}
	if !m.input.Focused() || m.input.Value() != "" || !m.skillsLoaded || m.skillCount != 1 || !slashCatalogContains(m.slashPopup.commands, "think") {
		t.Fatalf("input or skill state: focused=%t value=%q loaded=%t count=%d commands=%+v", m.input.Focused(), m.input.Value(), m.skillsLoaded, m.skillCount, m.slashPopup.commands)
	}
	_, persisted, err := rt.ShowSession(t.Context(), "current")
	if err != nil || len(persisted.Messages()) != len(oldMessages) {
		t.Fatalf("old session after /new: messages=%d err=%v", len(persisted.Messages()), err)
	}

	m.input.SetValue("New prompt")
	updated, turnCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	batch, ok := turnCmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("new turn command = %T with %d entries", batch, len(batch))
	}
	batch[1]()
	var turnErr error
	for update := range m.eventCh {
		if update.done != nil {
			turnErr = update.done.err
		}
	}
	if turnErr != nil {
		t.Fatalf("new turn error = %v", turnErr)
	}
	if provider.request.SessionID != "fresh" {
		t.Fatalf("new turn session ID = %q, want fresh", provider.request.SessionID)
	}
	if m.turnCancel != nil {
		m.turnCancel()
	}
	if m.turnAbandon != nil {
		m.turnAbandon()
	}
}

func TestNewCommandRequiresExactInput(t *testing.T) {
	m := New(Options{SessionID: "current"})
	m.loading = false
	m.input.SetValue("/new with context")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || !m.turnActive || m.sessionID != "current" || len(m.messages) != 2 || m.messages[0].content.String() != "/new with context" {
		t.Fatalf("non-exact /new state: cmd=%v active=%t id=%q messages=%d", cmd, m.turnActive, m.sessionID, len(m.messages))
	}
	if m.turnCancel != nil {
		m.turnCancel()
	}
	if m.turnAbandon != nil {
		m.turnAbandon()
	}
}

// slashCatalogContains reports whether a command name remains available after reset.
func slashCatalogContains(commands []slashCommand, name string) bool {
	for _, command := range commands {
		if command.name == name {
			return true
		}
	}
	return false
}
