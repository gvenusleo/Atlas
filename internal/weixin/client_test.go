package weixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendsTypingWithTicket(t *testing.T) {
	var gotPath string
	var gotAuth string
	var got sendTypingRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.SendTyping(context.Background(), "user-1", "ticket-1", true); err != nil {
		t.Fatalf("SendTyping() error = %v", err)
	}

	if gotPath != "/ilink/bot/sendtyping" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if got.UserID != "user-1" || got.TypingTicket != "ticket-1" || got.Status != typingStatusTyping {
		t.Fatalf("request = %#v", got)
	}
	if got.BaseInfo.BotAgent == "" {
		t.Fatalf("base_info = %#v", got.BaseInfo)
	}
}

func TestNewClientRejectsUnsupportedScheme(t *testing.T) {
	if _, err := NewClient(ClientOptions{BaseURL: "ftp://ilinkai.weixin.qq.com"}); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func TestClientReturnsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ret":3,"errmsg":"bad token"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	err = client.SendText(context.Background(), "user-1", "hello", "", "")
	if err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("SendText() error = %v", err)
	}
}

func TestClientSendTextBuildsMessage(t *testing.T) {
	var got sendMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.SendText(context.Background(), "user-1", "hello", "ctx", "run"); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}

	if got.Message.ToUserID != "user-1" || got.Message.MessageType != messageTypeBot || got.Message.MessageState != messageStateFinish {
		t.Fatalf("message = %#v", got.Message)
	}
	if got.Message.ContextToken != "ctx" || got.Message.RunID != "run" {
		t.Fatalf("message = %#v", got.Message)
	}
	if len(got.Message.Items) != 1 || got.Message.Items[0].TextItem == nil || got.Message.Items[0].TextItem.Text != "hello" {
		t.Fatalf("items = %#v", got.Message.Items)
	}
	if !strings.HasPrefix(got.Message.ClientID, "atlas-") {
		t.Fatalf("client id = %q", got.Message.ClientID)
	}
}

func TestClientGetConfigReturnsTypingTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got getConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got.UserID != "user-1" || got.ContextToken != "ctx" {
			t.Fatalf("request = %#v", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"typing_ticket":"ticket-1"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ticket, err := client.GetConfig(context.Background(), "user-1", "ctx")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if ticket != "ticket-1" {
		t.Fatalf("ticket = %q", ticket)
	}
}
