package compact

import "strings"

// contextKeywords are keywords directly related to context/input.
// These words alone are insufficient to determine overflow; they require combination with limitKeywords.
var contextKeywords = []string{
	"context",
	"input",
	"messages",
	"items",
	"tokens",
}

// limitKeywords are words indicating quantity/length limits.
// These words alone are insufficient to determine overflow (e.g., "maximum of 5 attachments").
var limitKeywords = []string{
	"maximum of",
	"maximum length",
	"maximum number of",
	"too long",
	"too many",
}

// exactPatterns are overflow patterns that are sufficiently explicit on their own without combination matching.
var exactPatterns = []string{
	"context length",
	"context window",
	"context_limit",
}

// IsContextOverflow determines whether an error indicates context overflow (token or input item count).
// Used by the agent loop to decide whether to trigger auto-compaction recovery when the provider returns a 400.
//
// The matching strategy has two layers:
//  1. Exact mode: the error message contains self-explicit patterns like "context length", "context window", "context_limit".
//  2. Combination mode: the error message contains both a limitKeywords word and a contextKeywords word,
//     e.g., "Maximum of 1000 items allowed in input" matches both "maximum of" + "items"/"input".
//     This avoids false compaction triggers from unrelated 400s like "maximum of 5 attachments".
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Exact mode
	for _, pattern := range exactPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	// Combination mode: limitKeyword + contextKeyword
	hasLimit := false
	for _, kw := range limitKeywords {
		if strings.Contains(msg, kw) {
			hasLimit = true
			break
		}
	}
	if !hasLimit {
		return false
	}
	for _, kw := range contextKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
