package polaris

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Gemini API response types
type geminiModelEntry struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

type geminiModelsResponse struct {
	Models []geminiModelEntry `json:"models"`
}

func loadGeminiAPIKey() (string, error) {
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("GEMINI_API_KEY environment variable not set")
}

// FetchGeminiModels calls the Generative Language API using the GEMINI_API_KEY
// environment variable and returns all gemini-* models that support generateContent.
func FetchGeminiModels() ([]ModelInfo, error) {
	key, err := loadGeminiAPIKey()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, "https://generativelanguage.googleapis.com/v1beta/models", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("key", key)
	q.Set("pageSize", "100")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call gemini models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini models endpoint returned %d", resp.StatusCode)
	}

	var data geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode gemini models response: %w", err)
	}

	var out []ModelInfo
	for _, m := range data.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if !strings.HasPrefix(id, "gemini-") {
			continue
		}
		canGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				canGenerate = true
				break
			}
		}
		if !canGenerate {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = id
		}
		out = append(out, ModelInfo{Value: id, Name: name, Family: ""})
	}
	return out, nil
}
