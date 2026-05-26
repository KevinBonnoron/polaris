package polaris

import "strings"

// Mistral runs over ACP through the `vibe-acp` binary, which ships alongside the
// `vibe` CLI. It authenticates with its own config / MISTRAL_API_KEY from the
// user's environment, so we only inject the model (VIBE_ACTIVE_MODEL) and the
// `auto-approve` agent profile (VIBE_DEFAULT_AGENT), whose bypass_tool_permissions
// makes it run unattended like claude's bypassPermissions. Any VibeConfig field
// is overridable with the VIBE_ prefix.
func buildMistralEnv(model string) []string {
	env := []string{"VIBE_DEFAULT_AGENT=auto-approve"}
	if m := strings.TrimSpace(model); m != "" {
		env = append(env, "VIBE_ACTIVE_MODEL="+m)
	}
	return env
}
