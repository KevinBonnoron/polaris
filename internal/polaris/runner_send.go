package polaris

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (service *Service) Send(agentID, message string) error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("message is required")
	}
	if agentID == "" {
		return errors.New("agentId is required")
	}

	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}

	// A new user message supersedes any pending AskUserQuestion: the user
	// chose to answer via free text instead of the choices, so drop the
	// persisted question so the panel disappears.
	hadPending := agent.PendingQuestion != nil
	if hadPending {
		_ = service.store.PatchAgent(agentID, map[string]any{"pendingQuestion": nil})
	}

	// ACP-based agents (opencode, mistral) run over a persistent session: a
	// follow-up is a new prompt on the same process (queued if a turn is in
	// flight).
	if agent.Kind == "opencode" || agent.Kind == "mistral" {
		service.runner.mu.Lock()
		a := service.runner.acp[agentID]
		service.runner.mu.Unlock()
		if a == nil {
			// Process gone (finished session / app restart): respawn the backend
			// and resume the session via session/load, then run this message.
			return service.resumeACP(agent, message)
		}
		_ = service.appendAgentEvent(agentID, StreamEvent{Type: "user_message", Content: message})
		_ = service.store.PatchAgent(agentID, map[string]any{"status": "working"})
		a.prompt(service.runner, message)
		return nil
	}

	// claude-code also runs over a persistent session: a follow-up is a new user
	// turn on the same process (queued if a turn is in flight), respawned with
	// --resume only when the process is gone (finished / app restart).
	if agent.Kind == "claude-code" {
		service.runner.mu.Lock()
		c := service.runner.claude[agentID]
		service.runner.mu.Unlock()
		if c == nil {
			return service.resumeClaude(agent, message)
		}
		_ = service.store.PatchAgent(agentID, map[string]any{"status": "working"})
		// The session decides whether this is logged now (immediate) or queued as a
		// pending chip and logged when picked up — so the message is not appended
		// here. Superseding a pending AskUserQuestion: the turn is blocked waiting
		// for a tool_result, so close the question (dismissal tool_result) to
		// unblock it; the new message then drains as the next turn.
		if hadPending && agent.PendingQuestion != nil {
			// Resolve the surfaced question in the transcript (it has no real
			// tool_result) and keep it reviewable, paired with the free-text reply.
			_ = service.appendAgentEvent(agentID, StreamEvent{Type: "tool_result", ID: agent.PendingQuestion.ToolUseID, Content: summarizeAnswer(message), RenderedContent: questionAnswerRecap(agent.PendingQuestion.Input, message)})
			c.supersedeQuestion(message)
		} else {
			c.sendOrQueue(message)
		}
		return nil
	}

	appendedUserMessage := false

	// 1. If the agent is already running, try to deliver the message to the live process.
	if service.runner.isRunning(agentID) {
		// Turn-based CLIs use a queue; their prompt is passed as command args,
		// not delivered to a live stdin pipe.
		if agent.Kind == "cursor" || agent.Kind == "codex" {
			if err := service.appendAgentEvent(agentID, StreamEvent{Type: "user_message", Content: message}); err != nil {
				return fmt.Errorf("write log: %w", err)
			}
			appendedUserMessage = true
			if service.runner.queueIfRunning(agentID, message) {
				return nil
			}
			// The process exited between isRunning and queueIfRunning. Fall
			// through to the resume path without logging the message twice.
		}

		// Interactive CLIs (copilot) use direct stdin.
		if err := service.appendAgentEvent(agentID, StreamEvent{Type: "user_message", Content: message}); err != nil {
			return fmt.Errorf("write log: %w", err)
		}

		if !service.runner.writeStdin(agentID, message) {
			return fmt.Errorf("cannot write to %q stdin", agent.Kind)
		}

		_ = service.store.PatchAgent(agentID, map[string]any{"status": "working"})
		return nil
	}

	// 2. Not running: try to resume the session with a fresh process.
	if agent.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}

	if !appendedUserMessage {
		if err := service.appendAgentEvent(agentID, StreamEvent{Type: "user_message", Content: message}); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
	}

	// Already running: queue and let the in-flight turn pick it up.
	if service.runner.queueIfRunning(agentID, message) {
		return nil
	}

	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}

	workDir := agentRunDir(agent, project)
	binary, args, env, err := service.buildResume(agent, message)
	if err != nil {
		return err
	}

	if err := service.store.PatchAgent(agentID, map[string]any{"status": "working"}); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// startMsg empty: we already wrote it above before queueing/spawning.
	// claude-code never reaches here (handled by its persistent-session branch).
	go service.runner.run(service, agentID, agent.Kind, binary, args, workDir, "", true, env, nil)
	return nil
}

