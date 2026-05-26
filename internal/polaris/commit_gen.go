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
	messagesEndpoint  = "https://api.anthropic.com/v1/messages"
	commitGenModel    = "claude-haiku-4-5-20251001"
	commitGenMaxToks  = 128
	commitGenTimeout  = 20 * time.Second
	commitGenVersion  = "2023-06-01"
	commitGenOAuthBeta = "oauth-2025-04-20"
)

type messagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []messagesMessage  `json:"messages"`
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

// GenerateCommitMessage calls the Anthropic Messages API to produce a
// conventional-commit subject line for the given staged diff. It tries
// ANTHROPIC_API_KEY first, then the Claude OAuth token from credentials.
func GenerateCommitMessage(diff string) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     commitGenModel,
		MaxTokens: commitGenMaxToks,
		System:    "You generate git commit messages following the Conventional Commits specification. Output only the subject line — no explanation, no quotes, no trailing period.",
		Messages: []messagesMessage{
			{Role: "user", Content: "Generate a commit message for this diff:\n\n" + diff},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), commitGenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", commitGenVersion)

	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		req.Header.Set("x-api-key", key)
	} else {
		tok, err := loadClaudeToken()
		if err != nil {
			return "", fmt.Errorf("no Anthropic credentials found (set ANTHROPIC_API_KEY or run `claude` to authenticate): %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("anthropic-beta", commitGenOAuthBeta)
	}

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
