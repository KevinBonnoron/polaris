package polaris

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

// claudeSession drives claude-code as a long-lived process, mirroring acpSession
// for ACP backends. claude --print --input-format stream-json keeps the CLI
// alive between turns (a turn boundary is a `result` event, not process exit),
// so a follow-up is a new user message on the same stdin instead of a fresh
// --resume subprocess, and an in-flight turn can be aborted with a stream-json
// control_request/interrupt. This is what lets a message sent mid-turn be taken
// into account immediately rather than only at the next turn.
type claudeSession struct {
	svc     *Service
	runner  *Runner
	agentID string
	workDir string
	logFile *os.File

	mu      sync.Mutex
	writeMu sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	closed  bool
	running bool
	seq     int
	// done is closed once readLoop has fully drained the pipes and closed the log
	// file, so a caller that rewrites the log (StopAndRetractLast) can wait for the
	// writer to be gone instead of racing it.
	done chan struct{}

	// curMsg is the message the in-flight turn is answering, retained so an
	// overloaded turn can be re-sent (same model) or replayed after a
	// model-fallback respawn.
	curMsg     string
	model      string
	retryCount int
	retrySeen  bool

	// baseParts/baseCost are the agent's totals before this turn; baseParts
	// grows as each turn completes so the live counter reflects the cumulative
	// usage. lastCost/lastTools track the last result's cumulative figures so a
	// per-turn delta can be persisted (total_cost_usd is cumulative, the parser's
	// tool counter accumulates, but the usage block is per-turn).
	baseParts usageParts
	baseCost  float64
	lastCost  float64
	lastTools int
}

// startClaudeSession spawns claude-code as a persistent session and writes the
// first user message. stdinMsg is the text delivered as the first turn; startMsg,
// when non-empty, is also surfaced as a user_message log bubble. appendLog=false
// truncates the log (fresh spawn); resume builds a --resume command.
func (runner *Runner) startClaudeSession(svc *Service, agent *Agent, workDir, stdinMsg, startMsg string, appendLog, resume bool) error {
	binary, args, err := claudeSessionCommand(agent, resume)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runner.logsRoot, 0o755); err != nil {
		return fmt.Errorf("logs dir: %w", err)
	}
	logPath := filepath.Join(runner.logsRoot, agent.ID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	if !appendLog {
		_ = logFile.Truncate(0)
	}
	if startMsg != "" {
		evt := StreamEvent{Type: "user_message", Content: startMsg}
		emitEvent(logFile, nil, evt)
		svc.emitLogEvent(agent.ID, evt)
	}

	cmd := exec.Command(binary, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	env := os.Environ()
	if os.Getenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS") == "" {
		env = append(env, "CLAUDE_CODE_MAX_OUTPUT_TOKENS=64000")
	}
	cmd.Env = env
	sysexec.Hide(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logFile.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		_ = logFile.Close()
		return fmt.Errorf("stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = logFile.Close()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		_ = stdin.Close()
		_ = logFile.Close()
		return fmt.Errorf("start: %w", err)
	}

	c := &claudeSession{
		svc:       svc,
		runner:    runner,
		agentID:   agent.ID,
		workDir:   workDir,
		logFile:   logFile,
		cmd:       cmd,
		stdin:     stdin,
		running:   true,
		curMsg:    stdinMsg,
		model:     agent.Model,
		baseParts: agent.Tokens,
		baseCost:  agent.CostUSD,
		done:      make(chan struct{}),
	}

	runner.mu.Lock()
	runner.claude[agent.ID] = c
	runner.mu.Unlock()
	_ = svc.store.PatchAgent(agent.ID, map[string]any{"pid": cmd.Process.Pid})

	go c.readLoop(stdout, stderr)

	if err := c.writeUser(stdinMsg); err != nil {
		c.respawnResume(c.model, stdinMsg)
	}
	return nil
}

// claudeSessionCommand builds the spawn or resume command for the persistent
// session, reusing the same arg builders as the one-shot path.
func claudeSessionCommand(agent *Agent, resume bool) (string, []string, error) {
	if resume {
		return buildResumeCommand(agent.Kind, "", agent.SessionID, "", agent.Source, agent.Model, agent.AllowedTools)
	}
	return buildSpawnCommand(agent.Kind, "", agent.Model, agent.SessionID, "", agent.Source, agent.AllowedTools)
}

func (c *claudeSession) readLoop(stdout, stderr io.Reader) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		streamLines(stderr, c.logFile, func(evt StreamEvent) {
			if isRetryableAPIError(evt.Content) {
				c.markRetryable()
			}
			c.svc.emitLogEvent(c.agentID, evt)
		})
	}()

	streamClaudeJSON(stdout, c.logFile,
		func(evt StreamEvent) {
			if isRetryableAPIError(evt.Content) {
				c.markRetryable()
			}
			c.svc.emitLogEvent(c.agentID, evt)
		},
		func(toolUseID string, input map[string]any) {
			c.svc.emitAskUserQuestion(c.agentID, toolUseID, input)
		},
		func(tokens int, parts usageParts, costUSD float64) {
			c.mu.Lock()
			base := c.baseParts
			baseCost := c.baseCost
			c.mu.Unlock()
			c.svc.emitTokens(c.agentID, base.Total()+tokens, base.Add(parts), baseCost+costUSD)
		},
		func(s streamTurnStats) {
			c.onTurnEnd(s)
		},
	)

	wg.Wait()
	c.onProcessExit()
	close(c.done)
}

