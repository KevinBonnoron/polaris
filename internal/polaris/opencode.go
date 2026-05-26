package polaris

import (
	"encoding/json"
	"strings"
)

// opencode is the harness used to run user-defined custom providers. A provider
// (endpoint + key + models) is injected into opencode entirely via the
// OPENCODE_CONFIG_CONTENT env var, so nothing is written to the user's repo and
// the secret never touches disk. The API key is embedded as a literal in that
// JSON rather than via opencode's `{env:...}` interpolation, which is broken for
// apiKey (sst/opencode#19946).

type opencodeConfig struct {
	// omitempty matters: a nil Provider map marshals to `"provider":null`, which
	// opencode rejects ("Expected object | undefined, got null"), crashing the
	// plain-opencode session that injects no provider.
	Provider   map[string]opencodeProvider `json:"provider,omitempty"`
	Model      string                      `json:"model,omitempty"`
	Permission map[string]string           `json:"permission,omitempty"`
}

type opencodeProvider struct {
	NPM     string                   `json:"npm"`
	Name    string                   `json:"name"`
	Options opencodeProviderOptions  `json:"options"`
	Models  map[string]opencodeModel `json:"models"`
}

type opencodeProviderOptions struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey,omitempty"`
}

type opencodeModel struct {
	Name string `json:"name"`
}

// buildOpencodeEnv returns the extra environment for an opencode spawn: the
// inline provider config and an OPENAI_API_KEY (real or dummy) that prevents
// opencode's sub-session fallback from demanding a key (sst/opencode#20725).
func buildOpencodeEnv(p *CustomProvider, model string) ([]string, error) {
	npm := "@ai-sdk/openai-compatible"
	if p.APIType == "Anthropic-compatible" {
		npm = "@ai-sdk/anthropic"
	}

	models := map[string]opencodeModel{}
	for _, m := range p.Models {
		if m = strings.TrimSpace(m); m != "" {
			models[m] = opencodeModel{Name: m}
		}
	}
	if model = strings.TrimSpace(model); model != "" {
		if _, ok := models[model]; !ok {
			models[model] = opencodeModel{Name: model}
		}
	}

	opts := opencodeProviderOptions{BaseURL: strings.TrimSpace(p.Endpoint)}
	apiKey := strings.TrimSpace(p.APIKey)
	if apiKey != "" {
		opts.APIKey = apiKey
	}

	cfg := opencodeConfig{
		Provider: map[string]opencodeProvider{
			p.ID: {NPM: npm, Name: p.Name, Options: opts, Models: models},
		},
		// Default the session to the chosen provider/model so the ACP session
		// runs against it without a separate set-model round trip.
		Model: p.ID + "/" + model,
		// Run unattended like claude's bypassPermissions: allow the consequential
		// tools instead of prompting. `task` (subagents) stays denied — that's a
		// stability guard (weak local models loop on schema errors), not a
		// permission gate, and it isn't carried over ACP usefully.
		Permission: opencodePermissions(),
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	if apiKey == "" {
		apiKey = "opencode"
	}

	return []string{
		"OPENCODE_CONFIG_CONTENT=" + string(raw),
		"OPENAI_API_KEY=" + apiKey,
	}, nil
}

func opencodePermissions() map[string]string {
	return map[string]string{"edit": "allow", "write": "allow", "bash": "allow", "webfetch": "allow", "task": "deny"}
}

// buildOpencodeBaseEnv configures a plain opencode session (no custom provider):
// opencode uses its own authenticated providers; we only pin the chosen
// provider/model and the permission gates. model is a "provider/model" id from
// opencode's own model list.
func buildOpencodeBaseEnv(model string) ([]string, error) {
	cfg := opencodeConfig{Model: strings.TrimSpace(model), Permission: opencodePermissions()}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return []string{"OPENCODE_CONFIG_CONTENT=" + string(raw)}, nil
}
