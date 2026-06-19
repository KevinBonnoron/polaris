package polaris

import (
	"fmt"
	"strings"
)

const (
	titleGenMaxToks      = 32
	titleGenMaxInputByte = 8_000
)

const titleGenSystem = "You write a very short title (at most 6 words) summarizing what a user's request is about, used to label a conversation in a sidebar. Reply with only the title: no surrounding quotes, no trailing punctuation, no preamble. Write the title in the same language as the user's request."

// GenerateAgentTitle asks the configured model to summarize the initial user
// request into a short conversation title, in the same language as the request.
func (s *Service) GenerateAgentTitle(task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("empty task")
	}
	if len(task) > titleGenMaxInputByte {
		task = task[:titleGenMaxInputByte]
	}
	out, err := s.completeOneShot(oneShotPrompt{
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
func (s *Service) applyGeneratedTitle(agentID, task, fallback string) {
	title, err := s.GenerateAgentTitle(task)
	if err != nil || title == "" {
		title = fallback
	}
	if title == "" {
		return
	}
	_ = s.store.PatchAgent(agentID, map[string]any{"summary": title})
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
	if len(s) > 200 {
		s = s[:197] + "..."
	}
	return s
}