// onTurnEnd runs at each turn boundary: it persists the turn's stats, retries an
// overloaded turn, then either delivers the next queued message on the same
// process or marks the agent completed — without the process exiting.
func (c *claudeSession) onTurnEnd(s streamTurnStats) {
	c.mu.Lock()
	costDelta := s.CostUSD - c.lastCost
	if costDelta < 0 {
		costDelta = 0
	}
	c.lastCost = s.CostUSD
	toolsDelta := s.ToolsUsed - c.lastTools
	if toolsDelta < 0 {
		toolsDelta = 0
	}
	c.lastTools = s.ToolsUsed
	turnParts := s.Parts
	// Only baseParts grows: the usage block is per-turn, so the live counter needs
	// the running sum. baseCost is deliberately left alone — total_cost_usd is
	// already cumulative, so the live emit (baseCost+costUSD) would double-count if
	// we folded the delta in here.
	c.baseParts = c.baseParts.Add(turnParts)
	retrySeen := c.retrySeen
	c.retrySeen = false
	c.mu.Unlock()

	c.svc.persistTurnStats(c.agentID, streamTurnStats{
		Tokens:    turnParts.Total(),
		Parts:     turnParts,
		CostUSD:   costDelta,
		ToolsUsed: toolsDelta,
		Succeeded: s.Succeeded,
	})

	if retrySeen && !s.Succeeded {
		c.retryOverload()
		return
	}

	c.mu.Lock()
	c.retryCount = 0
	c.mu.Unlock()

	if next, has := c.runner.popPending(c.agentID); has {
		// The message was held off the log while queued; record it here, at the
		// boundary where the turn actually picks it up, and clear the chip.
		_ = c.svc.appendAgentEvent(c.agentID, StreamEvent{Type: "user_message", Content: next})
		_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": nil})
		c.mu.Lock()
		c.curMsg = next
		c.running = true
		c.mu.Unlock()
		if err := c.writeUser(next); err != nil {
			c.respawnResume(c.model, next)
		}
		return
	}

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	c.svc.markAgentCompleted(c.agentID)
}

// retryOverload re-runs a turn that hit a transient upstream error, escalating to
// a fallback model after a couple of attempts (Opus→Sonnet), mirroring run().
func (c *claudeSession) retryOverload() {
	c.mu.Lock()
	if c.retryCount >= maxAPIRetries {
		c.retryCount = 0
		c.running = false
		c.mu.Unlock()
		c.svc.markAgentError(c.agentID, fmt.Sprintf("upstream API unavailable after %d attempts (overloaded / rate limited); giving up", maxAPIRetries+1))
		c.dropQueue()
		return
	}
	c.retryCount++
	attempt := c.retryCount
	msg := c.curMsg
	model := c.model
	c.mu.Unlock()

	fallback := fallbackClaudeModel(model)
	useFallback := fallback != "" && attempt > overloadRetriesBeforeFallback
	delay := retryBackoff(attempt)
	c.logSystem(fmt.Sprintf("⟳ upstream API overloaded — retrying in %s (attempt %d/%d, model %s)", delay, attempt, maxAPIRetries, strDefault(model, "auto")))
	time.Sleep(delay)

	if useFallback {
		c.logSystem(fmt.Sprintf("↘ still overloaded — falling back to %q", fallback))
		_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"model": fallback})
		c.respawnResume(fallback, msg)
		return
	}
	if err := c.writeUser(msg); err != nil {
		c.respawnResume(model, msg)
	}
}

// onProcessExit handles the CLI exiting. A deliberate shutdown (cancel) or a
// model-fallback handoff sets closed and is ignored; any other exit is an
// unexpected crash, surfaced as an error with queued messages dropped.
func (c *claudeSession) onProcessExit() {
	_ = c.cmd.Wait()
	// Remove ourselves from the map only if we're still the registered session: a
	// respawn handoff has already swapped in a replacement, in which case we must
	// not touch the map or reset the pid (that would clobber the new process).
	c.runner.mu.Lock()
	hasReplacement := c.runner.claude[c.agentID] != nil && c.runner.claude[c.agentID] != c
	if c.runner.claude[c.agentID] == c {
		delete(c.runner.claude, c.agentID)
	}
	c.runner.mu.Unlock()

	_ = c.logFile.Close()
	if !hasReplacement {
		_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"pid": 0})
	}

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	c.svc.markAgentError(c.agentID, "claude-code exited unexpectedly")
	c.dropQueue()
}

