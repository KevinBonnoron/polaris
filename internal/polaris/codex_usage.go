package polaris

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	codexUsageCacheTTL     = 2 * time.Minute
	codexUsageDefaultBase  = "https://chatgpt.com/backend-api"
	codexUsageUserAgent    = "codex-cli"
	codexUsageProductSku   = "codex"
	codexUsageHTTPTimeout  = 15 * time.Second
	codexChatGPTAccountHdr = "ChatGPT-Account-Id"
)

var (
	codexUsageCache   *CodexUsage
	codexUsageCacheAt time.Time
	codexUsageCacheMu sync.Mutex
	codexUsageHTTP    = &http.Client{Timeout: codexUsageHTTPTimeout}
)

// CodexUsage represents the usage statistics for Codex.
type CodexUsage struct {
	PercentUsed         int        `json:"percentUsed"`
	ResetAt             string     `json:"resetAt,omitempty"`
	WindowMinutes       int        `json:"windowMinutes,omitempty"`
	WeeklyPercentUsed   *int       `json:"weeklyPercentUsed"`
	WeeklyResetAt       *string    `json:"weeklyResetAt"`
	WeeklyWindowMinutes *int       `json:"weeklyWindowMinutes"`
	PlanType            string     `json:"planType,omitempty"`
	TotalTokens         TokenUsage `json:"totalTokens"`
	LifetimeTokens      int        `json:"lifetimeTokens,omitempty"`
	LastUpdated         string     `json:"lastUpdated"`
}

