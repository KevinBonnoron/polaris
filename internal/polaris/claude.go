package polaris

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	// sink serialises writes to logFile from the concurrent stdout/stderr
	// drain goroutines so JSONL lines never interleave.
	sink *lockedWriter

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
		sink:      newLockedWriter(logFile),
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
		streamLines(stderr, c.sink, func(evt StreamEvent) {
			if isRetryableAPIError(evt.Content) {
				c.markRetryable()
			}
			c.svc.emitLogEvent(c.agentID, evt)
		})
	}()

	streamClaudeJSON(stdout, c.sink,
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
	c.svc.finishTurn(c.agentID, s)
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

// answerQuestion delivers an AskUserQuestion / ExitPlanMode answer as the next
// user turn on the live session. In --print mode claude auto-dismisses these
// interactive tools itself (an is_error tool_result it injects) and ends the
// turn, so the process is idle by the time the user answers: the answer can't be
// a tool_result for the already-closed tool — it is a fresh user message. The
// caller logs the choice separately, so this writes straight to stdin without a
// chat bubble.
func (c *claudeSession) answerQuestion(message string) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.running = true
	c.curMsg = message
	c.retryCount = 0
	c.mu.Unlock()
	if err := c.writeUser(message); err != nil {
		c.respawnResume(c.model, message)
	}
	return true
}

// supersedeQuestion handles a user who types a new message instead of answering a
// pending question. claude already auto-dismissed the tool and ended the turn, so
// the message is simply the next user turn (logged as a normal bubble).
func (c *claudeSession) supersedeQuestion(message string) {
	c.runner.consumeAwaiting(c.agentID)
	c.sendOrQueue(message)
}

// dropQueue discards the pending message on a broken chain (crash / giving up),
// logging the drop and clearing the chip.
func (c *claudeSession) dropQueue() {
	if dropped := c.runner.clearPending(c.agentID); len(dropped) > 0 {
		_ = c.svc.appendAgentEvent(c.agentID, StreamEvent{Type: "system", Content: fmt.Sprintf("(dropped %d queued message(s))", len(dropped))})
	}
	_ = c.svc.store.PatchAgent(c.agentID, map[string]any{"queuedMessage": nil})
}

// interruptForAwait aborts the in-flight turn because an AskUserQuestion /
// ExitPlanMode was surfaced. In --print mode claude auto-dismisses the
// interactive tool and the model then keeps going — running real operations
// while it "should" be waiting for the user. Interrupting stops it without
// killing the process, so the answer can still be delivered as the next turn.
// running is cleared because the aborted turn may not reach a clean result
// boundary (the interrupt can pre-empt the auto-dismiss tool_result that
// onTurnEnd waits for via askPending), and a stuck running flag would make a
// superseding message queue forever with no turn boundary to drain it.
func (c *claudeSession) interruptForAwait() {
	c.interrupt()
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
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

// --- streaming (moved from stream_claude.go) ---
var tmpWriteRe = regexp.MustCompile(`(?:>+|tee)\s+(/tmp/\S+)`)

// onResult fires once per turn boundary (each stream-json `result` event), with
// the stats accumulated so far. The persistent claude session uses it to close
// the turn (persist per-turn stats, drain the next queued message) without the
// process exiting; the one-shot path uses it to close stdin so the process ends.
func streamClaudeJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onAsk onAskUserQuestion, onTok onTokens, onResult func(streamTurnStats)) streamTurnStats {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	filesSet := make(map[string]struct{})
	toolInputs := make(map[string]toolInputSnapshot)
	var stats streamTurnStats

	askPending := map[string]struct{}{}
	// surfacedIDs holds the tool_use ids of AskUserQuestion/ExitPlanMode calls we
	// surfaced this turn. In --print mode claude auto-dismisses these itself: it
	// injects an is_error tool_result for the tool it just called and then emits
	// filler text ("the question was dismissed, let me know…"). Both are noise —
	// the question is surfaced via the panel + the tool_call line, and the real
	// answer is recorded when the user responds — so they are dropped (suppressed
	// per turn, reset at the result boundary).
	surfacedIDs := map[string]struct{}{}
	suppressTrailing := false
	activateSuppress := false
	wrappedAsk := onAsk
	if onAsk != nil {
		wrappedAsk = func(toolUseID string, input map[string]any) {
			askPending[toolUseID] = struct{}{}
			surfacedIDs[toolUseID] = struct{}{}
			activateSuppress = true
			onAsk(toolUseID, input)
		}
	}

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: raw})
			continue
		}
		kind, _ := evt["type"].(string)
		if kind == "user" {
			for _, id := range userToolResultIDs(evt) {
				delete(askPending, id)
			}
		}
		prevTokens := stats.Tokens
		for _, se := range claudeEventToStreamEvents(evt, filesSet, toolInputs, wrappedAsk, &stats) {
			if suppressTrailing && (se.Type == "text" || se.Type == "thinking") {
				continue
			}
			if se.Type == "tool_result" {
				if _, ok := surfacedIDs[se.ID]; ok {
					continue
				}
			}
			if se.Type == "tool_call" && isSchedulingTool(se.Name) {
				stats.ScheduledWakeup = true
			}
			emitEvent(sink, onEvent, se)
		}
		// Activated after the event that surfaced the question is fully emitted, so
		// the tool_call line (and its leading thinking) survive and only the
		// follow-on filler is dropped.
		if activateSuppress {
			suppressTrailing = true
			activateSuppress = false
		}
		if onTok != nil && stats.Tokens != prevTokens {
			onTok(stats.Tokens, stats.Parts, stats.CostUSD)
		}
		if kind == "result" {
			if onResult != nil && len(askPending) == 0 {
				stats.FilesModified = len(filesSet)
				onResult(stats)
				stats.ScheduledWakeup = false
			}
			suppressTrailing = false
			surfacedIDs = map[string]struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: fmt.Sprintf("stream read error: %v", err)})
	}

	stats.FilesModified = len(filesSet)
	return stats
}

