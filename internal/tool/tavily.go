package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/liuyuxin/atlas/internal/model"
)

const (
	defaultTavilySearchDepth = "basic"
	defaultTavilyMaxResults  = 5
	maxTavilyMaxResults      = 20

	defaultTavilyExtractDepth = "basic"
	defaultTavilyFetchFormat  = "markdown"
	maxTavilyToolOutputBytes  = 256 * 1024
)

// TavilyClient 调用 Tavily REST API。
type TavilyClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewTavilyClient 创建 Tavily REST client。
func NewTavilyClient(baseURL, apiKey string, httpClient *http.Client) (*TavilyClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("tavily base_url is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("tavily base_url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("tavily base_url must use http or https")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("tavily api_key is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &TavilyClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}, nil
}

// TavilySearch 是基于 Tavily Search 的网页搜索工具。
type TavilySearch struct {
	Client *TavilyClient
}

// Definition 返回 web_search 的模型可见定义。
func (TavilySearch) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "web_search",
		Description: "Search the public web with Tavily and return ranked results with URLs and snippets.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query.",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return. Defaults to 5; maximum is 20.",
					"minimum":     0,
					"maximum":     maxTavilyMaxResults,
				},
				"search_depth": map[string]any{
					"type":        "string",
					"description": "Latency and relevance tradeoff. advanced costs more Tavily credits.",
					"enum":        []string{"basic", "advanced", "fast", "ultra-fast"},
				},
				"topic": map[string]any{
					"type":        "string",
					"description": "Search category.",
					"enum":        []string{"general", "news", "finance"},
				},
				"time_range": map[string]any{
					"type":        "string",
					"description": "Optional recency filter.",
					"enum":        []string{"day", "week", "month", "year", "d", "w", "m", "y"},
				},
				"include_domains": map[string]any{
					"type":        "array",
					"description": "Optional domains to include.",
					"items":       map[string]any{"type": "string"},
				},
				"exclude_domains": map[string]any{
					"type":        "array",
					"description": "Optional domains to exclude.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"query"},
		},
	}
}

// Run 使用 JSON 参数中的 query 调用 Tavily Search。
func (t TavilySearch) Run(ctx context.Context, arguments string) (string, error) {
	if t.Client == nil {
		return "", fmt.Errorf("web_search is not configured")
	}
	var args struct {
		Query          string   `json:"query"`
		MaxResults     *int     `json:"max_results"`
		SearchDepth    string   `json:"search_depth"`
		Topic          string   `json:"topic"`
		TimeRange      string   `json:"time_range"`
		IncludeDomains []string `json:"include_domains"`
		ExcludeDomains []string `json:"exclude_domains"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid web_search arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("web_search query is required")
	}
	maxResults := defaultTavilyMaxResults
	if args.MaxResults != nil {
		maxResults = *args.MaxResults
	}
	if maxResults < 0 || maxResults > maxTavilyMaxResults {
		return "", fmt.Errorf("web_search max_results must be between 0 and %d", maxTavilyMaxResults)
	}
	searchDepth, err := normalizeTavilySearchDepth(args.SearchDepth)
	if err != nil {
		return "", err
	}
	topic, err := normalizeTavilyTopic(args.Topic)
	if err != nil {
		return "", err
	}
	timeRange, err := normalizeTavilyTimeRange(args.TimeRange)
	if err != nil {
		return "", err
	}
	includeDomains, err := normalizeTavilyStringList(args.IncludeDomains, "include_domains", 300)
	if err != nil {
		return "", err
	}
	excludeDomains, err := normalizeTavilyStringList(args.ExcludeDomains, "exclude_domains", 150)
	if err != nil {
		return "", err
	}

	resp, err := t.Client.search(ctx, tavilySearchRequest{
		Query:             query,
		SearchDepth:       searchDepth,
		MaxResults:        maxResults,
		Topic:             topic,
		TimeRange:         timeRange,
		IncludeDomains:    includeDomains,
		ExcludeDomains:    excludeDomains,
		IncludeAnswer:     false,
		IncludeRawContent: false,
		IncludeImages:     false,
		IncludeUsage:      true,
	})
	if err != nil {
		return "", err
	}
	return limitTavilyOutput(formatTavilySearchResponse(resp)), nil
}

// TavilyFetch 是基于 Tavily Extract 的网页内容提取工具。
type TavilyFetch struct {
	Client *TavilyClient
}

// Definition 返回 web_fetch 的模型可见定义。
func (TavilyFetch) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "web_fetch",
		Description: "Extract readable markdown or text from a single public web URL with Tavily.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTP or HTTPS URL to fetch.",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Extracted content format. Defaults to markdown.",
					"enum":        []string{"markdown", "text"},
				},
				"extract_depth": map[string]any{
					"type":        "string",
					"description": "Extraction depth. advanced may retrieve more content and costs more Tavily credits.",
					"enum":        []string{"basic", "advanced"},
				},
				"timeout_seconds": map[string]any{
					"type":        "number",
					"description": "Optional Tavily extraction timeout from 1 to 60 seconds.",
					"minimum":     1,
					"maximum":     60,
				},
			},
			"required": []string{"url"},
		},
	}
}

// Run 使用 JSON 参数中的 url 调用 Tavily Extract。
func (t TavilyFetch) Run(ctx context.Context, arguments string) (string, error) {
	if t.Client == nil {
		return "", fmt.Errorf("web_fetch is not configured")
	}
	var args struct {
		URL            string   `json:"url"`
		Format         string   `json:"format"`
		ExtractDepth   string   `json:"extract_depth"`
		TimeoutSeconds *float64 `json:"timeout_seconds"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid web_fetch arguments: %w", err)
	}
	targetURL := strings.TrimSpace(args.URL)
	if err := validateTavilyFetchURL(targetURL); err != nil {
		return "", err
	}
	format, err := normalizeTavilyFetchFormat(args.Format)
	if err != nil {
		return "", err
	}
	extractDepth, err := normalizeTavilyExtractDepth(args.ExtractDepth)
	if err != nil {
		return "", err
	}
	if args.TimeoutSeconds != nil && (*args.TimeoutSeconds < 1 || *args.TimeoutSeconds > 60) {
		return "", fmt.Errorf("web_fetch timeout_seconds must be between 1 and 60")
	}

	resp, err := t.Client.extract(ctx, tavilyExtractRequest{
		URLs:         targetURL,
		Format:       format,
		ExtractDepth: extractDepth,
		Timeout:      args.TimeoutSeconds,
		IncludeUsage: true,
	})
	if err != nil {
		return "", err
	}
	return limitTavilyOutput(formatTavilyExtractResponse(resp)), nil
}

