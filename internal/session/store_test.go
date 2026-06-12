package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestStoreSaveAndLoadTranscript(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	messages := []model.Message{
		{Role: model.RoleUser, Content: "hello"},
		{
			Role:             model.RoleAssistant,
			Content:          "reading",
			ReasoningContent: "need file",
			ToolCalls: []model.ToolCall{{
				ID:        "call-1",
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			}},
		},
		{Role: model.RoleTool, Content: "content", ToolCallID: "call-1"},
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", messages); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}

	trans, err := store.LoadTranscript(ctx, "work")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	got := trans.Messages()
	if len(got) != len(messages) {
		t.Fatalf("messages = %d, want %d", len(got), len(messages))
	}
	if got[1].ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool calls = %#v", got[1].ToolCalls)
	}
	if got[1].ReasoningContent != "need file" {
		t.Fatalf("reasoning content = %q", got[1].ReasoningContent)
	}
	if got[2].ToolCallID != "call-1" {
		t.Fatalf("tool call id = %q", got[2].ToolCallID)
	}
}

func TestStoreLoadMissingSession(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	trans, err := store.LoadTranscript(context.Background(), "missing")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(trans.Messages()) != 0 {
		t.Fatalf("messages = %#v", trans.Messages())
	}
}

func TestStoreSaveTranscriptReplacesMessages(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if err := store.SaveTranscript(ctx, "work", "/tmp/work", []model.Message{
		{Role: model.RoleUser, Content: "old"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", []model.Message{
		{Role: model.RoleUser, Content: "new"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}

	trans, err := store.LoadTranscript(ctx, "work")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	got := trans.Messages()
	if len(got) != 1 || got[0].Content != "new" {
		t.Fatalf("messages = %#v", got)
	}
}

func TestStoreListGetAndDeleteSessions(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if err := store.SaveTranscript(ctx, "first", "/tmp/first", []model.Message{
		{Role: model.RoleUser, Content: "first title"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveTranscript(ctx, "second", "/tmp/second", []model.Message{
		{Role: model.RoleUser, Content: "second title"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}

	sessions, err := store.ListSessions(ctx, 1)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v", sessions)
	}

	info, err := store.GetSession(ctx, "first")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.ID != "first" || info.Title != "first title" || info.CWD != "/tmp/first" {
		t.Fatalf("session = %#v", info)
	}

	if err := store.DeleteSession(ctx, "first"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := store.GetSession(ctx, "first"); err == nil {
		t.Fatal("GetSession() error = nil")
	}
	trans, err := store.LoadTranscript(ctx, "first")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(trans.Messages()) != 0 {
		t.Fatalf("messages = %#v", trans.Messages())
	}
}

func TestStoreListSessionsForCWD(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if err := store.SaveTranscript(ctx, "first", "/tmp/shared", []model.Message{
		{Role: model.RoleUser, Content: "first title"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveTranscript(ctx, "second", "/tmp/other", []model.Message{
		{Role: model.RoleUser, Content: "second title"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveTranscript(ctx, "third", "/tmp/shared", []model.Message{
		{Role: model.RoleUser, Content: "third title"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}

	sessions, err := store.ListSessionsForCWD(ctx, "/tmp/shared", 10)
	if err != nil {
		t.Fatalf("ListSessionsForCWD() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v", sessions)
	}
	for _, sess := range sessions {
		if sess.CWD != "/tmp/shared" {
			t.Fatalf("session = %#v", sess)
		}
	}
}

func TestStoreRejectsMissingDelete(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	if err := store.DeleteSession(context.Background(), "missing"); err == nil {
		t.Fatal("DeleteSession() error = nil")
	}
}

func TestStoreRejectsInvalidSessionID(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.LoadTranscript(context.Background(), "../bad"); err == nil {
		t.Fatal("LoadTranscript() error = nil")
	}
	if err := store.SaveTranscript(context.Background(), "../bad", "", nil); err == nil {
		t.Fatal("SaveTranscript() error = nil")
	}
}

func TestStoreReturnsCorruptToolCallsJSON(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.db.ExecContext(ctx, `
insert into sessions(id, cwd, created_at, updated_at)
values('work', '/tmp/work', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
insert into messages(session_id, role, content, tool_calls_json, created_at)
values('work', 'assistant', 'bad', '{', 'now')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.LoadTranscript(ctx, "work"); err == nil {
		t.Fatal("LoadTranscript() error = nil")
	}
}

func TestNewID(t *testing.T) {
	id, err := NewID(time.Date(2026, 6, 8, 15, 30, 12, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if !strings.HasPrefix(id, "20260608-153012-") {
		t.Fatalf("id = %q", id)
	}
	if err := ValidateID(id); err != nil {
		t.Fatalf("ValidateID() error = %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	return store
}
