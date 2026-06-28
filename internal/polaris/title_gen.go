package polaris

import (
	"fmt"
	"log"
	"strings"
	"unicode/utf8"
)

const (
	titleGenMaxToks       = 32
	titleGenMaxInputRunes = 8_000
)

const titleGenSystem = "You write a very short title (at most 6 words) summarizing what a user's request is about, used to label a conversation in a sidebar. Reply with only the title: no surrounding quotes, no trailing punctuation, no preamble. Write the title in the same language as the user's request."

// GenerateAgentTitle asks the agent's own backend to summarize the initial user
// request into a short conversation title, in the same language as the request.
// Routing through the agent's backend keeps a non-Anthropic agent (e.g. Mistral)
// from depending on Anthropic credentials.
func (s *Service) GenerateAgentTitle(agent Agent, task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("empty task")
	}
	task = truncateRunes(task, titleGenMaxInputRunes)
	out, err := s.completeOneShotForAgent(agent, oneShotPrompt{
		system:    titleGenSystem,
		user:      "Summarize this request as a short conversation title:\n\n" + task,
		maxTokens: titleGenMaxToks,
	})
	if err != nil {
		return "", err
	}
	return sanitizeTitle(out), nil
}

// applyGeneratedTitle generates a title for the agent's initial request and
// persists it as the summary. The summary stays empty while generation is
// pending; only if generation fails (or yields nothing) is it backfilled with
// the truncated fallback. Meant to run in a goroutine.
func (s *Service) applyGeneratedTitle(agent Agent, task, fallback string) {
	title, err := s.GenerateAgentTitle(agent, task)
	if err != nil || title == "" {
		title = fallback
	}
	if title == "" {
		return
	}
	if err := s.store.PatchAgent(agent.ID, map[string]any{"summary": title}); err != nil {
		log.Printf("agent %s: persist generated title: %v", agent.ID, err)
	}
}

// sanitizeTitle strips the wrapping a model commonly adds (quotes, trailing
// punctuation, extra lines) and caps the length.
func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".")
	s = strings.TrimSpace(s)
	return capSummary(s)
}

// summaryFromTask renders a one-line, length-capped fallback summary from a raw
// task string.
func summaryFromTask(task string) string {
	s := task
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return capSummary(s)
}

// capSummary caps a summary at 200 runes, appending an ellipsis when trimmed.
func capSummary(s string) string {
	if utf8.RuneCountInString(s) <= 200 {
		return s
	}
	return truncateRunes(s, 197) + "..."
}

// truncateRunes caps s to at most n runes without splitting a multibyte
// character, so non-ASCII text (accents, CJK, etc.) is never corrupted.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