type tavilySearchRequest struct {
	Query             string   `json:"query"`
	SearchDepth       string   `json:"search_depth"`
	MaxResults        int      `json:"max_results"`
	Topic             string   `json:"topic"`
	TimeRange         string   `json:"time_range,omitempty"`
	IncludeDomains    []string `json:"include_domains,omitempty"`
	ExcludeDomains    []string `json:"exclude_domains,omitempty"`
	IncludeAnswer     bool     `json:"include_answer"`
	IncludeRawContent bool     `json:"include_raw_content"`
	IncludeImages     bool     `json:"include_images"`
	IncludeUsage      bool     `json:"include_usage"`
}

type tavilySearchResponse struct {
	Query   string               `json:"query"`
	Answer  string               `json:"answer"`
	Results []tavilySearchResult `json:"results"`
	Usage   tavilyUsage          `json:"usage"`
}

type tavilySearchResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Content       string   `json:"content"`
	Score         *float64 `json:"score"`
	PublishedDate string   `json:"published_date"`
}

type tavilyExtractRequest struct {
	URLs         string   `json:"urls"`
	Format       string   `json:"format"`
	ExtractDepth string   `json:"extract_depth"`
	Timeout      *float64 `json:"timeout,omitempty"`
	IncludeUsage bool     `json:"include_usage"`
}

type tavilyExtractResponse struct {
	Results       []tavilyExtractResult `json:"results"`
	FailedResults []tavilyFailedResult  `json:"failed_results"`
	Usage         tavilyUsage           `json:"usage"`
}

type tavilyExtractResult struct {
	URL        string `json:"url"`
	RawContent string `json:"raw_content"`
}

type tavilyFailedResult struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

type tavilyUsage struct {
	Credits float64 `json:"credits"`
}

func (c *TavilyClient) search(ctx context.Context, req tavilySearchRequest) (tavilySearchResponse, error) {
	var resp tavilySearchResponse
	if err := c.post(ctx, "/search", "search", req, &resp); err != nil {
		return tavilySearchResponse{}, err
	}
	return resp, nil
}

func (c *TavilyClient) extract(ctx context.Context, req tavilyExtractRequest) (tavilyExtractResponse, error) {
	var resp tavilyExtractResponse
	if err := c.post(ctx, "/extract", "extract", req, &resp); err != nil {
		return tavilyExtractResponse{}, err
	}
	return resp, nil
}

