package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTavilySearchCallsAPI(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{
			"results": [
				{
					"title": "Atlas",
					"url": "https://example.com/atlas",
					"content": "Atlas result",
					"score": 0.9,
					"published_date": "2026-06-01"
				}
			],
			"usage": {"credits": 1}
		}`)
	}))
	defer server.Close()
	client := newTestTavilyClient(t, server.URL)
	result, err := (TavilySearch{Client: client}).Run(context.Background(), `{
		"query": "atlas agent",
		"max_results": 3,
		"search_depth": "fast",
		"topic": "news",
		"time_range": "week",
		"include_domains": ["example.com"],
		"exclude_domains": ["old.example.com"]
	}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotAuth != "Bearer tvly-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["query"] != "atlas agent" || gotBody["search_depth"] != "fast" || gotBody["topic"] != "news" || gotBody["time_range"] != "week" {
		t.Fatalf("body = %#v", gotBody)
	}
	if gotBody["max_results"].(float64) != 3 {
		t.Fatalf("max_results = %#v", gotBody["max_results"])
	}
	if gotBody["include_answer"] != false || gotBody["include_raw_content"] != false || gotBody["include_images"] != false || gotBody["include_usage"] != true {
		t.Fatalf("body = %#v", gotBody)
	}
	for _, want := range []string{
		"1. Atlas",
		"URL: https://example.com/atlas",
		"Published: 2026-06-01",
		"Score: 0.9",
		"Content: Atlas result",
		"Usage: 1 Tavily credits",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("result missing %q: %s", want, result)
		}
	}
}

func TestTavilySearchUsesDefaults(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"results": []}`)
	}))
	defer server.Close()
	client := newTestTavilyClient(t, server.URL)
	result, err := (TavilySearch{Client: client}).Run(context.Background(), `{"query":"atlas"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotBody["search_depth"] != "basic" || gotBody["topic"] != "general" {
		t.Fatalf("body = %#v", gotBody)
	}
	if gotBody["max_results"].(float64) != 5 {
		t.Fatalf("max_results = %#v", gotBody["max_results"])
	}
	if !strings.Contains(result, "No results found.") {
		t.Fatalf("result = %q", result)
	}
}

func TestTavilyFetchCallsAPI(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{
			"results": [
				{"url": "https://example.com/page", "raw_content": "# Page\n\nContent"}
			],
			"failed_results": [
				{"url": "https://example.com/missing", "error": "not found"}
			],
			"usage": {"credits": 1}
		}`)
	}))
	defer server.Close()
	client := newTestTavilyClient(t, server.URL)
	result, err := (TavilyFetch{Client: client}).Run(context.Background(), `{
		"url": "https://example.com/page",
		"format": "text",
		"extract_depth": "advanced",
		"timeout_seconds": 12.5
	}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotBody["urls"] != "https://example.com/page" || gotBody["format"] != "text" || gotBody["extract_depth"] != "advanced" {
		t.Fatalf("body = %#v", gotBody)
	}
	if gotBody["timeout"].(float64) != 12.5 || gotBody["include_usage"] != true {
		t.Fatalf("body = %#v", gotBody)
	}
	for _, want := range []string{
		"URL: https://example.com/page",
		"# Page",
		"Content",
		"Failed results:",
		"- https://example.com/missing: not found",
		"Usage: 1 Tavily credits",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("result missing %q: %s", want, result)
		}
	}
}

func TestTavilyFetchUsesDefaults(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"results": []}`)
	}))
	defer server.Close()
	client := newTestTavilyClient(t, server.URL)
	result, err := (TavilyFetch{Client: client}).Run(context.Background(), `{"url":"https://example.com/page"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotBody["format"] != "markdown" || gotBody["extract_depth"] != "basic" {
		t.Fatalf("body = %#v", gotBody)
	}
	if _, ok := gotBody["timeout"]; ok {
		t.Fatalf("unexpected timeout: %#v", gotBody)
	}
	if !strings.Contains(result, "No content extracted.") {
		t.Fatalf("result = %q", result)
	}
}

func TestTavilyHTTPErrorIncludesStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()
	client := newTestTavilyClient(t, server.URL)
	_, err := (TavilySearch{Client: client}).Run(context.Background(), `{"query":"atlas"}`)
	if err == nil || !strings.Contains(err.Error(), "status 401") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestTavilySearchRejectsInvalidArguments(t *testing.T) {
	tool := TavilySearch{Client: newTestTavilyClient(t, "https://example.com")}
	tests := []string{
		`{`,
		`{"query":""}`,
		`{"query":"atlas","max_results":21}`,
		`{"query":"atlas","search_depth":"deep"}`,
		`{"query":"atlas","topic":"sports"}`,
		`{"query":"atlas","time_range":"today"}`,
		`{"query":"atlas","include_domains":[""]}`,
	}
	for _, arguments := range tests {
		t.Run(arguments, func(t *testing.T) {
			if _, err := tool.Run(context.Background(), arguments); err == nil {
				t.Fatal("Run() error = nil")
			}
		})
	}
}

func TestTavilyFetchRejectsInvalidArguments(t *testing.T) {
	tool := TavilyFetch{Client: newTestTavilyClient(t, "https://example.com")}
	tests := []string{
		`{`,
		`{"url":""}`,
		`{"url":"file:///tmp/page"}`,
		`{"url":"https://example.com","format":"html"}`,
		`{"url":"https://example.com","extract_depth":"full"}`,
		`{"url":"https://example.com","timeout_seconds":61}`,
	}
	for _, arguments := range tests {
		t.Run(arguments, func(t *testing.T) {
			if _, err := tool.Run(context.Background(), arguments); err == nil {
				t.Fatal("Run() error = nil")
			}
		})
	}
}

func TestNewTavilyClientValidatesConfig(t *testing.T) {
	if _, err := NewTavilyClient(":", "tvly-test", nil); err == nil {
		t.Fatal("NewTavilyClient() error = nil")
	}
	if _, err := NewTavilyClient("ftp://example.com", "tvly-test", nil); err == nil {
		t.Fatal("NewTavilyClient() error = nil")
	}
	if _, err := NewTavilyClient("https://example.com", "", nil); err == nil {
		t.Fatal("NewTavilyClient() error = nil")
	}
}

func newTestTavilyClient(t *testing.T, baseURL string) *TavilyClient {
	t.Helper()

	client, err := NewTavilyClient(baseURL, "tvly-test", nil)
	if err != nil {
		t.Fatalf("NewTavilyClient() error = %v", err)
	}
	return client
}