// resumeClaude respawns a finished claude-code session as a fresh persistent
// process via --resume and delivers message as its first turn. Mirrors resumeACP.
func (service *Service) resumeClaude(agent *Agent, message string) error {
	if agent.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}
	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	workDir := agentRunDir(agent, project)
	_ = service.appendAgentEvent(agent.ID, StreamEvent{Type: "user_message", Content: message})
	_ = service.store.PatchAgent(agent.ID, map[string]any{"status": "working"})
	return service.runner.startClaudeSession(service, agent, workDir, message, "", true, true)
}

// InterruptAndSend aborts the in-flight claude-code turn and applies message
// immediately, instead of queueing it until the current turn ends. Only the
// persistent claude session supports a mid-turn interrupt; for any other state
// it degrades to an ordinary Send (the message is queued / resumes a session).
func (service *Service) InterruptAndSend(agentID, message string) error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("message is required")
	}
	if agentID == "" {
		return errors.New("agentId is required")
	}
	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}
	if agent.PendingQuestion != nil {
		_ = service.store.PatchAgent(agentID, map[string]any{"pendingQuestion": nil})
	}

	service.runner.mu.Lock()
	cs := service.runner.claude[agentID]
	service.runner.mu.Unlock()
	if cs == nil {
		return service.Send(agentID, message)
	}
	_ = service.store.PatchAgent(agentID, map[string]any{"status": "working"})
	// The message is logged when the aborted turn picks it up (interruptAndSend
	// queues it), so it lands in the transcript where it was taken into account.
	cs.interruptAndSend(message)
	return nil
}

// ClearQueuedMessage drops the agent's pending follow-up (the chip), used when
// the user pulls it back into the input to edit it.
func (service *Service) ClearQueuedMessage(agentID string) error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	if agentID == "" {
		return errors.New("agentId is required")
	}
	service.runner.clearPending(agentID)
	return service.store.PatchAgent(agentID, map[string]any{"queuedMessage": nil})
}

