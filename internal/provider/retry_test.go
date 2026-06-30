package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// withShortBackoff overrides the package-level backoff for fast tests.
func withShortBackoff(t *testing.T) {
	t.Helper()
	origMax := maxRetries
	origBackoff := initialBackoff
	maxRetries = 3
	initialBackoff = 1 * time.Millisecond
	t.Cleanup(func() {
		maxRetries = origMax
		initialBackoff = origBackoff
	})
}

func TestDoWithRetrySuccessOnFirstAttempt(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestDoWithRetry429ThenSuccess(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestDoWithRetry500ThenSuccess(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestDoWithRetryNonRetryable4xx(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry for 4xx)", calls)
	}
}

func TestDoWithRetryExhaustedReturnsLastResponse(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	// Should return the last 503 response so caller can read the body
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"error":"unavailable"}` {
		t.Fatalf("body = %q, want {\"error\":\"unavailable\"}", string(body))
	}
	// maxRetries=3 means 4 total attempts
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
}

func TestDoWithRetryNetworkErrorThenSuccess(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// Force connection reset
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server doesn't support hijacking")
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestDoWithRetryAllNetworkErrors(t *testing.T) {
	withShortBackoff(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately so all connections fail

	_, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDoWithRetryContextCancelled(t *testing.T) {
	withShortBackoff(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt fails
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := DoWithRetry(ctx, server.Client(), func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDoWithRetryRetryAfterHeader(t *testing.T) {
	withShortBackoff(t)
	var calls int32
	var firstCallTime time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			firstCallTime = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		elapsed := time.Since(firstCallTime)
		if elapsed < 900*time.Millisecond {
			t.Errorf("retry happened too early: %v", elapsed)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := DoWithRetry(context.Background(), server.Client(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestDoWithRetrySendFuncError(t *testing.T) {
	withShortBackoff(t)
	_, err := DoWithRetry(context.Background(), http.DefaultClient, func() (*http.Request, error) {
		return nil, errors.New("sendfunc error")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		input string
		min   time.Duration
		max   time.Duration
	}{
		{"", 0, 0},
		{"0", 0, 0},
		{"-1", 0, 0},
		{"invalid", 0, 0},
		{"2", 2 * time.Second, 2 * time.Second},
		{"120", 120 * time.Second, 120 * time.Second},
	}
	for _, tt := range tests {
		got := parseRetryAfter(tt.input)
		if tt.min == 0 && got != 0 {
			t.Errorf("parseRetryAfter(%q) = %v, want 0", tt.input, got)
		}
		if tt.min > 0 && (got < tt.min || got > tt.max) {
			t.Errorf("parseRetryAfter(%q) = %v, want %v-%v", tt.input, got, tt.min, tt.max)
		}
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(2 * time.Second)
	got := parseRetryAfter(future.UTC().Format(http.TimeFormat))
	if got < 1*time.Second || got > 3*time.Second {
		t.Errorf("parseRetryAfter(httpDate) = %v, want ~2s", got)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tt := range tests {
		if got := isRetryableStatus(tt.status); got != tt.want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestBackoffFor(t *testing.T) {
	orig := initialBackoff
	initialBackoff = 1 * time.Second
	defer func() { initialBackoff = orig }()

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
	}
	for _, tt := range tests {
		if got := backoffFor(tt.attempt); got != tt.want {
			t.Errorf("backoffFor(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
