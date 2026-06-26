package polaris

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Codex models cache structure
type codexModelsCache struct {
	Models []struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
		Visibility  string `json:"visibility"`
	} `json:"models"`
}

var fallbackCodexModels = []ModelInfo{
	{Value: "gpt-5.5", Name: "GPT-5.5"},
	{Value: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
}

func codexHomeDir() string {
	if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// ListCodexModels returns available Codex models from the local cache.
func ListCodexModels() []ModelInfo {
	if dir := codexHomeDir(); dir != "" {
		if models := ReadCodexModelsCache(filepath.Join(dir, "models_cache.json")); len(models) > 0 {
			return models
		}
	}

	return append([]ModelInfo(nil), fallbackCodexModels...)
}

// ReadCodexModelsCache parses the Codex models cache JSON file.
func ReadCodexModelsCache(path string) []ModelInfo {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cache codexModelsCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return nil
	}

	models := make([]ModelInfo, 0, len(cache.Models))
	seen := map[string]bool{}
	for _, m := range cache.Models {
		id := strings.TrimSpace(m.Slug)
		if id == "" || seen[id] || strings.EqualFold(strings.TrimSpace(m.Visibility), "hide") {
			continue
		}
		name := strings.TrimSpace(m.DisplayName)
		if name == "" {
			name = id
		}
		seen[id] = true
		models = append(models, ModelInfo{Value: id, Name: name})
	}

	return models
}
