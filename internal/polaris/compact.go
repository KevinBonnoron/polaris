package polaris

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const compactMaxInputRunes = 60_000
const compactMaxTokens = 8_000

const compactSystem = `You are a helpful assistant that summarizes conversations between a user and an AI coding agent. Your summary will be used to continue the conversation in a new session, so it must preserve all context needed to continue the work.

Include:
- The user's original task or goal
- Key decisions made and reasoning behind them
- Files created, modified, or deleted and what was changed
- Important findings (errors encountered, discoveries, constraints)
- Current state of the work and what remains to be done
- Any specific instructions, preferences, or requirements the user expressed

Be thorough but concise. Write in a clear, structured format.`

// buildConversationText extracts user messages and assistant text from the log
// events to form a readable conversation transcript for summarization.
func buildConversationText(events []StreamEvent) string {
	var b strings.Builder
	for _, ev := range events {
		switch ev.Type {
		case "compact":
			b.WriteString("[Previous summary: ")
			b.WriteString(ev.Content)
			b.WriteString("]\n\n")
		case "user_message":
			b.WriteString("User: ")
			b.WriteString(ev.Content)
			b.WriteString("\n\n")
		case "text":
			b.WriteString("Assistant: ")
			b.WriteString(ev.Content)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// Compact summarizes the agent's conversation log using the agent's own backend,
// stops the current session, truncates the log to the summary event, then
// restarts the session with the summary as context. Runs in a goroutine from
// the Wails layer; surfaces errors as system events rather than returning them.
func (service *Service) Compact(agentID string) {
	if err := service.compact(agentID); err != nil {
		service.markAgentError(agentID, "compact failed: "+err.Error())
	}
}

func (service *Service) compact(agentID string) error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	if agentID == "" {
		return errors.New("agentId is required")
	}

	if _, loaded := service.compacting.LoadOrStore(agentID, true); loaded {
		return errors.New("compact already in progress for this agent")
	}
	defer service.compacting.Delete(agentID)

	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}

	_ = service.store.PatchAgent(agentID, map[string]any{"status": "working"})
	progressEvt := StreamEvent{Type: "system", Content: "Compacting conversation…"}
	_ = service.appendAgentEvent(agentID, progressEvt)
	service.emitLogEvent(agentID, progressEvt)

	// Grab the claude session before cancelling so we can wait for its writer
	// to finish before we read the log — prevents a partial read mid-write.
	service.runner.mu.Lock()
	cs := service.runner.claude[agentID]
	service.runner.mu.Unlock()

	_ = service.runner.cancel(agentID)

	if cs != nil {
		select {
		case <-cs.done:
		case <-time.After(3 * time.Second):
		}
	}

	events, err := service.ReadLogEvents(agentID)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}

	conversation := buildConversationText(events)
	conversation = truncateRunes(conversation, compactMaxInputRunes)
	if conversation == "" {
		return errors.New("no conversation to compact")
	}

	summary, err := service.completeOneShotForAgent(*agent, oneShotPrompt{
		system:    compactSystem,
		user:      "Summarize this conversation between a user and an AI coding agent:\n\n" + conversation,
		maxTokens: compactMaxTokens,
	})
	if err != nil {
		if !errors.Is(err, errNoDirectCompletion) {
			return fmt.Errorf("generate summary: %w", err)
		}
		summary, err = service.completeOneShot(oneShotPrompt{
			system:    compactSystem,
			user:      "Summarize this conversation between a user and an AI coding agent:\n\n" + conversation,
			maxTokens: compactMaxTokens,
		})
		if err != nil {
			return fmt.Errorf("generate summary: %w", err)
		}
	}

	logPath := filepath.Join(service.runner.logsRoot, agentID+".log")
	compactEvt := StreamEvent{Type: "compact", Content: summary}
	line := marshalEvent(compactEvt) + "\n"
	if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
		return fmt.Errorf("truncate log: %w", err)
	}
	service.emitLogResetEvent(agentID)

	contextMsg := "Summary of our previous conversation:\n\n" + summary

	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	workDir := agentRunDir(agent, project)

	switch agent.Kind {
	case "claude-code":
		sessionID := newSessionUUID()
		if err := service.store.PatchAgent(agentID, map[string]any{
			"sessionId": sessionID,
			"status":    "working",
		}); err != nil {
			return fmt.Errorf("patch agent: %w", err)
		}
		agent, err = service.store.GetAgent(agentID)
		if err != nil || agent == nil {
			return fmt.Errorf("re-fetch agent: %w", err)
		}
		return service.runner.startClaudeSession(service, agent, workDir, contextMsg, "", true, false)

	case "opencode", "mistral":
		acpEnv, err := service.buildACPEnv(agent.Kind, agent.ProviderID, agent.Model)
		if err != nil {
			return err
		}
		rt, err := service.acpRuntime(agent.Kind, "")
		if err != nil {
			return err
		}
		if err := service.store.PatchAgent(agentID, map[string]any{
			"sessionId": "",
			"status":    "working",
		}); err != nil {
			return fmt.Errorf("patch agent: %w", err)
		}
		env := append(os.Environ(), acpEnv...)
		return service.runner.startACPSession(service, agentID, rt, workDir, contextMsg, env, 0, 0, "")

	default:
		_ = service.store.PatchAgent(agentID, map[string]any{"status": "idle"})
		return nil
	}
}
