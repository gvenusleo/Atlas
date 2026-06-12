package weixin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginSavesConfirmedAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			_, _ = w.Write([]byte(`{"qrcode":"qr-1","qrcode_img_content":"https://qr.example.com/1"}`))
		case "/ilink/bot/get_qrcode_status":
			_, _ = w.Write([]byte(`{"status":"confirmed","bot_token":"token-1","ilink_bot_id":"bot-1","ilink_user_id":"user-1"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store, err := NewStore(filepath.Join(t.TempDir(), "weixin"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	var output bytes.Buffer

	result, err := Login(context.Background(), LoginOptions{
		Store:  store,
		Client: client,
		Output: &output,
		Now:    func() time.Time { return time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Account.ID != "bot-1" || result.Account.UserID != "user-1" || result.Account.Token != "token-1" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(output.String(), "https://qr.example.com/1") {
		t.Fatalf("output = %q", output.String())
	}
	account, err := store.LoadAccount("bot-1")
	if err != nil {
		t.Fatalf("LoadAccount() error = %v", err)
	}
	if account.ID != "bot-1" || account.UserID != "user-1" {
		t.Fatalf("account = %#v", account)
	}
}
