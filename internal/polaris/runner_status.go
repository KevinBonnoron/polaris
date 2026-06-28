package polaris

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/KevinBonnoron/polaris/internal/providers/git"
)

// runnerError records a failure once the log file is already open: the message
// is appended to disk and emitted live, then the status flips to "error".
func (service *Service) runnerError(agentID string, logFile io.Writer, msg string) {
	evt := StreamEvent{Type: "system", Content: "✗ " + msg}
	emitEvent(logFile, nil, evt)
	service.emitLogEvent(agentID, evt)
	_ = service.store.PatchAgent(agentID, map[string]any{"status": "error"})
	service.notifyAgentEvent(agentID, "error", msg)
}

// markAgentError records a failure that happens outside the streaming pipeline
// (e.g. an ACP session that dies during bootstrap). The message is appended to
// the log file so it survives a reload — emitting live only would lose it for a
// viewer that opens the modal after the event fired.
func (service *Service) markAgentError(agentID, msg string) {
	if service.store == nil {
		return
	}

	_ = service.appendAgentEvent(agentID, StreamEvent{Type: "system", Content: "✗ " + msg})

	_ = service.store.PatchAgent(agentID, map[string]any{"status": "error"})
	service.notifyAgentEvent(agentID, "error", msg)
}

// persistTurnStats folds the per-turn stream-json stats into the agent row.
// Tokens and cost are additive; filesModified is re-derived from the full log
// to avoid double-counting files touched in multiple turns.
func (service *Service) persistTurnStats(agentID string, stats streamTurnStats) {
	if service.store == nil {
		return
	}

	if stats.Tokens == 0 && stats.CostUSD == 0 && stats.FilesModified == 0 && stats.ToolsUsed == 0 {
		return
	}

	agent, err := service.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	filesModified := agent.FilesModified + stats.FilesModified
	if logContent, logErr := service.ReadLog(agentID); logErr == nil && logContent != "" {
		logPaths := git.ExtractLogFilePaths(logContent)
		filesModified = len(logPaths)
		// Keep only the files git actually reports as changed. This drops scratch
		// files the agent wrote outside the repo (e.g. a plan .md) that never show
		// in the changed-files list, while staying session-specific: we start from
		// this agent's own log paths, so another agent's edits on the same branch
		// are never folded in.
		if dir := service.agentStatsDir(agent); dir != "" {
			if changed, cerr := git.AgentChangedPaths(dir); cerr == nil {
				filesModified = countChangedLogPaths(dir, logPaths, changed)
			}
		}
	}

	patch := map[string]any{
		"tokens":        agent.Tokens.Add(stats.Parts),
		"costUsd":       agent.CostUSD + stats.CostUSD,
		"filesModified": filesModified,
		"toolsUsed":     agent.ToolsUsed + stats.ToolsUsed,
	}
	_ = service.store.PatchAgent(agentID, patch)
}

// agentStatsDir resolves the git directory whose changes count toward the
// agent: its worktree when isolated, otherwise the project root.
func (service *Service) agentStatsDir(agent *Agent) string {
	if agent.Worktree.Path != "" {
		if info, err := os.Stat(agent.Worktree.Path); err == nil && info.IsDir() {
			return agent.Worktree.Path
		}
	}
	if project, err := service.store.GetProject(agent.ProjectID); err == nil && project != nil {
		return project.Path
	}
	return ""
}

// countChangedLogPaths counts how many of the agent's log-derived file paths git
// actually reports as changed in dir. Paths are normalised to absolute (git
// reports repo-relative) so a plan/scratch file outside the repo is excluded.
func countChangedLogPaths(dir string, logPaths, changed []string) int {
	set := make(map[string]struct{}, len(changed))
	for _, p := range changed {
		set[filepath.Clean(filepath.Join(dir, p))] = struct{}{}
	}
	n := 0
	for _, lp := range logPaths {
		abs := lp
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(dir, abs)
		}
		if _, ok := set[filepath.Clean(abs)]; ok {
			n++
		}
	}
	return n
}

