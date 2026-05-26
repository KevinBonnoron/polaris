package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProviderHealth is the result of probing a custom provider's endpoint. Code is
// a stable machine token the frontend localises; Status carries the HTTP code
// for code == "http_error". ResolvedEndpoint is the canonical base URL we
// reached (e.g. the user pastes a bare host and we resolve the /v1 form).
// DiscoveredModels are the ids the endpoint reports, and UnknownModels are
// configured ids absent from that list.
type ProviderHealth struct {
	OK               bool     `json:"ok"`
	Code             string   `json:"code"`
	Status           int      `json:"status,omitempty"`
	LatencyMs        int64    `json:"latencyMs"`
	ResolvedEndpoint string   `json:"resolvedEndpoint,omitempty"`
	DiscoveredModels []string `json:"discoveredModels,omitempty"`
	UnknownModels    []string `json:"unknownModels,omitempty"`
	// ToolModels is the subset of DiscoveredModels that support tool calling
	// (required by the opencode harness). ToolModelsKnown reports whether
	// capability detection succeeded — it only works against Ollama's native
	// /api/show; for other endpoints we can't tell, so callers show every model.
	ToolModels      []string `json:"toolModels,omitempty"`
	ToolModelsKnown bool     `json:"toolModelsKnown,omitempty"`
}

const providerProbeTimeout = 10 * time.Second

// probeCandidate is one URL we try when discovering a provider. parse extracts
// model ids from the response body; resolved is the canonical base URL to
// report back when this candidate succeeds.
type probeCandidate struct {
	url      string
	header   http.Header
	parse    func([]byte) []string
	resolved string
}

func (s *Service) TestCustomProvider(p CustomProvider) (ProviderHealth, error) {
	return probeCustomProvider(context.Background(), p)
}

func probeCustomProvider(ctx context.Context, p CustomProvider) (ProviderHealth, error) {
	base := strings.TrimRight(strings.TrimSpace(p.Endpoint), "/")
	if base == "" {
		return ProviderHealth{Code: "missing_endpoint"}, nil
	}
	parsed, err := url.ParseRequestURI(base)
	if err != nil || !strings.HasPrefix(parsed.Scheme, "http") {
		return ProviderHealth{Code: "invalid_url"}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()

	start := time.Now()
	var lastStatus int
	for _, c := range buildCandidates(base, p.APIType, p.APIKey) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url, nil)
		if err != nil {
			continue
		}
		req.Header = c.header

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			code := "unreachable"
			if errors.Is(err, context.DeadlineExceeded) {
				code = "timeout"
			}
			return ProviderHealth{Code: code, LatencyMs: time.Since(start).Milliseconds()}, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return ProviderHealth{Code: "auth_failed", Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds()}, nil
		}
		if resp.StatusCode < 400 {
			var discovered []string
			if c.parse != nil {
				discovered = c.parse(body)
			}
			tools, toolsKnown := toolCapableModels(ctx, ollamaRoot(base), discovered, p.APIKey)
			return ProviderHealth{
				OK:               true,
				Code:             "ok",
				Status:           resp.StatusCode,
				LatencyMs:        time.Since(start).Milliseconds(),
				ResolvedEndpoint: c.resolved,
				DiscoveredModels: discovered,
				UnknownModels:    unknownModels(p.Models, discovered),
				ToolModels:       tools,
				ToolModelsKnown:  toolsKnown,
			}, nil
		}
		lastStatus = resp.StatusCode
	}
	return ProviderHealth{Code: "http_error", Status: lastStatus, LatencyMs: time.Since(start).Milliseconds()}, nil
}

// buildCandidates derives the URLs worth trying from a possibly-incomplete base.
// We strip a trailing /v1 first so a user can paste either the bare host
// (http://host:11434) or the /v1 form and converge on the same probes — Ollama,
// LM Studio and friends all expose /v1/models, and Ollama also answers /api/tags.
func buildCandidates(base, apiType, apiKey string) []probeCandidate {
	root := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1"), "/")
	switch apiType {
	case "Anthropic-compatible":
		h := http.Header{}
		h.Set("anthropic-version", "2023-06-01")
		if apiKey != "" {
			h.Set("x-api-key", apiKey)
		}
		return []probeCandidate{{root + "/v1/models", h, parseOpenAIModels, root}}
	case "OpenAI-compatible":
		h := http.Header{}
		if apiKey != "" {
			h.Set("Authorization", "Bearer "+apiKey)
		}
		return []probeCandidate{
			{root + "/v1/models", h, parseOpenAIModels, root + "/v1"},
			{root + "/api/tags", h, parseOllamaTags, root + "/v1"},
		}
	default:
		h := http.Header{}
		if apiKey != "" {
			h.Set("Authorization", "Bearer "+apiKey)
		}
		return []probeCandidate{{base, h, nil, base}}
	}
}

// ollamaRoot strips a trailing /v1 (and slashes) so the native Ollama API
// (/api/show) can be reached from an OpenAI-compatible base URL.
func ollamaRoot(base string) string {
	return strings.TrimRight(strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(base), "/"), "/v1"), "/")
}

// toolCapableModels asks Ollama's native /api/show for each model's
// capabilities and returns those that support tool calling. The second return
// reports whether detection worked: it's false for non-Ollama endpoints (no
// /api/show), unreachable hosts, or when no model could be inspected, so
// callers fall back to offering every model.
func toolCapableModels(parent context.Context, root string, models []string, apiKey string) ([]string, bool) {
	if root == "" || len(models) == 0 {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(parent, providerProbeTimeout)
	defer cancel()

	showURL := root + "/api/show"
	var tools []string
	determined := false
	for _, m := range models {
		reqBody, _ := json.Marshal(map[string]string{"model": m})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, showURL, bytes.NewReader(reqBody))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, false
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		status := resp.StatusCode
		resp.Body.Close()
		if status == http.StatusNotFound {
			return nil, false
		}
		if status >= 400 {
			continue
		}
		var payload struct {
			Capabilities []string `json:"capabilities"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			continue
		}
		determined = true
		for _, c := range payload.Capabilities {
			if c == "tools" {
				tools = append(tools, m)
				break
			}
		}
	}
	return tools, determined
}

func parseOpenAIModels(body []byte) []string {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	ids := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseOllamaTags(body []byte) []string {
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	ids := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		id := strings.TrimSpace(m.Name)
		if id == "" {
			id = strings.TrimSpace(m.Model)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func unknownModels(configured, discovered []string) []string {
	if len(discovered) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(discovered))
	for _, d := range discovered {
		known[d] = struct{}{}
	}
	var unknown []string
	for _, c := range configured {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := known[c]; !ok {
			unknown = append(unknown, c)
		}
	}
	return unknown
}