func (c *TavilyClient) post(ctx context.Context, endpoint, operation string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode tavily %s request: %w", operation, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tavily %s request failed: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("tavily %s failed: status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(content)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode tavily %s response: %w", operation, err)
	}
	return nil
}

func normalizeTavilySearchDepth(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return defaultTavilySearchDepth, nil
	case "basic", "advanced", "fast", "ultra-fast":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("web_search search_depth must be one of basic, advanced, fast, ultra-fast")
	}
}

func normalizeTavilyTopic(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "general", nil
	case "general", "news", "finance":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("web_search topic must be one of general, news, finance")
	}
}

func normalizeTavilyTimeRange(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", nil
	case "day", "week", "month", "year", "d", "w", "m", "y":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("web_search time_range must be one of day, week, month, year, d, w, m, y")
	}
}

func normalizeTavilyExtractDepth(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return defaultTavilyExtractDepth, nil
	case "basic", "advanced":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("web_fetch extract_depth must be one of basic, advanced")
	}
}

func normalizeTavilyFetchFormat(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return defaultTavilyFetchFormat, nil
	case "markdown", "text":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("web_fetch format must be one of markdown, text")
	}
}

func normalizeTavilyStringList(values []string, name string, max int) ([]string, error) {
	if len(values) > max {
		return nil, fmt.Errorf("web_search %s must contain at most %d values", name, max)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("web_search %s must not contain empty values", name)
		}
		result = append(result, value)
	}
	return result, nil
}

func validateTavilyFetchURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("web_fetch url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("web_fetch url must be an absolute HTTP or HTTPS URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("web_fetch url must use http or https")
	}
	return nil
}

func formatTavilySearchResponse(resp tavilySearchResponse) string {
	var b strings.Builder
	if strings.TrimSpace(resp.Answer) != "" {
		b.WriteString("Answer:\n")
		b.WriteString(strings.TrimSpace(resp.Answer))
		b.WriteString("\n\n")
	}
	if len(resp.Results) == 0 {
		b.WriteString("No results found.")
		appendTavilyUsage(&b, resp.Usage)
		return b.String()
	}
	for i, result := range resp.Results {
		title := strings.TrimSpace(result.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, title)
		if strings.TrimSpace(result.URL) != "" {
			fmt.Fprintf(&b, "URL: %s\n", strings.TrimSpace(result.URL))
		}
		if strings.TrimSpace(result.PublishedDate) != "" {
			fmt.Fprintf(&b, "Published: %s\n", strings.TrimSpace(result.PublishedDate))
		}
		if result.Score != nil {
			fmt.Fprintf(&b, "Score: %s\n", strconv.FormatFloat(*result.Score, 'f', -1, 64))
		}
		if strings.TrimSpace(result.Content) != "" {
			fmt.Fprintf(&b, "Content: %s\n", strings.TrimSpace(result.Content))
		}
		if i != len(resp.Results)-1 {
			b.WriteString("\n")
		}
	}
	appendTavilyUsage(&b, resp.Usage)
	return b.String()
}

func formatTavilyExtractResponse(resp tavilyExtractResponse) string {
	var b strings.Builder
	if len(resp.Results) == 0 {
		b.WriteString("No content extracted.")
	}
	for i, result := range resp.Results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if strings.TrimSpace(result.URL) != "" {
			fmt.Fprintf(&b, "URL: %s\n\n", strings.TrimSpace(result.URL))
		}
		content := strings.TrimSpace(result.RawContent)
		if content == "" {
			b.WriteString("No content extracted.")
		} else {
			b.WriteString(content)
		}
	}
	if len(resp.FailedResults) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Failed results:\n")
		for _, failed := range resp.FailedResults {
			reason := strings.TrimSpace(failed.Error)
			if reason == "" {
				reason = "unknown error"
			}
			fmt.Fprintf(&b, "- %s: %s\n", strings.TrimSpace(failed.URL), reason)
		}
	}
	appendTavilyUsage(&b, resp.Usage)
	return strings.TrimSpace(b.String())
}

func appendTavilyUsage(b *strings.Builder, usage tavilyUsage) {
	if usage.Credits == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fmt.Fprintf(b, "Usage: %s Tavily credits", strconv.FormatFloat(usage.Credits, 'f', -1, 64))
}

func limitTavilyOutput(content string) string {
	if len(content) <= maxTavilyToolOutputBytes {
		return content
	}
	content = content[:maxTavilyToolOutputBytes]
	for !utf8.ValidString(content) && len(content) > 0 {
		content = content[:len(content)-1]
	}
	return content + "\n\n[Output truncated to 262144 bytes.]"
}
