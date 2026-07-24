package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
)

func TestResumeCommandOpensPickerAndEscapeReturnsToConversation(t *testing.T) {
	m := New(Options{CWD: "/work"})
	m.input.SetValue("/resume ")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || !m.resumePicker.active() || m.resumePicker.stage != sessionPickerList || m.input.Focused() {
		t.Fatalf("opened picker state: cmd=%v active=%t stage=%d focused=%t", cmd, m.resumePicker.active(), m.resumePicker.stage, m.input.Focused())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.resumePicker.active() || !m.input.Focused() {
		t.Fatalf("escape state: active=%t focused=%t", m.resumePicker.active(), m.input.Focused())
	}
}

func TestResumeCurrentSessionShowsNotice(t *testing.T) {
	m := New(Options{SessionID: "current", CWD: "/work"})
	m.loading = false
	m.input.SetValue("/resume current")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil || m.resumePicker.active() || len(m.messages) != 1 {
		t.Fatalf("current session state: cmd=%v active=%t messages=%d", cmd, m.resumePicker.active(), len(m.messages))
	}
	if got := m.messages[0].content.String(); got != "Already in this session." || m.messages[0].noticeError {
		t.Fatalf("current session notice = %q, failed=%t", got, m.messages[0].noticeError)
	}
}

func TestResumePickerCtrlCDoesNothing(t *testing.T) {
	m := New(Options{CWD: "/work"})
	m.resumePicker.open("/work", "", testTime())
	generation := m.resumePicker.generation

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)
	if cmd != nil || !m.resumePicker.active() || m.resumePicker.generation != generation {
		t.Fatalf("ctrl+c state: cmd=%v active=%t generation=%d", cmd, m.resumePicker.active(), m.resumePicker.generation)
	}
}

func TestResumeExactSessionReplacesConversationAtomically(t *testing.T) {
	cwd := t.TempDir()
	rt := resumeTestRuntime(t, []resumeTestSession{{
		id: "target", cwd: cwd, user: "Target prompt", assistant: "Target answer", tokens: 420,
	}})

	m := New(Options{Runtime: rt, SessionID: "current", CWD: cwd})
	m.loading = false
	m.messages = []*chatMessage{newUserMessage("Current prompt")}
	m.input.SetValue("/resume target")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || m.resumePicker.stage != sessionPickerLoading || m.sessionID != "current" {
		t.Fatalf("loading state: cmd=%v stage=%d session=%q", cmd, m.resumePicker.stage, m.sessionID)
	}

	loaded := cmd()
	updated, skillCmd := m.Update(loaded)
	m = updated.(Model)
	if skillCmd == nil || m.sessionID != "target" || m.cwd != cwd || m.contextTokens != 420 || m.resumePicker.active() {
		t.Fatalf("resumed state: session=%q cwd=%q tokens=%d active=%t cmd=%v", m.sessionID, m.cwd, m.contextTokens, m.resumePicker.active(), skillCmd)
	}
	if m.showWelcome {
		t.Fatal("resumed session still shows the new-session welcome")
	}
	if m.skillsLoaded || m.skillCount != 0 || m.skillStatusErr != nil {
		t.Fatalf("resumed skill state: loaded=%t count=%d err=%v", m.skillsLoaded, m.skillCount, m.skillStatusErr)
	}
	if len(m.messages) != 2 || m.messages[0].content.String() != "Target prompt" || m.messages[1].content.String() != "Target answer" {
		t.Fatalf("resumed messages = %#v", m.messages)
	}
}