// toolInputSnapshot captures a tool_use's name + input so we can render a
// richer result (e.g. diff for Edit) when the matching tool_result arrives.
type toolInputSnapshot struct {
	name  string
	input map[string]any
}

// claudeEventToStreamEvents converts one stream-json event into zero or more
// StreamEvents and updates the trackers/stats.
func claudeEventToStreamEvents(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	kind, _ := evt["type"].(string)
	switch kind {
	case "system", "init":
		return nil
	case "assistant":
		return renderClaudeAssistant(evt, files, toolInputs, onAsk, stats)
	case "user":
		return renderClaudeUserToolResults(evt, toolInputs)
	case "result":
		applyResultUsage(evt, stats)
		isErr, _ := evt["is_error"].(bool)
		status, _ := evt["status"].(string)
		stats.Succeeded = !isErr && status != "error"
		sub, _ := evt["subtype"].(string)
		if sub == "" {
			if isErr || status == "error" {
				sub = "error"
			} else {
				sub = "success"
			}
		}
		return []StreamEvent{{
			Type:    "turn_end",
			Status:  sub,
			Tokens:  stats.Tokens,
			CostUSD: stats.CostUSD,
			Parts:   &stats.Parts,
		}}
	default:
		return nil
	}
}

func renderClaudeAssistant(evt map[string]any, files map[string]struct{}, toolInputs map[string]toolInputSnapshot, onAsk onAskUserQuestion, stats *streamTurnStats) []StreamEvent {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	applyUsage(msg["usage"], stats)
	content, _ := msg["content"].([]any)
	events := make([]StreamEvent, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		btype, _ := block["type"].(string)
		switch btype {
		case "text":
			text, _ := block["text"].(string)
			if t := strings.TrimSpace(text); t != "" {
				events = append(events, StreamEvent{Type: "text", Content: t})
			}
		case "thinking":
			thinking, _ := block["thinking"].(string)
			if t := strings.TrimSpace(thinking); t != "" {
				events = append(events, StreamEvent{Type: "thinking", Content: t})
			}
		case "tool_use":
			rawName, _ := block["name"].(string)
			name := claudeToolName(rawName)
			input, _ := block["input"].(map[string]any)
			id, _ := block["id"].(string)
			stats.ToolsUsed++
			if fp, ok := extractFilePath(name, input); ok {
				files[fp] = struct{}{}
			}
			if id != "" {
				toolInputs[id] = toolInputSnapshot{name: name, input: input}
			}
			if name == "AskUserQuestion" && id != "" && onAsk != nil {
				onAsk(id, input)
			}
			if name == "ExitPlanMode" && id != "" && onAsk != nil {
				onAsk(id, exitPlanModeQuestion(input, toolInputs))
			}
			se := StreamEvent{
				Type:  "tool_call",
				ID:    id,
				Name:  name,
				Input: input,
			}
			// Embed a summary detail in Content so the frontend doesn't need
			// to re-derive it from Input for the compact display.
			if detail := summarizeToolInput(name, input); detail != "" {
				se.Content = strings.TrimPrefix(detail, " · ")
			}
			events = append(events, se)
		}
	}
	return events
}

