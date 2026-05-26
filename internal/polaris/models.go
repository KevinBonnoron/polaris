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
	modelsCache   []ClaudeModel
	modelsCacheAt time.Time
	modelsCacheMu sync.Mutex
)

// ClaudeModel is the newest model of a Claude family, as advertised by the
// OAuth-authenticated /v1/models endpoint. Value is the stable CLI alias
// (opus|sonnet|haiku) passed to `claude --model`; Name is the API display name.
type ClaudeModel struct {
	Value  string `json:"value"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

type modelEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

// families orders the picker (opus first = default selection) and maps each id
// prefix to the alias we send to the CLI.
var families = []struct{ alias, prefix string }{
	{"opus", "claude-opus-"},
	{"sonnet", "claude-sonnet-"},
	{"haiku", "claude-haiku-"},
}

func FetchClaudeModels(force bool) ([]ClaudeModel, error) {
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

func fetchClaudeModelsLive() ([]ClaudeModel, error) {
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
	req.Header.Set("anthropic-beta", usageBetaHdr)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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

	var data modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	// The endpoint returns models newest-first, so the first match per family
	// is the most recent one.
	out := make([]ClaudeModel, 0, len(families))
	for _, fam := range families {
		for _, m := range data.Data {
			if strings.HasPrefix(m.ID, fam.prefix) {
				out = append(out, ClaudeModel{Value: fam.alias, Name: m.DisplayName, Family: fam.alias})
				break
			}
		}
	}
	return out, nil
}