// sendOrQueue is the normal follow-up path. When a turn is in flight the message
// is queued (single replaceable slot, surfaced as a pending chip and moved into
// the log only when the next turn picks it up); otherwise it is written
// immediately and logged now.
func (c *claudeSession) sendOrQueue(message string) {
	c.mu.Lock()
	running := c.running
	if !running {
		c.running = true
		c.curMsg = message
		c.retryCount = 0
	}
	c.mu.Unlock()
	if running {
		c.queue(message)
		return
	}
	_ = c.svc.appendAgentEvent(c.agentID, StreamEvent{Type: "user_message", Content: message})
	if err := c.writeUser(message); err != nil {
		c.respawnResume(c.model, message)
	}
}

// queue stores message in the single replaceable pending slot and surfaces it as
// a chip on the agent record. The log entry is written later, when the turn
// boundary actually picks it up (see the drain in onTurnEnd), so the transcript
// shows the message where it was taken into account.
func (c *claudeSession) queue(message string) {
	c.runner.setPending(c.agentID, message)
	_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": message})
}

// interruptAndSend aborts the in-flight turn and applies message immediately: the
// message is queued first, then the interrupt makes the current turn end with a
// result, which drains the queued message as the next turn. When no turn is in
// flight it is an ordinary prompt.
func (c *claudeSession) interruptAndSend(message string) {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if !running {
		c.sendOrQueue(message)
		return
	}
	c.queue(message)
	c.interrupt()
}

// answerQuestion delivers an AskUserQuestion / ExitPlanMode answer as a proper
// tool_result on the live stdin, so the turn continues in place (no resume).
func (c *claudeSession) answerQuestion(toolUseID, answer string) bool {
	if err := c.writeToolResult(toolUseID, answer, false); err != nil {
		return false
	}
	return true
}

// supersedeQuestion handles a user who types a new message instead of answering
// a pending question: the question is closed with a dismissal tool_result (which
// unblocks the turn and lets it reach a turn boundary), and message is queued so
// it runs as the next turn.
func (c *claudeSession) supersedeQuestion(toolUseID, message string) {
	c.runner.consumeAwaiting(c.agentID)
	c.queue(message)
	if err := c.writeToolResult(toolUseID, "The user dismissed this prompt and will respond with a new message.", false); err != nil {
		// The respawn replays message directly as its first turn, so drop the
		// queued copy first — otherwise the next turn boundary drains it again and
		// sends it twice.
		c.runner.clearPending(c.agentID)
		_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": nil})
		c.respawnResume(c.model, message)
	}
}

// dropQueue discards the pending message on a broken chain (crash / giving up),
// logging the drop and clearing the chip.
func (c *claudeSession) dropQueue() {
	if dropped := c.runner.clearPending(c.agentID); len(dropped) > 0 {
		_ = c.svc.appendAgentEvent(c.agentID, StreamEvent{Type: "system", Content: fmt.Sprintf("(dropped %d queued message(s))", len(dropped))})
	}
	_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": nil})
}

func (c *claudeSession) interrupt() {
	c.mu.Lock()
	c.seq++
	reqID := fmt.Sprintf("int-%d", c.seq)
	c.mu.Unlock()
	_ = c.write(map[string]any{
		"type":       "control_request",
		"request_id": reqID,
		"request":    map[string]any{"subtype": "interrupt"},
	})
}

func (c *claudeSession) writeUser(text string) error {
	return c.write(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": escapeLeadingSlash(text),
		},
	})
}

func (c *claudeSession) writeToolResult(toolUseID, content string, isErr bool) error {
	return c.write(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": toolUseID,
				"content":     content,
				"is_error":    isErr,
			}},
		},
	})
}

func (c *claudeSession) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return errors.New("session closed")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

func (c *claudeSession) markRetryable() {
	c.mu.Lock()
	c.retrySeen = true
	c.mu.Unlock()
}

func (c *claudeSession) logSystem(text string) {
	_ = c.svc.appendAgentEvent(c.agentID, StreamEvent{Type: "system", Content: text})
}

// shutdown kills the session on user cancel, dropping queued messages.
func (c *claudeSession) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.runner.mu.Lock()
	if c.runner.claude[c.agentID] == c {
		delete(c.runner.claude, c.agentID)
	}
	c.runner.mu.Unlock()
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.runner.clearPending(c.agentID)
	_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": nil})
}

// respawnResume kills the current process and starts a fresh persistent session
// resumed from the same session id, replaying msg. Used for a model fallback or
// after a broken stdin pipe. The old readLoop sees closed and stands down,
// leaving the new session as the live one.
func (c *claudeSession) respawnResume(model, msg string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	agent, err := c.svc.store.GetAgent(c.agentID)
	if err != nil || agent == nil {
		return
	}
	agent.Model = model
	_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"status": "working"})
	if err := c.runner.startClaudeSession(c.svc, agent, c.workDir, msg, "", true, true); err != nil {
		c.svc.markAgentError(c.agentID, fmt.Sprintf("claude-code resume: %v", err))
	}
}
