package runtime

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/session"
)

func TestCreateDefaultThinkingLevel(t *testing.T) {
	for _, tt := range []struct {
		name  string
		model config.Model
		want  agent.ThinkingLevel
	}{
		{
			name: "first configured effort",
			model: config.Model{Reasoning: true, ReasoningOptions: []config.ReasoningOption{
				{Type: config.ReasoningTypeToggle},
				{Type: config.ReasoningTypeEffort},
				{Type: config.ReasoningTypeEffort, Values: []string{"high", "low"}},
				{Type: config.ReasoningTypeEffort, Values: []string{"medium"}},
			}},
			want: agent.ThinkingHigh,
		},
		{
			name: "off is a configured level",
			model: config.Model{Reasoning: true, ReasoningOptions: []config.ReasoningOption{
				{Type: config.ReasoningTypeEffort, Values: []string{"off", "high"}},
			}},
			want: agent.ThinkingOff,
		},
		{name: "no reasoning", model: config.Model{}},
		{name: "no configured levels", model: config.Model{Reasoning: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("ATLAS_TEST_API_KEY", "test-key")
			dir := filepath.Join(home, ".atlas")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			settings := config.Settings{Model: "atlas_test/model", Providers: map[string]config.Provider{
				"atlas_test": {
					WireAPI: config.WireAPIOpenAIResponses,
					BaseURL: "http://localhost:1/v1",
					Models:  map[string]config.Model{"model": tt.model},
				},
			}}
			data, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := session.Open(t.Context(), filepath.Join(home, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			s, err := New(store).Create(t.Context(), home)
			if err != nil {
				t.Fatal(err)
			}
			info, err := store.Get(t.Context(), s.ID())
			if err != nil {
				t.Fatal(err)
			}
			if info.ThinkingLevel != tt.want || s.loop.ThinkingLevel != tt.want {
				t.Fatalf("thinking level: stored=%q, runtime=%q; want %q", info.ThinkingLevel, s.loop.ThinkingLevel, tt.want)
			}
			resumed, err := New(store).Resume(t.Context(), s.ID())
			if err != nil {
				t.Fatal(err)
			}
			if resumed.loop.ThinkingLevel != tt.want {
				t.Fatalf("resumed thinking level=%q; want %q", resumed.loop.ThinkingLevel, tt.want)
			}
		})
	}
}
