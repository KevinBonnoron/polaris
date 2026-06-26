package polaris

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func resetCodexUsageCacheForTest(t *testing.T) {
	t.Helper()
	codexUsageCacheMu.Lock()
	oldCache := codexUsageCache
	oldCacheAt := codexUsageCacheAt
	codexUsageCache = nil
	codexUsageCacheAt = codexUsageCacheAt.Add(-codexUsageCacheTTL)
	codexUsageCacheMu.Unlock()
	t.Cleanup(func() {
		codexUsageCacheMu.Lock()
		defer codexUsageCacheMu.Unlock()
		codexUsageCache = oldCache
		codexUsageCacheAt = oldCacheAt
	})
}

func TestFetchCodexUsageLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetCodexUsageCacheForTest(t)

	dir := filepath.Join(home, ".codex", "sessions", "2026", "06", "24")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout.jsonl")
	lines := []string{
		mustJSON(map[string]any{"timestamp": "2026-06-24T07:00:00Z", "type": "event_msg", "payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{"last_token_usage": map[string]any{
				"input_tokens":            100,
				"cached_input_tokens":     25,
				"output_tokens":           10,
				"reasoning_output_tokens": 5,
			}},
			"rate_limits": map[string]any{"plan_type": "free", "primary": map[string]any{
				"used_percent":   12,
				"window_minutes": 43200,
				"resets_at":      1784875584,
			}},
		}}),
		mustJSON(map[string]any{"timestamp": "2026-06-24T07:10:00Z", "type": "event_msg", "payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{"last_token_usage": map[string]any{
				"input_tokens":            50,
				"cached_input_tokens":     10,
				"output_tokens":           3,
				"reasoning_output_tokens": 2,
			}},
			"rate_limits": map[string]any{"plan_type": "plus", "primary": map[string]any{
				"used_percent":   42,
				"window_minutes": 10080,
				"resets_at":      1785000000,
			}},
		}}),
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	usage, err := FetchCodexUsage(true)
	if err != nil {
		t.Fatal(err)
	}
	if usage.PercentUsed != 42 {
		t.Fatalf("PercentUsed = %d, want 42", usage.PercentUsed)
	}
	if usage.WindowMinutes != 10080 {
		t.Fatalf("WindowMinutes = %d, want 10080", usage.WindowMinutes)
	}
	if usage.PlanType != "plus" {
		t.Fatalf("PlanType = %q, want plus", usage.PlanType)
	}
	if usage.TotalTokens.Input != 150 || usage.TotalTokens.CacheRead != 35 || usage.TotalTokens.Output != 20 {
		t.Fatalf("TotalTokens = %+v, want input=150 cacheRead=35 output=20", usage.TotalTokens)
	}
}

func TestFetchCodexUsageLocalIgnoresLargeNonTokenLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetCodexUsageCacheForTest(t)

	dir := filepath.Join(home, ".codex", "sessions", "2026", "06", "24")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout.jsonl")
	large := `{"timestamp":"2026-06-24T07:00:00Z","type":"event_msg","payload":{"type":"agent_message","text":"` + string(bytes.Repeat([]byte("x"), 5*1024*1024)) + `"}}`
	token := mustJSON(map[string]any{"timestamp": "2026-06-24T07:10:00Z", "type": "event_msg", "payload": map[string]any{
		"type": "token_count",
		"info": map[string]any{"last_token_usage": map[string]any{
			"input_tokens":        10,
			"cached_input_tokens": 5,
			"output_tokens":       3,
		}},
	}})
	if err := os.WriteFile(path, []byte(large+"\n"+token+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	usage, err := FetchCodexUsage(true)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens.Input != 10 || usage.TotalTokens.CacheRead != 5 || usage.TotalTokens.Output != 3 {
		t.Fatalf("TotalTokens = %+v, want input=10 cacheRead=5 output=3", usage.TotalTokens)
	}
}

func TestFetchCodexUsageLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": "test-token",
			"account_id":   "test-account",
		},
	}
	rawAuth, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), rawAuth, 0o600); err != nil {
		t.Fatal(err)
	}

	oldHTTP := codexUsageHTTP
	defer func() { codexUsageHTTP = oldHTTP }()
	codexUsageHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get(codexChatGPTAccountHdr); got != "test-account" {
			t.Fatalf("%s = %q", codexChatGPTAccountHdr, got)
		}
		body := ""
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			body = `{
				"plan_type": "plus",
				"rate_limit": {
					"primary_window": {
						"used_percent": 42,
						"limit_window_seconds": 604800,
						"reset_at": 1785000000
					},
					"secondary_window": {
						"used_percent": 15,
						"limit_window_seconds": 604800,
						"reset_at": 1785604800
					}
				}
			}`
		case "/backend-api/wham/profiles/me":
			body = `{"stats":{"lifetime_tokens":12345}}`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
		}, nil
	})}
	t.Setenv("CODEX_CHATGPT_BASE_URL", "https://chatgpt.example/backend-api")

	usage, err := fetchCodexUsageLive()
	if err != nil {
		t.Fatal(err)
	}
	if usage.PercentUsed != 42 {
		t.Fatalf("PercentUsed = %d, want 42", usage.PercentUsed)
	}
	if usage.WindowMinutes != 10080 {
		t.Fatalf("WindowMinutes = %d, want 10080", usage.WindowMinutes)
	}
	if usage.PlanType != "plus" {
		t.Fatalf("PlanType = %q, want plus", usage.PlanType)
	}
	if usage.WeeklyPercentUsed != 15 {
		t.Fatalf("WeeklyPercentUsed = %d, want 15", usage.WeeklyPercentUsed)
	}
	if usage.WeeklyWindowMinutes != 10080 {
		t.Fatalf("WeeklyWindowMinutes = %d, want 10080", usage.WeeklyWindowMinutes)
	}
	if usage.LifetimeTokens != 12345 {
		t.Fatalf("LifetimeTokens = %d, want 12345", usage.LifetimeTokens)
	}
}
