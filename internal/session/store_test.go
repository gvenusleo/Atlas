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
	oldText := "old"

	messages := []model.Message{
		{
			Role:    model.RoleUser,
			Content: "hello",
			Parts: []model.ContentPart{
				{Type: model.ContentPartText, Text: "hello"},
				{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,aGVsbG8=", Detail: model.ImageDetailAuto},
			},
		},
		{
			Role:             model.RoleAssistant,
			Content:          "reading",
			ReasoningContent: "need file",
			Usage:            model.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
			ProviderItems: []model.ProviderItem{{
				Type: "responses",
				JSON: `{"type":"reasoning","id":"rs_1","summary":[]}`,
			}},
			ToolCalls: []model.ToolCall{{
				ID:        "call-1",
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			}},
		},
		{
			Role:       model.RoleTool,
			Content:    "content",
			ToolCallID: "call-1",
			ToolMetadata: model.ToolMetadata{
				Locations: []model.ToolLocation{{Path: "/tmp/work/README.md", Line: 2}},
				Diff:      &model.ToolDiff{Path: "/tmp/work/README.md", OldText: &oldText, NewText: "new"},
			},
		},
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
	if len(got[0].Parts) != 2 || got[0].Parts[1].Type != model.ContentPartImage || got[0].Parts[1].MimeType != "image/png" {
		t.Fatalf("parts = %#v", got[0].Parts)
	}
	if got[1].ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool calls = %#v", got[1].ToolCalls)
	}
	if got[1].ReasoningContent != "need file" {
		t.Fatalf("reasoning content = %q", got[1].ReasoningContent)
	}
	if got[1].Usage != (model.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 13}) {
		t.Fatalf("usage = %#v", got[1].Usage)
	}
	if len(got[1].ProviderItems) != 1 || got[1].ProviderItems[0].Type != "responses" {
		t.Fatalf("provider items = %#v", got[1].ProviderItems)
	}
	if got[2].ToolCallID != "call-1" {
		t.Fatalf("tool call id = %q", got[2].ToolCallID)
	}
	if len(got[2].ToolMetadata.Locations) != 1 || got[2].ToolMetadata.Locations[0].Path != "/tmp/work/README.md" || got[2].ToolMetadata.Locations[0].Line != 2 {
		t.Fatalf("tool locations = %#v", got[2].ToolMetadata.Locations)
	}
	if got[2].ToolMetadata.Diff == nil || got[2].ToolMetadata.Diff.NewText != "new" {
		t.Fatalf("tool diff = %#v", got[2].ToolMetadata.Diff)
	}
	info, err := store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.LastInputTokens != 10 || info.LastOutputTokens != 3 || info.LastTotalTokens != 13 {
		t.Fatalf("session usage = %#v", info)
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
		{Role: model.RoleAssistant, Content: "old response", Usage: model.Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}},
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
	info, err := store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.LastTotalTokens != 0 {
		t.Fatalf("session usage = %#v", info)
	}
}

func TestStoreSaveCompactionPreservesFullTranscript(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	messages := []model.Message{
		{Role: model.RoleUser, Content: "old"},
		{Role: model.RoleAssistant, Content: "old response"},
		{Role: model.RoleUser, Content: "recent"},
		{Role: model.RoleAssistant, Content: "recent response"},
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", messages); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveCompaction(ctx, "work", "summary", 2, 100); err != nil {
		t.Fatalf("SaveCompaction() error = %v", err)
	}

	info, err := store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.ContextSummary != "summary" || info.CompactedMessageCount != 2 || info.CompactedInputTokens != 100 {
		t.Fatalf("session compaction = %#v", info)
	}
	full, err := store.LoadTranscript(ctx, "work")
	if err != nil {
		t.Fatalf("LoadTranscript() error = %v", err)
	}
	if len(full.Messages()) != len(messages) {
		t.Fatalf("full messages = %#v", full.Messages())
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", append(messages, model.Message{Role: model.RoleUser, Content: "new"})); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	info, err = store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.ContextSummary != "summary" || info.CompactedMessageCount != 2 || info.CompactedInputTokens != 100 {
		t.Fatalf("session compaction after save = %#v", info)
	}
}

