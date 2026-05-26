package polaris

import (
	"regexp"
	"strings"
	"time"
)

// maxAPIRetries is how many extra attempts a claude-code turn gets after the
// first one fails with a transient upstream error (so up to maxAPIRetries+1
// total spawns).
const maxAPIRetries = 4

// overloadRetriesBeforeFallback is how many retries stay on the originally
// chosen model (giving its capacity a chance to recover) before switching to
// the fallback model — mirroring standard claude's Opus→Sonnet behaviour.
const overloadRetriesBeforeFallback = 2

// fallbackClaudeModel returns a less-congested model to retry with when the
// chosen one stays overloaded. Opus→Sonnet→(stop). An empty result means no
// further fallback (already on the smallest tier, or the model is custom/auto).
func fallbackClaudeModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return "sonnet"
	case strings.Contains(m, "sonnet"):
		return "haiku"
	default:
		return ""
	}
}

func fallbackGeminiModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "3-pro"):
		return "gemini-3-flash-preview"
	case strings.Contains(m, "2.5-pro"):
		return "gemini-2.5-flash"
	case strings.Contains(m, "flash") && !strings.Contains(m, "lite"):
		return "gemini-2.5-flash-lite"
	default:
		return "gemini-2.5-flash"
	}
}

// retryableAPIErrorRe matches transient upstream API failures (HTTP 429/5xx,
// "overloaded", rate limits) that claude or gemini surfaces after exhausting
// its own internal retries. A turn that hits one of these is worth re-running
// with backoff rather than failing the agent outright.
var retryableAPIErrorRe = regexp.MustCompile(`(?i)overloaded|api error:\s*(?:429|500|502|503|529)|too many requests|rate[ _-]?limit|service unavailable|exhausted your capacity`)

func isRetryableAPIError(line string) bool {
	return retryableAPIErrorRe.MatchString(line)
}

var technicalNoiseRe = regexp.MustCompile(`(?i)loaded cached credentials|attempt \d+ failed:.*retrying`)

// isTechnicalNoise returns true for CLI output lines that are purely technical
// and shouldn't clutter the user chat (e.g. auth messages, retry attempts).
func isTechnicalNoise(line string) bool {
	return technicalNoiseRe.MatchString(line)
}

var outputTokenLimitRe = regexp.MustCompile(`(?i)response exceeded the \d+ output token maximum`)

// outputTokenLimitError returns a short user-facing error message when line
// contains the Claude Code output-token-limit error, or "" if it doesn't match.
func outputTokenLimitError(line string) string {
	if !outputTokenLimitRe.MatchString(line) {
		return ""
	}
	return "Response exceeded the output token limit. Set CLAUDE_CODE_MAX_OUTPUT_TOKENS=64000 to increase it."
}

// retryBackoff returns the delay before the given 1-based retry attempt:
// 2s, 4s, 8s, 16s, then capped at 30s.
func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}