// isSchedulingTool reports whether a tool call indicates the agent intends to
// resume later via an external trigger (scheduled wakeup, remote routine, etc.).
func isSchedulingTool(name string) bool {
	switch name {
	case "ScheduleWakeup", "CronCreate":
		return true
	}
	return false
}

func claudeToolName(name string) string {
	switch name {
	case "ReadImage", "ViewImage":
		return "Read"
	default:
		return name
	}
}

func renderClaudeUserToolResults(evt map[string]any, toolInputs map[string]toolInputSnapshot) []StreamEvent {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	events := make([]StreamEvent, 0, len(content))
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		if t, _ := block["type"].(string); t != "tool_result" {
			continue
		}
		body := toolResultText(block["content"])
		isErr, _ := block["is_error"].(bool)
		id, _ := block["tool_use_id"].(string)
		se := StreamEvent{
			Type:  "tool_result",
			ID:    id,
			Error: isErr,
		}
		if !isErr {
			if snap, ok := toolInputs[id]; ok {
				se.RenderedContent = renderToolSnapshot(snap.name, snap.input)
			}
		}
		se.Content = truncate(body, 4000)
		events = append(events, se)
	}
	return events
}

func userToolResultIDs(evt map[string]any) []string {
	msg, _ := evt["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	var ids []string
	for _, item := range content {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		if t, _ := block["type"].(string); t != "tool_result" {
			continue
		}
		if id, _ := block["tool_use_id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func applyUsage(raw any, stats *streamTurnStats) {
	if p, ok := usageBreakdown(raw); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

func applyResultUsage(evt map[string]any, stats *streamTurnStats) {
	if cost, ok := numericField(evt, "total_cost_usd"); ok {
		stats.CostUSD = cost
	}
	raw := evt["usage"]
	if raw == nil {
		raw = evt["stats"]
	}
	if p, ok := usageBreakdown(raw); ok {
		stats.Parts = p
		stats.Tokens = p.Total()
	}
}

func usageBreakdown(raw any) (usageParts, bool) {
	usage, _ := raw.(map[string]any)
	if usage == nil {
		return usageParts{}, false
	}
	get := func(k string) int {
		if v, ok := numericField(usage, k); ok {
			return int(v)
		}
		return 0
	}
	res := usageParts{
		Input:         get("input_tokens"),
		Output:        get("output_tokens"),
		CacheCreation: get("cache_creation_input_tokens"),
		CacheRead:     get("cache_read_input_tokens"),
	}
	if res.CacheRead == 0 {
		res.CacheRead = get("cached")
	}
	return res, true
}

// exitPlanModeQuestion turns an ExitPlanMode tool_use into an AskUserQuestion
// payload so the user can approve/reject/revise the plan.
func exitPlanModeQuestion(input map[string]any, toolInputs map[string]toolInputSnapshot) map[string]any {
	plan, _ := input["plan"].(string)
	plan = strings.TrimSpace(plan)
	if strings.HasPrefix(plan, "/") {
		if content, err := os.ReadFile(plan); err == nil {
			plan = strings.TrimSpace(string(content))
		}
	}
	if plan == "" {
		if path := tmpPlanFile(toolInputs); path != "" {
			if content, err := os.ReadFile(path); err == nil {
				plan = strings.TrimSpace(string(content))
			}
		}
	}
	if plan == "" {
		plan = writtenPlanContent(toolInputs)
	}
	if plan == "" {
		plan = "The agent finished planning and wants to proceed."
	}
	return map[string]any{
		"questions": []map[string]any{{
			"header":   "Plan",
			"question": plan,
			"options": []map[string]any{
				{"label": "Approve & proceed"},
				{"label": "Reject"},
			},
		}},
	}
}

func tmpPlanFile(toolInputs map[string]toolInputSnapshot) string {
	for _, snap := range toolInputs {
		if snap.name != "Bash" {
			continue
		}
		cmd, _ := snap.input["command"].(string)
		if m := tmpWriteRe.FindStringSubmatch(cmd); len(m) > 1 {
			return strings.TrimRight(m[1], "\"';")
		}
	}
	return ""
}

func writtenPlanContent(toolInputs map[string]toolInputSnapshot) string {
	var fallback string
	for _, snap := range toolInputs {
		if snap.name != "Write" {
			continue
		}
		fp, _ := snap.input["file_path"].(string)
		content, _ := snap.input["content"].(string)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if strings.Contains(strings.ToLower(fp), "plan") {
			return content
		}
		fallback = content
	}
	return fallback
}

// --- claude helpers (moved from runner.go) ---
// relocateClaudeSession moves a Claude Code session transcript from the source
// working directory's project slot to the destination's, so --resume works
// after the agent's CWD changes (e.g. on worktree promotion). Best-effort.
func relocateClaudeSession(srcDir, dstDir, sessionID string) {
	src := filepath.Join(claudeProjectDir(srcDir), sessionID+".jsonl")
	dst := filepath.Join(claudeProjectDir(dstDir), sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	_ = os.Rename(src, dst)
}

// forgetClaudeMessage rewinds Claude Code's own session transcript so a retracted
// message and its aborted turn are gone from the model's context on the next
// --resume, not just from the Polaris log. It removes everything from the last
// user line whose text matches onward — a clean suffix trim that keeps the
// parentUuid chain intact. Best-effort: the claude session process must already
// be stopped (no concurrent writes), and a no-match leaves the file untouched.
func forgetClaudeMessage(workDir, sessionID, text string) {
	dir := claudeProjectDir(workDir)
	if dir == "" || sessionID == "" {
		return
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	want := normalizeClaudeText(text)
	if want == "" {
		return
	}
	cut := -1
	for i := len(lines) - 1; i >= 0; i-- {
		var l struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(lines[i]), &l) != nil {
			continue
		}
		if l.Type != "user" || l.Message.Role != "user" {
			continue
		}
		if normalizeClaudeText(claudeContentText(l.Message.Content)) == want {
			cut = i
			break
		}
	}
	if cut < 0 {
		return
	}
	out := ""
	if cut > 0 {
		out = strings.Join(lines[:cut], "\n") + "\n"
	}
	_ = os.WriteFile(path, []byte(out), 0o644)
}

// claudeContentText flattens a session message's content (a JSON string, or an
// array of content blocks) down to its plain text for matching.
func claudeContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// normalizeClaudeText strips the invisible leading marker escapeLeadingSlash adds
// and surrounding whitespace, so the Polaris-logged text matches what Claude
// recorded for the same message.
func normalizeClaudeText(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(s, "\u200b"))
}

// writeClaudeUserText sends a plain user message as a stream-json event on
// claude's stdin.
func writeClaudeUserText(w io.Writer, text string) error {
	evt := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": escapeLeadingSlash(text),
		},
	}
	return writeJSONLine(w, evt)
}

// escapeLeadingSlash neutralises a leading "/" so claude-code does not treat
// the message as one of its own slash commands (unknown ones are rejected with
// no turn). Slash commands belong to Polaris and are handled before sending, so
// any "/" that reaches the CLI is meant as literal text. A zero-width space is
// prepended: it is not Unicode whitespace, so it survives the CLI's trimming
// while staying invisible to the model.
func escapeLeadingSlash(text string) string {
	if strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), "/") {
		return string(rune(0x200b)) + text
	}
	return text
}