// StopAndRetractLast stops the agent (Escape). When its current turn produced no
// visible response yet, the user's last message is removed from the log and
// returned so the UI can drop it back into the input for editing — the "I sent
// the wrong thing" recovery. Returns "" when the agent had already responded
// (nothing is retracted), so Escape is otherwise a plain stop. For claude-code it
// also rewinds the model's own transcript so a retracted message is forgotten.
func (service *Service) StopAndRetractLast(agentID string) (string, error) {
	if service.runner == nil {
		return "", errors.New("runner not initialised")
	}
	if service.store == nil {
		return "", errors.New("store not initialised")
	}
	// Grab the live session before cancel removes it, so we can wait for its reader
	// goroutine to finish (and close the log file) before rewriting the log —
	// otherwise we'd race its final writes.
	service.runner.mu.Lock()
	cs := service.runner.claude[agentID]
	service.runner.mu.Unlock()
	_ = service.runner.cancel(agentID)

	// The retract is claude-code only: it's the one kind whose persistent reader we
	// can synchronize on (cs.done) before rewriting the log. Other kinds drain
	// asynchronously in run(), so we just stop them — never rewrite their log.
	agent, _ := service.store.GetAgent(agentID)
	if agent == nil || agent.Kind != "claude-code" {
		return "", nil
	}
	if cs != nil {
		select {
		case <-cs.done:
		case <-time.After(2 * time.Second):
		}
	}

	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	last := -1
	for i := len(lines) - 1; i >= 0; i-- {
		var ev StreamEvent
		if json.Unmarshal([]byte(lines[i]), &ev) == nil && ev.Type == "user_message" {
			last = i
			break
		}
	}
	if last < 0 {
		return "", nil
	}
	// A visible response after the message means the agent already acted on it.
	for _, ln := range lines[last+1:] {
		var ev StreamEvent
		if json.Unmarshal([]byte(ln), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "text", "thinking", "tool_call", "tool_result":
			return "", nil
		}
	}

	var msg StreamEvent
	_ = json.Unmarshal([]byte(lines[last]), &msg)
	out := ""
	if last > 0 {
		out = strings.Join(lines[:last], "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return "", err
	}

	// Also rewind claude-code's own transcript so the message is gone from the
	// model's context on the next --resume, not just from the Polaris log.
	if agent.SessionID != "" {
		project, _ := service.store.GetProject(agent.ProjectID)
		forgetClaudeMessage(agentRunDir(agent, project), agent.SessionID, msg.Content)
	}
	return msg.Content, nil
}

// buildACPEnv returns the extra environment for an ACP spawn/resume. opencode
// injects an inline provider config (custom provider) or pins the chosen
// provider/model (plain opencode); Mistral only selects the active model.
func (service *Service) buildACPEnv(kind, providerID, model string) ([]string, error) {
	switch kind {
	case "mistral":
		return buildMistralEnv(model), nil
	case "opencode":
		if providerID != "" {
			p, err := service.store.GetCustomProvider(providerID)
			if err != nil {
				return nil, fmt.Errorf("find custom provider: %w", err)
			}
			if p == nil {
				return nil, fmt.Errorf("custom provider %q not found", providerID)
			}
			return buildOpencodeEnv(p, model)
		}
		// Plain opencode: uses its own authenticated providers; Model is a
		// "provider/model" id from opencode's own list.
		return buildOpencodeBaseEnv(model)
	default:
		return nil, fmt.Errorf("agent kind %q is not ACP-based", kind)
	}
}

// resumeACP respawns the ACP backend for a finished session and resumes it via
// session/load, then runs message as the next turn. Token/cost totals carry over
// so the running counts continue from where the session left off.
func (service *Service) resumeACP(agent *Agent, message string) error {
	if agent.SessionID == "" {
		return fmt.Errorf("agent %q has no session id; cannot resume", agent.ID)
	}

	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	workDir := agentRunDir(agent, project)

	acpEnv, err := service.buildACPEnv(agent.Kind, agent.ProviderID, agent.Model)
	if err != nil {
		return err
	}
	rt, err := service.acpRuntime(agent.Kind, "")
	if err != nil {
		return err
	}

	_ = service.appendAgentEvent(agent.ID, StreamEvent{Type: "user_message", Content: message})
	_ = service.store.PatchAgent(agent.ID, map[string]any{"status": "working"})

	env := append(os.Environ(), acpEnv...)
	return service.runner.startACPSession(service, agent.ID, rt, workDir, message, env, agent.Tokens.Total(), agent.CostUSD, agent.SessionID)
}

// appendAgentLog writes a legacy text message to the agent's log file.
// Remaining callers pass pre-formatted strings; new code should call
// appendAgentEvent directly with an appropriate Type.
func (service *Service) appendAgentLog(agentID, line string) error {
	content := strings.TrimSpace(line)
	if content == "" {
		return nil
	}
	return service.appendAgentEvent(agentID, StreamEvent{Type: "text", Content: content})
}

// onTurnFinished drains the next queued message (if any) and chains a new
// turn, or marks the agent as completed when the queue is empty.
func (service *Service) onTurnFinished(agentID string) {
	next, has := service.runner.popPending(agentID)
	if !has {
		service.markAgentCompleted(agentID)
		return
	}

	agent, err := service.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}
	project, _ := service.store.GetProject(agent.ProjectID)
	workDir := agentRunDir(agent, project)
	binary, args, env, err := service.buildResume(agent, next)
	if err != nil {
		service.markAgentError(agentID, fmt.Sprintf("resume queued message: %v", err))
		return
	}
	var writeInitial func(io.Writer) error
	if agent.Kind == "claude-code" && next != "" {
		writeInitial = func(w io.Writer) error { return writeClaudeUserText(w, next) }
	}
	go service.runner.run(service, agentID, agent.Kind, binary, args, workDir, "", true, env, writeInitial)
}
