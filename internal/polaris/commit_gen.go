package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	messagesEndpoint   = "https://api.anthropic.com/v1/messages"
	commitGenModel     = "claude-haiku-4-5-20251001"
	commitGenMaxToks   = 128
	commitGenTimeout   = 20 * time.Second
	commitGenVersion   = "2023-06-01"
	commitGenOAuthBeta = "oauth-2025-04-20"
	// ~125k tokens worth of diff; leaves headroom under the 200k context limit.
	commitGenMaxDiffBytes = 500_000
)

const commitGenSystem = "You generate git commit messages following the Conventional Commits specification. Output only the subject line — no explanation, no quotes, no trailing period."

// GenerateCommitMessage routes to the user-configured provider when one is set
// (opencode default model → custom provider lookup), then falls back to the
// Anthropic API.
func (s *Service) GenerateCommitMessage(diff string) (string, error) {
	if len(diff) > commitGenMaxDiffBytes {
		diff = diff[:commitGenMaxDiffBytes] + "\n... (diff truncated)"
	}

	defaults, _ := s.store.GetAgentDefaultModels()
	if model := defaults["opencode"]; model != "" {
		if idx := strings.Index(model, "/"); idx > 0 {
			providerID := model[:idx]
			modelName := model[idx+1:]
			if p, err := s.store.GetCustomProvider(providerID); err == nil && p != nil {
				return generateWithCustomProvider(p, modelName, diff)
			}
		}
	}

	return generateWithAnthropic(diff)
}

func generateWithCustomProvider(p *CustomProvider, model, diff string) (string, error) {
	if p.APIType == "Anthropic-compatible" {
		return generateAnthropicMessages(strings.TrimRight(p.Endpoint, "/")+"/messages", p.APIKey, model, diff)
	}
	return generateOpenAIChat(strings.TrimRight(p.Endpoint, "/")+"/chat/completions", p.APIKey, model, diff)
}

// generateWithAnthropic calls the public Anthropic API, using ANTHROPIC_API_KEY
// or the Claude OAuth token.
func generateWithAnthropic(diff string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey != "" {
		return generateAnthropicMessages(messagesEndpoint, apiKey, commitGenModel, diff)
	}
	tok, err := loadClaudeToken()
	if err != nil {
		return "", fmt.Errorf("no Anthropic credentials found (set ANTHROPIC_API_KEY or run `claude` to authenticate): %w", err)
	}
	return generateAnthropicMessagesOAuth(messagesEndpoint, tok, diff)
}

type messagesRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system"`
	Messages  []messagesMessage `json:"messages"`
}

type messagesMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func generateAnthropicMessages(endpoint, apiKey, model, diff string) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     model,
		MaxTokens: commitGenMaxToks,
		System:    commitGenSystem,
		Messages:  []messagesMessage{{Role: "user", Content: "Generate a commit message for this diff:\n\n" + diff}},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commitGenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", commitGenVersion)
	req.Header.Set("x-api-key", apiKey)
	return doMessagesRequest(req)
}

func generateAnthropicMessagesOAuth(endpoint, token, diff string) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     commitGenModel,
		MaxTokens: commitGenMaxToks,
		System:    commitGenSystem,
		Messages:  []messagesMessage{{Role: "user", Content: "Generate a commit message for this diff:\n\n" + diff}},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commitGenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", commitGenVersion)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", commitGenOAuthBeta)
	return doMessagesRequest(req)
}

func doMessagesRequest(req *http.Request) (string, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call messages API: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out messagesResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("API error: %s", out.Error.Message)
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}
	return strings.TrimSpace(out.Content[0].Text), nil
}

type openAIChatRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []openAIChatMsg `json:"messages"`
}

type openAIChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMsg `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func generateOpenAIChat(endpoint, apiKey, model, diff string) (string, error) {
	body, err := json.Marshal(openAIChatRequest{
		Model:     model,
		MaxTokens: commitGenMaxToks,
		Messages: []openAIChatMsg{
			{Role: "system", Content: commitGenSystem},
			{Role: "user", Content: "Generate a commit message for this diff:\n\n" + diff},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commitGenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out openAIChatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("API error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