type codexAuthFile struct {
	AuthMode string `json:"auth_mode"`
	Tokens   struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

type codexRateLimitStatus struct {
	PlanType  string `json:"plan_type"`
	RateLimit *struct {
		PrimaryWindow   *codexRateLimitWindow `json:"primary_window"`
		SecondaryWindow *codexRateLimitWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	AdditionalRateLimits []struct {
		MeteredFeature string `json:"metered_feature"`
		LimitName      string `json:"limit_name"`
		RateLimit      *struct {
			PrimaryWindow *codexRateLimitWindow `json:"primary_window"`
		} `json:"rate_limit"`
	} `json:"additional_rate_limits"`
}

type codexRateLimitWindow struct {
	UsedPercent       float64 `json:"used_percent"`
	LimitWindowSecond int     `json:"limit_window_seconds"`
	ResetAt           int64   `json:"reset_at"`
}

type codexTokenUsageProfile struct {
	Stats struct {
		LifetimeTokens int `json:"lifetime_tokens"`
	} `json:"stats"`
}

type codexSessionEvent struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTokenCountPayload struct {
	Info struct {
		LastTokenUsage struct {
			InputTokens           int `json:"input_tokens"`
			CachedInputTokens     int `json:"cached_input_tokens"`
			OutputTokens          int `json:"output_tokens"`
			ReasoningOutputTokens int `json:"reasoning_output_tokens"`
		} `json:"last_token_usage"`
	} `json:"info"`
	RateLimits struct {
		Primary *struct {
			UsedPercent   float64 `json:"used_percent"`
			WindowMinutes int     `json:"window_minutes"`
			ResetsAt      int64   `json:"resets_at"`
		} `json:"primary"`
		PlanType string `json:"plan_type"`
	} `json:"rate_limits"`
}

// FetchCodexUsage retrieves the current usage statistics for Codex.
// If force is true, it bypasses the cache and fetches fresh data.
func FetchCodexUsage(force bool) (*CodexUsage, error) {
	codexUsageCacheMu.Lock()
	defer codexUsageCacheMu.Unlock()

	if !force && codexUsageCache != nil && time.Since(codexUsageCacheAt) < codexUsageCacheTTL {
		return codexUsageCache, nil
	}

	usage, err := fetchCodexUsageLive()
	if err != nil {
		usage, err = fetchCodexUsageLocal()
	}
	if err != nil {
		return nil, err
	}
	codexUsageCache = usage
	codexUsageCacheAt = time.Now()
	return usage, nil
}

func fetchCodexUsageLive() (*CodexUsage, error) {
	auth, err := loadCodexAuth()
	if err != nil {
		return nil, err
	}

	baseURL := codexChatGPTBaseURL()
	status, err := fetchCodexRateLimitStatus(baseURL, auth)
	if err != nil {
		return nil, err
	}

	out := &CodexUsage{LastUpdated: time.Now().UTC().Format(time.RFC3339), PlanType: status.PlanType}
	// Primary window (session window)
	primaryWindow := preferredCodexRateLimitWindow(status)
	if primaryWindow != nil {
		out.PercentUsed = roundPct(primaryWindow.UsedPercent)
		if primaryWindow.LimitWindowSecond > 0 {
			out.WindowMinutes = (primaryWindow.LimitWindowSecond + 59) / 60
		}
		if primaryWindow.ResetAt > 0 {
			out.ResetAt = time.Unix(primaryWindow.ResetAt, 0).UTC().Format(time.RFC3339)
		}
	}

	// Secondary window (weekly window)
	if status.RateLimit != nil && status.RateLimit.SecondaryWindow != nil {
		secondary := status.RateLimit.SecondaryWindow
		pct := roundPct(secondary.UsedPercent)
		out.WeeklyPercentUsed = &pct
		if secondary.LimitWindowSecond > 0 {
			minutes := (secondary.LimitWindowSecond + 59) / 60
			out.WeeklyWindowMinutes = &minutes
		}
		if secondary.ResetAt > 0 {
			reset := time.Unix(secondary.ResetAt, 0).UTC().Format(time.RFC3339)
			out.WeeklyResetAt = &reset
		}
	}

	if profile, err := fetchCodexTokenUsageProfile(baseURL, auth); err == nil {
		out.LifetimeTokens = profile.Stats.LifetimeTokens
	}
	if local, err := fetchCodexUsageLocal(); err == nil {
		out.TotalTokens = local.TotalTokens
	}
	return out, nil
}

func fetchCodexRateLimitStatus(baseURL string, auth codexAuthFile) (*codexRateLimitStatus, error) {
	var out codexRateLimitStatus
	if err := codexBackendGET(baseURL, "/api/codex/usage", "/wham/usage", auth, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func fetchCodexTokenUsageProfile(baseURL string, auth codexAuthFile) (*codexTokenUsageProfile, error) {
	var out codexTokenUsageProfile
	if err := codexBackendGET(baseURL, "/api/codex/profiles/me", "/wham/profiles/me", auth, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func codexBackendGET(baseURL, codexPath, chatGPTPath string, auth codexAuthFile, out any) error {
	url := strings.TrimRight(baseURL, "/") + codexPath
	if strings.Contains(baseURL, "/backend-api") {
		url = strings.TrimRight(baseURL, "/") + chatGPTPath
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", codexUsageUserAgent)
	req.Header.Set("OAI-Product-Sku", codexUsageProductSku)
	req.Header.Set("Authorization", "Bearer "+auth.Tokens.AccessToken)
	if auth.Tokens.AccountID != "" {
		req.Header.Set(codexChatGPTAccountHdr, auth.Tokens.AccountID)
	}

	resp, err := codexUsageHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call codex usage endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("codex token rejected (run `codex login` to refresh): %s", strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("codex usage endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode codex usage response: %w", err)
	}
	return nil
}

func preferredCodexRateLimitWindow(status *codexRateLimitStatus) *codexRateLimitWindow {
	if status == nil {
		return nil
	}
	if status.RateLimit != nil && status.RateLimit.PrimaryWindow != nil {
		return status.RateLimit.PrimaryWindow
	}
	for _, additional := range status.AdditionalRateLimits {
		if additional.MeteredFeature != "codex" && additional.LimitName != "codex" {
			continue
		}
		if additional.RateLimit != nil && additional.RateLimit.PrimaryWindow != nil {
			return additional.RateLimit.PrimaryWindow
		}
	}
	return nil
}

func loadCodexAuth() (codexAuthFile, error) {
	var auth codexAuthFile
	home, err := os.UserHomeDir()
	if err != nil {
		return auth, fmt.Errorf("locate home dir: %w", err)
	}
	path := filepath.Join(home, ".codex", "auth.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return auth, fmt.Errorf("read codex auth at %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		return auth, fmt.Errorf("parse codex auth: %w", err)
	}
	if auth.Tokens.AccessToken == "" {
		return auth, fmt.Errorf("no codex access token found in %s", path)
	}
	return auth, nil
}

func codexChatGPTBaseURL() string {
	if raw := strings.TrimSpace(os.Getenv("CODEX_CHATGPT_BASE_URL")); raw != "" {
		return normalizeCodexBaseURL(raw)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if raw, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml")); err == nil {
			if configured := parseCodexConfigString(string(raw), "chatgpt_base_url"); configured != "" {
				return normalizeCodexBaseURL(configured)
			}
		}
	}
	return codexUsageDefaultBase
}

func normalizeCodexBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if (strings.HasPrefix(base, "https://chatgpt.com") || strings.HasPrefix(base, "https://chat.openai.com")) && !strings.Contains(base, "/backend-api") {
		return base + "/backend-api"
	}
	return base
}

func parseCodexConfigString(raw, key string) string {
	prefix := key + " = "
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		value = strings.Trim(value, `"'`)
		return strings.TrimSpace(value)
	}
	return ""
}

func fetchCodexUsageLocal() (*CodexUsage, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home dir: %w", err)
	}
	root := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("codex sessions directory not found at %s", root)
		}
		return nil, fmt.Errorf("stat codex sessions directory: %w", err)
	}

	out := &CodexUsage{LastUpdated: time.Now().UTC().Format(time.RFC3339)}
	var latestRateLimit time.Time

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		return scanCodexUsageFile(path, out, &latestRateLimit)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scanCodexUsageFile(path string, out *CodexUsage, latestRateLimit *time.Time) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var evt codexSessionEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			if err == io.EOF {
				return nil
			}
			continue
		}
		if evt.Type != "event_msg" {
			if err == io.EOF {
				return nil
			}
			continue
		}
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(evt.Payload, &peek) != nil || peek.Type != "token_count" {
			if err == io.EOF {
				return nil
			}
			continue
		}
		var payload codexTokenCountPayload
		if json.Unmarshal(evt.Payload, &payload) != nil {
			if err == io.EOF {
				return nil
			}
			continue
		}
		out.TotalTokens = out.TotalTokens.Add(TokenUsage{
			Input:     payload.Info.LastTokenUsage.InputTokens,
			Output:    payload.Info.LastTokenUsage.OutputTokens + payload.Info.LastTokenUsage.ReasoningOutputTokens,
			CacheRead: payload.Info.LastTokenUsage.CachedInputTokens,
		})
		if payload.RateLimits.Primary == nil {
			continue
		}
		ts := parseCodexEventTime(evt.Timestamp)
		if ts.IsZero() || ts.Before(*latestRateLimit) {
			continue
		}
		*latestRateLimit = ts
		out.PercentUsed = roundPct(payload.RateLimits.Primary.UsedPercent)
		out.WindowMinutes = payload.RateLimits.Primary.WindowMinutes
		if payload.RateLimits.Primary.ResetsAt > 0 {
			out.ResetAt = time.Unix(payload.RateLimits.Primary.ResetsAt, 0).UTC().Format(time.RFC3339)
		}
		out.PlanType = payload.RateLimits.PlanType
		if err == io.EOF {
			return nil
		}
	}
}

func parseCodexEventTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}
