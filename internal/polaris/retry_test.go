package polaris

import (
	"testing"
	"time"
)

func TestIsRetryableAPIError(t *testing.T) {
	retryable := []string{
		`API Error: 529 {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
		`[result] error_during_execution · overloaded_error`,
		"api error: 503 service unavailable",
		"Error: 429 Too Many Requests",
		"hit the rate-limit, backing off",
	}
	for _, line := range retryable {
		if !isRetryableAPIError(line) {
			t.Errorf("expected retryable: %q", line)
		}
	}

	notRetryable := []string{
		"[result] success · 529 tokens · $0.0042",
		"compiled 529 files",
		"API Error: 400 invalid_request_error",
		"file not found",
	}
	for _, line := range notRetryable {
		if isRetryableAPIError(line) {
			t.Errorf("expected non-retryable: %q", line)
		}
	}
}

func TestFallbackClaudeModel(t *testing.T) {
	cases := map[string]string{
		"opus":             "sonnet",
		"claude-opus-4-6":  "sonnet",
		"sonnet":           "haiku",
		"claude-sonnet-46": "haiku",
		"haiku":            "",
		"auto":             "",
		"":                 "",
	}
	for in, want := range cases {
		if got := fallbackClaudeModel(in); got != want {
			t.Errorf("fallbackClaudeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRetryBackoff(t *testing.T) {
	cases := map[int]time.Duration{
		1: 2 * time.Second,
		2: 4 * time.Second,
		3: 8 * time.Second,
		4: 16 * time.Second,
		5: 30 * time.Second,
		9: 30 * time.Second,
	}
	for attempt, want := range cases {
		if got := retryBackoff(attempt); got != want {
			t.Errorf("retryBackoff(%d) = %s, want %s", attempt, got, want)
		}
	}
}
