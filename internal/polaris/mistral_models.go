package polaris

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// Mistral API response types
type mistralModelEntry struct {
	ID           string `json:"id"`
	Capabilities struct {
		CompletionChat bool `json:"completion_chat"`
	} `json:"capabilities"`
	Deprecated bool `json:"deprecated"`
}

type mistralModelsResponse struct {
	Data []mistralModelEntry `json:"data"`
}

// vibeConfig represents the structure of ~/.vibe/config.toml
type vibeConfig struct {
	Models []struct {
		Name     string `toml:"name"`
		Alias    string `toml:"alias"`
		Provider string `toml:"provider"`
	} `toml:"models"`
}

// listMistralModelsFromConfig reads ~/.vibe/config.toml and returns Mistral models.
// It returns nil if the file can't be read or parsed.
func listMistralModelsFromConfig() []ModelInfo {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(home, ".vibe", "config.toml"))
	if err != nil {
		return nil
	}

	var cfg vibeConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	var out []ModelInfo
	for _, m := range cfg.Models {
		if m.Provider == "mistral" && m.Alias != "" {
			out = append(out, ModelInfo{Value: m.Alias, Name: m.Alias, Family: ""})
		}
	}
	return out
}

func loadMistralAPIKey() (string, error) {
	if key := os.Getenv("MISTRAL_API_KEY"); key != "" {
		return key, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(home, ".vibe", ".env"))
	if err != nil {
		return "", fmt.Errorf("vibe credentials not found: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key, ok := strings.CutPrefix(strings.TrimSpace(line), "MISTRAL_API_KEY="); ok {
			return strings.Trim(key, `'"`), nil
		}
	}
	return "", fmt.Errorf("MISTRAL_API_KEY not found in ~/.vibe/.env")
}

// mistralFamily strips the trailing date version (-YYMM) or -latest suffix
// to produce a stable family name, e.g. "mistral-medium-2508" → "mistral-medium".
func mistralFamily(id string) string {
	idx := strings.LastIndex(id, "-")
	if idx == -1 {
		return id
	}
	suffix := id[idx+1:]
	if suffix == "latest" {
		return id[:idx]
	}
	allDigits := len(suffix) > 0
	for _, r := range suffix {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	if allDigits {
		return id[:idx]
	}
	return id
}

// FetchMistralModels returns Mistral models available in the local vibe config.
// It first tries to read ~/.vibe/config.toml for models that are known to work
// with the vibe CLI. If the config is not available, it falls back to the Mistral API.
// The API may return models that are not yet available in the vibe CLI.
func FetchMistralModels() ([]ModelInfo, error) {
	// Try local vibe config first - these are the models known to work with vibe CLI
	if models := listMistralModelsFromConfig(); len(models) > 0 {
		return models, nil
	}

	// Fallback: try the API
	key, err := loadMistralAPIKey()
	if err == nil {
		if models, apiErr := fetchMistralModelsFromAPI(key); apiErr == nil && len(models) > 0 {
			return models, nil
		}
	}

	return nil, fmt.Errorf("no Mistral models found: no models in ~/.vibe/config.toml and API unavailable")
}

// fetchMistralModelsFromAPI calls the Mistral API and returns models.
func fetchMistralModelsFromAPI(key string) ([]ModelInfo, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.mistral.ai/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call mistral models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mistral models endpoint returned %d", resp.StatusCode)
	}

	var data mistralModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode mistral models response: %w", err)
	}

	// Keep the first (newest) model per family; the API returns newest-first.
	seen := map[string]bool{}
	var out []ModelInfo
	for _, m := range data.Data {
		if m.Deprecated || !m.Capabilities.CompletionChat {
			continue
		}
		family := mistralFamily(m.ID)
		if seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, ModelInfo{Value: m.ID, Name: family, Family: ""})
	}
	return out, nil
}
