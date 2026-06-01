package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStorePersistsSessionsAndMessages(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now()
	session := Session{
		ID:        "s1",
		Title:     "test",
		Workdir:   "/tmp/work",
		Model:     "deepseek-chat",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateSession(session); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(Message{
		SessionID: "s1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != session.ID || got.Model != session.Model {
		t.Fatalf("unexpected session: %+v", got)
	}

	messages, err := store.Messages("s1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}
