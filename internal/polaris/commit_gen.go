package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// errNoDirectCompletion marks an agent backend that has no directly reachable
// one-shot API (CLI-only agents, plain opencode). Only this case may fall back to
// the global route — a configured provider's auth/network failure must propagate,
// never silently reroute the prompt to Anthropic.
var errNoDirectCompletion = errors.New("no direct completion API")

const (
	messagesEndpoint    = "https://api.anthropic.com/v1/messages"
	mistralChatEndpoint = "https://api.mistral.ai/v1/chat/completions"
	commitGenModel      = "claude-haiku-4-5-20251001"
	mistralOneShotModel = "mistral-small-latest"
	commitGenMaxToks    = 128
	commitGenTimeout    = 20 * time.Second
	commitGenVersion    = "2023-06-01"
	commitGenOAuthBeta  = "oauth-2025-04-20"
	// ~125k tokens worth of diff; leaves headroom under the 200k context limit.
	commitGenMaxDiffBytes = 500_000
)

const commitGenSystem = "You generate git commit messages following the Conventional Commits specification. Output only the subject line — no explanation, no quotes, no trailing period."

// oneShotPrompt is a single stateless completion request shared by every
// short-lived generation (commit messages, conversation titles).
type oneShotPrompt struct {
	system    string
	user      string
	maxTokens int
}

// GenerateCommitMessage builds a commit-message prompt and routes it through
// completeOneShot. Used where there is no agent context (project-level commits).
func (s *Service) GenerateCommitMessage(diff string) (string, error) {
	return s.completeOneShot(commitMessagePrompt(diff))
}

// GenerateCommitMessageForAgent generates a commit message using the agent's own
// backend, so a non-Anthropic agent never relies on Anthropic credentials. Only a
// backend with no directly reachable API falls back to the global route; a
// configured provider's failure (auth, network, config) propagates so the diff is
// never silently sent to a different provider.
func (s *Service) GenerateCommitMessageForAgent(agent Agent, diff string) (string, error) {
	p := commitMessagePrompt(diff)
	msg, err := s.completeOneShotForAgent(agent, p)
	if err == nil {
		return msg, nil
	}
	if errors.Is(err, errNoDirectCompletion) {
		return s.completeOneShot(p)
	}
	return "", err
}

func commitMessagePrompt(diff string) oneShotPrompt {
	if len(diff) > commitGenMaxDiffBytes {
		diff = diff[:commitGenMaxDiffBytes] + "\n... (diff truncated)"
	}
	return oneShotPrompt{
		system:    commitGenSystem,
		user:      "Generate a commit message for this diff:\n\n" + diff,
		maxTokens: commitGenMaxToks,
	}
}

// completeOneShot routes a prompt to the user-configured provider when one is
// set (opencode default model → custom provider lookup), then falls back to the
// Anthropic API.
func (s *Service) completeOneShot(p oneShotPrompt) (string, error) {
	defaults, _ := s.store.GetAgentDefaultModels()
	if model := defaults["opencode"]; model != "" {
		if idx := strings.Index(model, "/"); idx > 0 {
			providerID := model[:idx]
			modelName := model[idx+1:]
			if prov, err := s.store.GetCustomProvider(providerID); err == nil && prov != nil {
				return generateWithCustomProvider(prov, modelName, p)
			}
		}
	}

	return generateWithAnthropic(p)
}

// completeOneShotForAgent routes a one-shot completion to the same backend the
// agent itself runs on, so e.g. a Mistral agent's title/commit generation never
// depends on Anthropic credentials. Returns an error — letting callers fall back —
// when the agent's backend exposes no directly reachable completion API (CLI-only
// agents such as codex, gemini, copilot, cursor, or plain opencode).
func (s *Service) completeOneShotForAgent(agent Agent, p oneShotPrompt) (string, error) {
	if agent.ProviderID != "" {
		prov, err := s.store.GetCustomProvider(agent.ProviderID)
		if err != nil || prov == nil {
			return "", fmt.Errorf("custom provider %q unavailable", agent.ProviderID)
		}
		return generateWithCustomProvider(prov, agent.Model, p)
	}
	switch agent.Kind {
	case "claude-code", "":
		return generateWithAnthropic(p)
	case "mistral":
		return generateWithMistral(agent.Model, p)
	default:
		return "", fmt.Errorf("%w for agent kind %q", errNoDirectCompletion, agent.Kind)
	}
}

// generateWithMistral calls the Mistral API (OpenAI-compatible chat completions)
// with the key from MISTRAL_API_KEY or ~/.vibe/.env, mirroring how Mistral agents
// authenticate.
func generateWithMistral(model string, p oneShotPrompt) (string, error) {
	key, err := loadMistralAPIKey()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(model) == "" {
		model = mistralOneShotModel
	}
	return generateOpenAIChat(mistralChatEndpoint, key, model, p)
}

func generateWithCustomProvider(prov *CustomProvider, model string, p oneShotPrompt) (string, error) {
	if prov.APIType == "Anthropic-compatible" {
		return generateAnthropicMessages(strings.TrimRight(prov.Endpoint, "/")+"/messages", prov.APIKey, model, p)
	}
	return generateOpenAIChat(strings.TrimRight(prov.Endpoint, "/")+"/chat/completions", prov.APIKey, model, p)
}

// generateWithAnthropic calls the public Anthropic API, using ANTHROPIC_API_KEY
// or the Claude OAuth token.
func generateWithAnthropic(p oneShotPrompt) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey != "" {
		return generateAnthropicMessages(messagesEndpoint, apiKey, commitGenModel, p)
	}
	tok, err := loadClaudeToken()
	if err != nil {
		return "", fmt.Errorf("no Anthropic credentials found (set ANTHROPIC_API_KEY or run `claude` to authenticate): %w", err)
	}
	return generateAnthropicMessagesOAuth(messagesEndpoint, tok, p)
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

func generateAnthropicMessages(endpoint, apiKey, model string, p oneShotPrompt) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     model,
		MaxTokens: p.maxTokens,
		System:    p.system,
		Messages:  []messagesMessage{{Role: "user", Content: p.user}},
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

func generateAnthropicMessagesOAuth(endpoint, token string, p oneShotPrompt) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     commitGenModel,
		MaxTokens: p.maxTokens,
		System:    p.system,
		Messages:  []messagesMessage{{Role: "user", Content: p.user}},
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

func generateOpenAIChat(endpoint, apiKey, model string, p oneShotPrompt) (string, error) {
	body, err := json.Marshal(openAIChatRequest{
		Model:     model,
		MaxTokens: p.maxTokens,
		Messages: []openAIChatMsg{
			{Role: "system", Content: p.system},
			{Role: "user", Content: p.user},
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
