package weixin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStoreSavesCurrentAccount(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "weixin"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	account := Account{
		ID:        "bot/1",
		Token:     "token-1",
		BaseURL:   "https://weixin.example.com",
		UserID:    "user-1",
		UpdatedAt: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	}
	if err := store.SaveAccount(account); err != nil {
		t.Fatalf("SaveAccount() error = %v", err)
	}

	got, err := store.LoadAccount("")
	if err != nil {
		t.Fatalf("LoadAccount() error = %v", err)
	}
	if got.ID != account.ID || got.UserID != account.UserID || got.Token != account.Token {
		t.Fatalf("account = %#v", got)
	}
	info, err := os.Stat(filepath.Join(store.dir, accountsDirName, "bot_1.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
