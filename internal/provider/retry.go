// Package provider provides shared HTTP retry logic for model provider implementations.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Retry configuration. Package-level variables so tests can override backoff duration.
var (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
)

// DoWithRetry executes an HTTP request with exponential backoff retry on transient errors.
// sendFunc creates a fresh *http.Request for each attempt (the body is consumed per attempt).
// Returns the HTTP response for successful (2xx) and non-retryable responses; the caller
// must close resp.Body. Retries on network errors, 429, and 5xx with exponential backoff
// (1s, 2s, 4s), respecting the Retry-After header. Does not retry on other 4xx errors.
// When retries are exhausted on a retryable HTTP status, the last response is returned
// (with its body unread) so the caller can format a detailed error message.
func DoWithRetry(ctx context.Context, client *http.Client, sendFunc func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := sendFunc()
		if err != nil {
			lastErr = err
			if attempt == maxRetries {
				break
			}
			if err := ctxSleep(ctx, backoffFor(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == maxRetries {
				break
			}
			if err := ctxSleep(ctx, backoffFor(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		if !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Retryable status (429, 5xx)
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		if attempt == maxRetries {
			// Exhausted retries — return response so caller can read body
			return resp, nil
		}

		wait := backoffFor(attempt)
		if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
			wait = ra
		}
		resp.Body.Close()
		if err := ctxSleep(ctx, wait); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

// isRetryableStatus returns true for transient HTTP status codes that warrant a retry.
func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// backoffFor returns the exponential backoff duration for the given attempt index.
// attempt 0 → 1s, attempt 1 → 2s, attempt 2 → 4s.
func backoffFor(attempt int) time.Duration {
	return initialBackoff * time.Duration(1<<attempt)
}

// parseRetryAfter parses the Retry-After header value into a duration.
// Supports both delta-seconds and HTTP-date formats per RFC 7231.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// ctxSleep sleeps for the given duration, returning early if the context is cancelled.
func ctxSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
