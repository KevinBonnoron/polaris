package polaris

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	usageCacheTTL  = 5 * time.Minute
	usageEndpoint  = "https://api.anthropic.com/api/oauth/usage"
	usageUserAgent = "claude-code/2.0.32"
	usageBetaHdr   = "oauth-2025-04-20"
)

var (
	usageCache   *ClaudeUsage
	usageCacheAt time.Time
	usageCacheMu sync.Mutex
)

type ClaudeUsage struct {
	SessionPercentUsed int                   `json:"sessionPercentUsed"`
	SessionResetAt     string                `json:"sessionResetAt,omitempty"`
	WeeklyPercentUsed  int                   `json:"weeklyPercentUsed"`
	WeeklyResetAt      string                `json:"weeklyResetAt,omitempty"`
	WeeklyByModel      map[string]ModelUsage `json:"weeklyByModel,omitempty"`
	LastUpdated        string                `json:"lastUpdated"`
	Error              string                `json:"error,omitempty"`
}

// ModelUsage is a per-model weekly limit bucket (e.g. Opus has its own cap on
// top of the all-models weekly window).
type ModelUsage struct {
	PercentUsed int    `json:"percentUsed"`
	ResetAt     string `json:"resetAt,omitempty"`
}

type usageBucket struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type usageResponse struct {
	FiveHour       *usageBucket `json:"five_hour"`
	SevenDay       *usageBucket `json:"seven_day"`
	SevenDayOpus   *usageBucket `json:"seven_day_opus"`
	SevenDaySonnet *usageBucket `json:"seven_day_sonnet"`
}

type oauthCredentials struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

func FetchClaudeUsage(force bool) (*ClaudeUsage, error) {
	usageCacheMu.Lock()
	defer usageCacheMu.Unlock()

	if !force && usageCache != nil && time.Since(usageCacheAt) < usageCacheTTL {
		return usageCache, nil
	}

	usage, err := fetchClaudeUsageLive()
	if err != nil {
		return nil, err
	}
	usageCache = usage
	usageCacheAt = time.Now()
	return usage, nil
}

func fetchClaudeUsageLive() (*ClaudeUsage, error) {
	token, err := loadClaudeToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, usageEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", usageUserAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", usageBetaHdr)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call usage endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("claude token rejected (run `claude` to refresh): %s", strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("usage endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode usage response: %w", err)
	}

	out := &ClaudeUsage{LastUpdated: time.Now().UTC().Format(time.RFC3339)}
	if data.FiveHour != nil {
		out.SessionPercentUsed = roundPct(data.FiveHour.Utilization)
		out.SessionResetAt = normalizeResetAt(data.FiveHour.ResetsAt)
	}
	if data.SevenDay != nil {
		out.WeeklyPercentUsed = roundPct(data.SevenDay.Utilization)
		out.WeeklyResetAt = normalizeResetAt(data.SevenDay.ResetsAt)
	}
	// Opus and Sonnet carry their own weekly cap on top of the all-models
	// window. The keys are always present in the response (null when unused),
	// so we surface them at 0% rather than hiding the bar. Haiku has no
	// dedicated cap, so it is intentionally absent here.
	out.WeeklyByModel = map[string]ModelUsage{
		"opus":   bucketUsage(data.SevenDayOpus),
		"sonnet": bucketUsage(data.SevenDaySonnet),
	}
	return out, nil
}

func bucketUsage(b *usageBucket) ModelUsage {
	if b == nil {
		return ModelUsage{}
	}
	return ModelUsage{PercentUsed: roundPct(b.Utilization), ResetAt: normalizeResetAt(b.ResetsAt)}
}

func roundPct(v float64) int {
	if v < 0 {
		return 0
	}

	if v > 100 {
		return 100
	}

	return int(v + 0.5)
}

func normalizeResetAt(s string) string {
	if s == "" {
		return ""
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return ""
		}
	}

	return t.UTC().Format(time.RFC3339)
}

func loadClaudeToken() (string, error) {
	if tok := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")); tok != "" {
		return tok, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}

	path := filepath.Join(home, ".claude", ".credentials.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read claude credentials at %s: %w", path, err)
	}

	var creds oauthCredentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return "", fmt.Errorf("parse claude credentials: %w", err)
	}

	if creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no access token found in %s", path)
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}