func TestResumeDifferentDirectoryRequiresConfirmation(t *testing.T) {
	currentCWD := t.TempDir()
	targetCWD := t.TempDir()
	rt := resumeTestRuntime(t, []resumeTestSession{{id: "target", cwd: targetCWD, user: "Target prompt", assistant: "Target answer"}})

	m := New(Options{Runtime: rt, SessionID: "current", CWD: currentCWD})
	m.loading = false
	m.messages = []*chatMessage{newUserMessage("Current prompt")}
	m.input.SetValue("/resume target")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.resumePicker.stage != sessionPickerConfirm || m.sessionID != "current" || m.cwd != currentCWD {
		t.Fatalf("confirmation state: stage=%d session=%q cwd=%q", m.resumePicker.stage, m.sessionID, m.cwd)
	}

	updated, skillCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if skillCmd == nil || m.sessionID != "target" || m.cwd != filepath.Clean(targetCWD) || m.resumePicker.active() {
		t.Fatalf("confirmed state: session=%q cwd=%q active=%t cmd=%v", m.sessionID, m.cwd, m.resumePicker.active(), skillCmd)
	}
}

func TestResumeMissingDirectoryKeepsCurrentSession(t *testing.T) {
	currentCWD := t.TempDir()
	missingCWD := filepath.Join(t.TempDir(), "missing")
	rt := resumeTestRuntime(t, []resumeTestSession{{id: "target", cwd: missingCWD, user: "Target", assistant: "Answer"}})

	m := New(Options{Runtime: rt, SessionID: "current", CWD: currentCWD})
	m.loading = false
	m.input.SetValue("/resume target")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	updated, applyCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if applyCmd != nil || m.sessionID != "current" || m.cwd != currentCWD || m.resumePicker.stage != sessionPickerConfirm || !strings.Contains(m.resumePicker.err, "unavailable") {
		t.Fatalf("failed confirmation state: cmd=%v session=%q cwd=%q stage=%d err=%q", applyCmd, m.sessionID, m.cwd, m.resumePicker.stage, m.resumePicker.err)
	}
}

func TestResumeLoadResultIsIgnoredAfterEscape(t *testing.T) {
	cwd := t.TempDir()
	rt := resumeTestRuntime(t, []resumeTestSession{{id: "target", cwd: cwd, user: "Target", assistant: "Answer"}})
	m := New(Options{Runtime: rt, SessionID: "current", CWD: cwd})
	m.loading = false
	m.input.SetValue("/resume target")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	loaded := cmd()

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	updated, _ = m.Update(loaded)
	m = updated.(Model)
	if m.sessionID != "current" || m.resumePicker.active() {
		t.Fatalf("stale load changed state: session=%q active=%t", m.sessionID, m.resumePicker.active())
	}
}

func TestResumeUnknownExactSessionKeepsCurrentConversation(t *testing.T) {
	cwd := t.TempDir()
	rt := resumeTestRuntime(t, nil)
	m := New(Options{Runtime: rt, SessionID: "current", CWD: cwd})
	m.loading = false
	m.messages = []*chatMessage{newUserMessage("Current prompt")}
	m.input.SetValue("/resume missing")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.sessionID != "current" || m.resumePicker.active() || len(m.messages) != 2 {
		t.Fatalf("missing session state: session=%q active=%t messages=%d", m.sessionID, m.resumePicker.active(), len(m.messages))
	}
	if rendered := m.messages[1].content.String(); !strings.Contains(rendered, "Resume failed") {
		t.Fatalf("missing session notice = %q", rendered)
	}
}

type resumeTestSession struct {
	id        string
	cwd       string
	user      string
	assistant string
	tokens    int
}

func resumeTestRuntime(t *testing.T, sessions []resumeTestSession) *runtime.Runtime {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatalf("session.Open() error = %v", err)
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	for _, saved := range sessions {
		messages := []model.Message{
			model.TextMessage(model.RoleUser, saved.user),
			{Role: model.RoleAssistant, Content: saved.assistant, Usage: model.Usage{TotalTokens: saved.tokens}},
		}
		if err := store.SaveTranscript(context.Background(), saved.id, saved.cwd, messages); err != nil {
			t.Fatalf("SaveTranscript(%q) error = %v", saved.id, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Store.Close() error = %v", err)
	}

	rt := runtime.New(runtime.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return config.Config{Session: config.SessionConfig{DBPath: dbPath}}, nil
		},
	})
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	return rt
}

func testTime() time.Time {
	return time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
}
