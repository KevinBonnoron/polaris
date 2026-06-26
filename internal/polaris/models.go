package polaris

// ModelInfo describes a model discovered from a CLI or provider API.
// Value is the identifier passed to the CLI (e.g. "opus", "mistral-medium-3.5").
// Name is the human-readable display name. Family groups related models
// (e.g. "opus", "sonnet", "haiku" for Claude; empty for CLIs that don't expose it).
type ModelInfo struct {
	Value  string `json:"value"`
	Name   string `json:"name"`
	Family string `json:"family"`
}