// writeClaudeToolResult answers a tool_use mid-turn with a tool_result event.
// Used by AskUserQuestion: the user's selection is delivered as the proper
// tool_result so Claude sees it as the tool's return value, not a new message.
func writeClaudeToolResult(w io.Writer, toolUseID, content string, isError bool) error {
	evt := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": toolUseID,
					"content":     content,
					"is_error":    isError,
				},
			},
		},
	}
	return writeJSONLine(w, evt)
}

func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// --- command building (moved from runner.go) ---

func claudeSpawnCommand(binary, model, sessionID string, allowedTools []string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "claude"
	}
	// --print has no TTY so any permission prompt would stall; always bypass.
	// stream-json both ways gives a full-duplex JSON channel (events out, the
	// task and each follow-up delivered as a new user turn on stdin).
	args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "bypassPermissions"}
	if sessionID != "" {
		args = append(args, "--session-id", sessionID)
	}
	if model != "" && model != "auto" {
		args = append(args, "--model", model)
	}
	appendToolArgs(&args, allowedTools)
	return bin, args, nil
}

func claudeResumeCommand(binary, sessionID, model string, allowedTools []string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "claude"
	}
	args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "bypassPermissions", "--resume", sessionID}
	// Pin the model on resume so a fallback/user switch is honoured rather than
	// defaulting to whatever the session was created with.
	if model != "" && model != "auto" {
		args = append(args, "--model", model)
	}
	appendToolArgs(&args, allowedTools)
	return bin, args, nil
}

// appendToolArgs adds --tools/--allowed-tools from AllowedTools. The sentinel
// ["__no_tools__"] disables all tools via --tools ""; a named list uses
// --allowed-tools.
func appendToolArgs(args *[]string, tools []string) {
	if len(tools) == 0 {
		return
	}
	if len(tools) == 1 && tools[0] == "__no_tools__" {
		*args = append(*args, "--tools", "")
		return
	}
	*args = append(*args, "--allowed-tools", strings.Join(tools, ","))
}
