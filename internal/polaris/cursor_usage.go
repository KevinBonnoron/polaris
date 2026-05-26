package polaris

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const cursorUsageCacheTTL = 5 * time.Minute

var (
	cursorUsageCache   *CursorUsage
	cursorUsageCacheAt time.Time
	cursorUsageCacheMu sync.Mutex
)

type CursorUsage struct {
	NumRequests      int    `json:"numRequests"`
	NumRequestsTotal int    `json:"numRequestsTotal"`
	NumTokens        int    `json:"numTokens"`
	NumCents         int    `json:"numCents"`
	StartOfMonth     string `json:"startOfMonth,omitempty"`
	MembershipType   string `json:"membershipType,omitempty"`
	LastUpdated      string `json:"lastUpdated"`
	Error            string `json:"error,omitempty"`
}

type cursorUsageResp struct {
	NumRequests      int    `json:"numRequests"`
	NumRequestsTotal int    `json:"numRequestsTotal"`
	NumTokens        int    `json:"numTokens"`
	NumCents         int    `json:"numCents"`
	StartOfMonth     string `json:"startOfMonth"`
}

type cursorStripeResp struct {
	MembershipType string `json:"membershipType"`
}

func FetchCursorUsage(force bool) (*CursorUsage, error) {
	cursorUsageCacheMu.Lock()
	defer cursorUsageCacheMu.Unlock()

	if !force && cursorUsageCache != nil && time.Since(cursorUsageCacheAt) < cursorUsageCacheTTL {
		return cursorUsageCache, nil
	}

	usage, err := fetchCursorUsageLive()
	if err != nil {
		return nil, err
	}
	cursorUsageCache = usage
	cursorUsageCacheAt = time.Now()
	return usage, nil
}

func fetchCursorUsageLive() (*CursorUsage, error) {
	token, err := loadCursorToken()
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	userID := cursorUserID(token)
	usageURL := "https://www.cursor.com/api/usage"
	if userID != "" {
		usageURL += "?user=" + userID
	}

	usageReq, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	setCursorHeaders(usageReq, token)

	usageResp, err := client.Do(usageReq)
	if err != nil {
		return nil, fmt.Errorf("call cursor usage: %w", err)
	}
	defer usageResp.Body.Close()

	if usageResp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("cursor token rejected (open Cursor to re-authenticate)")
	}
	if usageResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(usageResp.Body, 512))
		return nil, fmt.Errorf("cursor usage endpoint returned %d: %s", usageResp.StatusCode, strings.TrimSpace(string(body)))
	}

	var usageData cursorUsageResp
	if err := json.NewDecoder(usageResp.Body).Decode(&usageData); err != nil {
		return nil, fmt.Errorf("decode cursor usage: %w", err)
	}

	out := &CursorUsage{
		NumRequests:      usageData.NumRequests,
		NumRequestsTotal: usageData.NumRequestsTotal,
		NumTokens:        usageData.NumTokens,
		NumCents:         usageData.NumCents,
		StartOfMonth:     usageData.StartOfMonth,
		LastUpdated:      time.Now().UTC().Format(time.RFC3339),
	}

	// Best-effort: fetch membership type from stripe endpoint.
	stripeReq, err := http.NewRequest(http.MethodGet, "https://www.cursor.com/api/auth/stripe", nil)
	if err == nil {
		setCursorHeaders(stripeReq, token)
		if stripeResp, err := client.Do(stripeReq); err == nil {
			defer stripeResp.Body.Close()
			var stripeData cursorStripeResp
			if stripeResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(stripeResp.Body).Decode(&stripeData)
				out.MembershipType = stripeData.MembershipType
			}
		}
	}

	return out, nil
}

func setCursorHeaders(req *http.Request, token string) {
	req.Header.Set("Cookie", "WorkosCursorSessionToken="+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Origin", "https://cursor.com")
	req.Header.Set("Referer", "https://cursor.com/settings")
}

// cursorUserID extracts the user ID from the session token.
// Token format: "user_XXXXX::JWT..." — the user ID is the prefix before "::".
func cursorUserID(token string) string {
	if idx := strings.Index(token, "::"); idx > 0 {
		return token[:idx]
	}
	return ""
}

func loadCursorToken() (string, error) {
	if tok := strings.TrimSpace(os.Getenv("CURSOR_SESSION_TOKEN")); tok != "" {
		return tok, nil
	}

	dbPath, err := cursorStateDBPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("cursor state db not found at %s (is Cursor installed?)", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return "", fmt.Errorf("open cursor state db: %w", err)
	}
	defer db.Close()

	for _, key := range []string{
		"cursorAuth/accessToken",
		"cursor.accessToken",
		"workos.accessToken",
	} {
		var val string
		if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", key).Scan(&val); err == nil && val != "" {
			// Value may be JSON-encoded (e.g. "\"token...\"").
			var str string
			if json.Unmarshal([]byte(val), &str) == nil {
				return str, nil
			}
			return val, nil
		}
	}

	return "", fmt.Errorf("cursor auth token not found in state db (set CURSOR_SESSION_TOKEN env var as a fallback)")
}

func cursorStateDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}

	var base string
	switch runtime.GOOS {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		base = filepath.Join(appData, "Cursor", "User", "globalStorage")
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		base = filepath.Join(configHome, "Cursor", "User", "globalStorage")
	}

	return filepath.Join(base, "state.vscdb"), nil
}
