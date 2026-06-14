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
)

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

// FetchMistralModels calls the Mistral API using the key from MISTRAL_API_KEY or
// ~/.vibe/.env and returns one model per family (newest first, deprecated excluded).
func FetchMistralModels() ([]ModelInfo, error) {
	key, err := loadMistralAPIKey()
	if err != nil {
		return nil, err
	}

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

	var data struct {
		Data []struct {
			ID           string `json:"id"`
			Capabilities struct {
				CompletionChat bool `json:"completion_chat"`
			} `json:"capabilities"`
			Deprecated bool `json:"deprecated"`
		} `json:"data"`
	}
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
