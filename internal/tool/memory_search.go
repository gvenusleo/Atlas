package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuyuxin/atlas/internal/model"
)

// MemorySearchFunc searches long-term memory entries by keyword.
type MemorySearchFunc func(ctx context.Context, query string, limit int) ([]MemoryEntry, error)

// MemoryEntry is a flat view of a memory entry returned to the model.
type MemoryEntry struct {
	Scope      string
	Type       string
	Content    string
	SourceNote string
}

const (
	defaultMemorySearchLimit = 20
	maxMemorySearchLimit     = 50
)

// MemorySearch exposes long-term memory retrieval as a model-invocable tool.
type MemorySearch struct {
	Search MemorySearchFunc
}

// Definition returns the model-visible definition for memory_search.
func (MemorySearch) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "memory_search",
		Description: "Search long-term memory entries from prior Atlas sessions by keyword. Use this when the task involves project history, user preferences, prior decisions, or repeatable workflows.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search keywords. Entries matching any keyword as a substring are returned.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results. Defaults to 20; maximum is 50.",
					"minimum":     1,
					"maximum":     maxMemorySearchLimit,
				},
			},
			"required": []string{"query"},
		},
	}
}

// Run executes the memory_search tool.
func (m MemorySearch) Run(ctx context.Context, arguments string) (string, error) {
	if m.Search == nil {
		return "", fmt.Errorf("memory_search is not configured")
	}
	var args struct {
		Query string `json:"query"`
		Limit *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid memory_search arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("memory_search query is required")
	}
	limit := defaultMemorySearchLimit
	if args.Limit != nil {
		limit = *args.Limit
	}
	if limit < 1 || limit > maxMemorySearchLimit {
		return "", fmt.Errorf("memory_search limit must be between 1 and %d", maxMemorySearchLimit)
	}
	entries, err := m.Search(ctx, query, limit)
	if err != nil {
		return "", fmt.Errorf("memory_search failed: %w", err)
	}
	return formatMemoryEntries(entries, query), nil
}

// formatMemoryEntries renders search results as readable text for the model.
func formatMemoryEntries(entries []MemoryEntry, query string) string {
	if len(entries) == 0 {
		return fmt.Sprintf("No memories found for query: %s", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d memories:\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "[%s/%s] %s\n", e.Scope, e.Type, e.Content)
		if e.SourceNote != "" {
			fmt.Fprintf(&b, "  source: %s\n", e.SourceNote)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