func (service *Service) markAgentCompleted(agentID string) {
	if service.store == nil {
		return
	}

	// claude-code in --print mode exits after emitting the AskUserQuestion
	// tool_use without waiting for the tool_result, so cmd.Wait returns
	// cleanly and we land here while a question is still pending. Treating
	// that as "completed" would hide the choices behind a green dot — stay
	// in "waiting" until RespondToAgentQuestion clears the question (which
	// then drives a resume turn that finishes normally).
	agent, _ := service.store.GetAgent(agentID)
	if agent != nil && agent.PendingQuestion != nil {
		return
	}

	_ = service.store.PatchAgent(agentID, map[string]any{"status": "completed"})
	service.notifyAgentEvent(agentID, "completed", "")
}

// markAgentStopped finalises an agent the user cancelled. The session is left
// resumable (sessionId is kept); any pending question is cleared so its panel
// disappears.
func (service *Service) markAgentStopped(agentID string) {
	if service.store == nil {
		return
	}
	_ = service.appendAgentEvent(agentID, StreamEvent{Type: "system", Content: "(stopped by user)"})
	_ = service.store.PatchAgent(agentID, map[string]any{"status": "stopped", "pendingQuestion": nil})
}

// markAgentWaiting flips the agent to "waiting" when the subprocess emits an
// interactive prompt (e.g. `[y/N]`). The status reverts to "working" the next
// time Send writes to stdin. Re-emitting from the stream parser on every byte
// would spam: we only patch when the recorded status isn't already "waiting".
func (service *Service) markAgentWaiting(agentID string) {
	if service.store == nil {
		return
	}
	agent, err := service.store.GetAgent(agentID)
	if err != nil || agent == nil || agent.Status == "waiting" {
		return
	}
	_ = service.store.PatchAgent(agentID, map[string]any{"status": "waiting"})
	service.notifyAgentEvent(agentID, "waiting", "")
}

// notifyAgentEvent emits a Notification tied to an agent's lifecycle. The
// title falls back to the agent's task summary so the inbox row stays
// meaningful even when the underlying reason is empty. The kind ("completed",
// "error", "waiting") is mapped to an AgentEvent + severity; users who don't
// want a given category can mute it via NotificationSettings.
func (service *Service) notifyAgentEvent(agentID, kind, detail string) {
	if service.store == nil {
		return
	}

	agent, err := service.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	event, severity := agentEventFromKind(kind)
	_, _ = service.Notify(NotifyInput{
		ProjectID:     agent.ProjectID,
		Type:          NotifTypeAgent,
		Severity:      severity,
		TitleTemplate: agentNotificationTitle(service, agent, event, severity, detail),
		Payload:       map[string]any{"agentId": agent.ID, "event": string(event)},
	})
}

func agentEventFromKind(kind string) (AgentEvent, NotificationSeverity) {
	switch kind {
	case "waiting":
		return AgentEventWaiting, SeverityInfo
	case "completed":
		return AgentEventCompleted, SeveritySuccess
	case "error":
		return AgentEventCompleted, SeverityError
	default:
		return AgentEventCompleted, SeverityInfo
	}
}

func agentKindLabel(service *Service, agent *Agent) string {
	if agent.ProviderID != "" && service != nil && service.store != nil {
		if prov, err := service.store.GetCustomProvider(agent.ProviderID); err == nil && prov != nil && strings.TrimSpace(prov.Name) != "" {
			return prov.Name
		}
	}
	switch agent.Kind {
	case "claude-code":
		return "Claude Code"
	case "copilot":
		return "Copilot"
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini"
	case "cursor":
		return "Cursor"
	case "aider":
		return "Aider"
	default:
		return agent.Kind
	}
}

func agentNotificationTitle(service *Service, agent *Agent, event AgentEvent, severity NotificationSeverity, detail string) string {
	task := strings.TrimSpace(agent.Summary)
	if len(task) > 80 {
		task = task[:77] + "..."
	}
	label := agentKindLabel(service, agent)

	switch {
	case event == AgentEventCompleted && severity == SeverityError:
		if detail != "" {
			return fmt.Sprintf("%s failed: %s", label, detail)
		}
		if task != "" {
			return fmt.Sprintf("%s failed: %s", label, task)
		}
		return fmt.Sprintf("%s failed", label)

	case event == AgentEventCompleted:
		if task != "" {
			return fmt.Sprintf("%s completed: %s", label, task)
		}
		return fmt.Sprintf("%s completed", label)

	default:
		if task != "" {
			return fmt.Sprintf("%s needs input: %s", label, task)
		}
		return fmt.Sprintf("%s needs input", label)
	}
}