func TestStoreSaveMemoryExtractionPreservesBoundary(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	messages := []model.Message{
		{Role: model.RoleUser, Content: "remember tests"},
		{Role: model.RoleAssistant, Content: "ok"},
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", messages); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	if err := store.SaveMemoryExtraction(ctx, "work", len(messages), 42, "hash-2"); err != nil {
		t.Fatalf("SaveMemoryExtraction() error = %v", err)
	}
	info, err := store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.MemoryExtractedMessageCount != 2 || info.MemoryExtractedInputTokens != 42 || info.MemoryExtractedHash != "hash-2" || info.MemoryExtractedAt.IsZero() {
		t.Fatalf("memory extraction = %#v", info)
	}
	if err := store.SaveMemoryExtraction(ctx, "work", 1, 10, "hash-1"); err != nil {
		t.Fatalf("older SaveMemoryExtraction() error = %v", err)
	}
	info, err = store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.MemoryExtractedMessageCount != 2 || info.MemoryExtractedInputTokens != 42 || info.MemoryExtractedHash != "hash-2" {
		t.Fatalf("memory extraction after older save = %#v", info)
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", append(messages, model.Message{Role: model.RoleUser, Content: "new"})); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	info, err = store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if info.MemoryExtractedMessageCount != 2 || info.MemoryExtractedInputTokens != 42 || info.MemoryExtractedHash != "hash-2" {
		t.Fatalf("memory extraction after transcript save = %#v", info)
	}
}

func TestStoreSaveTranscriptPersistsAdditionalDirectories(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	roots := []string{"/tmp/extra", "/tmp/shared"}
	if err := store.SaveTranscriptWithOptions(ctx, "work", "/tmp/work", []model.Message{
		{Role: model.RoleUser, Content: "hello"},
	}, SaveTranscriptOptions{AdditionalDirectories: roots, AdditionalDirectoriesSet: true}); err != nil {
		t.Fatalf("SaveTranscriptWithOptions() error = %v", err)
	}
	info, err := store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if strings.Join(info.AdditionalDirectories, ",") != strings.Join(roots, ",") {
		t.Fatalf("additional directories = %#v", info.AdditionalDirectories)
	}
	if err := store.SaveTranscript(ctx, "work", "/tmp/work", []model.Message{
		{Role: model.RoleUser, Content: "again"},
	}); err != nil {
		t.Fatalf("SaveTranscript() error = %v", err)
	}
	info, err = store.GetSession(ctx, "work")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if strings.Join(info.AdditionalDirectories, ",") != strings.Join(roots, ",") {
		t.Fatalf("additional directories after save = %#v", info.AdditionalDirectories)
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
		{Role: model.RoleAssistant, Content: "second response", Usage: model.Usage{InputTokens: 20, OutputTokens: 4, TotalTokens: 24}},
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
	second, err := store.GetSession(ctx, "second")
	if err != nil {
		t.Fatalf("GetSession(second) error = %v", err)
	}
	if second.LastInputTokens != 20 || second.LastOutputTokens != 4 || second.LastTotalTokens != 24 {
		t.Fatalf("session usage = %#v", second)
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

func TestStoreListSessionsPageReturnsCursor(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	for _, id := range []string{"first", "second", "third"} {
		if err := store.SaveTranscript(ctx, id, "/tmp/work", []model.Message{{Role: model.RoleUser, Content: id}}); err != nil {
			t.Fatalf("SaveTranscript(%s) error = %v", id, err)
		}
		time.Sleep(time.Millisecond)
	}
	page, err := store.ListSessionsPage(ctx, "", 2)
	if err != nil {
		t.Fatalf("ListSessionsPage() error = %v", err)
	}
	if len(page.Sessions) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %#v", page)
	}
	next, err := store.ListSessionsPage(ctx, page.NextCursor, 2)
	if err != nil {
		t.Fatalf("ListSessionsPage(next) error = %v", err)
	}
	if len(next.Sessions) != 1 || next.NextCursor != "" {
		t.Fatalf("next page = %#v", next)
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
