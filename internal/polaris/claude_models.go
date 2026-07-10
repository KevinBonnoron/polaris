package polaris

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	modelsCacheTTL = 6 * time.Hour
	modelsEndpoint = "https://api.anthropic.com/v1/models"
)

var (
	modelsCache      []ModelInfo
	modelsCacheAt    time.Time
	modelsCacheMu    sync.Mutex
	claudeModelsHTTP = &http.Client{Timeout: 15 * time.Second}
)

// Claude API response types
type claudeModelEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type claudeModelsResponse struct {
	Data []claudeModelEntry `json:"data"`
}

// FetchClaudeModels returns Claude models, using a cache with TTL.
func FetchClaudeModels(force bool) ([]ModelInfo, error) {
	modelsCacheMu.Lock()
	defer modelsCacheMu.Unlock()

	if !force && modelsCache != nil && time.Since(modelsCacheAt) < modelsCacheTTL {
		return modelsCache, nil
	}

	models, err := fetchClaudeModelsLive()
	if err != nil {
		return nil, err
	}
	modelsCache = models
	modelsCacheAt = time.Now()
	return models, nil
}

func fetchClaudeModelsLive() ([]ModelInfo, error) {
	token, err := loadClaudeToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", usageUserAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := claudeModelsHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("claude token rejected (run `claude` to refresh): %s", strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("models endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data claudeModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	// The endpoint returns models newest-first. Keep the first (newest) model
	// per family; family is the word between "claude-" and the version number.
	seen := map[string]bool{}
	var out []ModelInfo
	for _, m := range data.Data {
		rest, ok := strings.CutPrefix(m.ID, "claude-")
		if !ok {
			continue
		}
		family, _, ok := strings.Cut(rest, "-")
		if !ok {
			continue
		}
		if seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, ModelInfo{Value: m.ID, Name: m.DisplayName, Family: family})
	}
	return out, nil
}
