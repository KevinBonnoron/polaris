package polaris

import (
	"bufio"
	"bytes"
	"crypto/rand"
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
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/providers/repository"
	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type SpawnAgentInput struct {
	ProjectID string `json:"projectId"`
	// ID, when set, is reused as the agent's id so a draft already in the UI is
	// promoted to a running agent in place (no new row, no flicker). Empty lets the
	// store assign one (automations and other non-draft spawns).
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind"`
	Task   string `json:"task"`
	Model  string `json:"model,omitempty"`
	Binary string `json:"binary,omitempty"`
	// ProviderID, when set, routes the spawn through the opencode harness
	// against the referenced custom provider (Kind is forced to "opencode").
	ProviderID string `json:"providerId,omitempty"`
	// Source distinguishes user-initiated spawns from automation-driven ones
	// so completion/error notifications can be suppressed for the former.
	// Empty defaults to "manual".
	Source string `json:"source,omitempty"`
	// IssueKey/IssueSummary/IssueType, when set, opt the spawn into the
	// isolated-worktree workflow: a fresh branch (prefix derived from type)
	// is created off project HEAD and the agent runs inside that worktree.
	IssueKey     string `json:"issueKey,omitempty"`
	IssueSummary string `json:"issueSummary,omitempty"`
	IssueType    string `json:"issueType,omitempty"`
	// Isolated opts the spawn into the worktree workflow even without a ticket
	// ticket. When true (manual spawn with project IsolatedDefault on, or
	// explicit override), a fresh branch is created off project HEAD with
	// BranchName if provided, else `{project.BranchPrefix}{slug(task)}-{id}`.
	Isolated bool `json:"isolated,omitempty"`
	// BranchName forces the worktree branch name when Isolated is true. Used by
	// callers that know the desired name (e.g. automation builders with custom
	// prefixes). Empty triggers auto-generation.
	BranchName string `json:"branchName,omitempty"`
	// AllowedTools restricts which tools the claude-code CLI exposes to the
	// model via --allowed-tools. Empty means all tools (no restriction).
	// Ignored for non-claude-code kinds.
	AllowedTools []string `json:"allowedTools,omitempty"`
}

// runningProc holds the live subprocess plus the stdin handle used to answer
// interactive prompts. stdin is nil for kinds that don't accept follow-up
// input on stdin (claude-code uses --resume to start a new turn instead).
type runningProc struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

type Runner struct {
	mu            sync.Mutex
	procs         map[string]*runningProc
	acp           map[string]*acpSession
	claude        map[string]*claudeSession
	pending       map[string][]string
	awaiting      map[string]bool
	logsRoot      string
	worktreesRoot string
}

func NewRunner(logsRoot, worktreesRoot string) *Runner {
	return &Runner{
		procs:         make(map[string]*runningProc),
		acp:           make(map[string]*acpSession),
		claude:        make(map[string]*claudeSession),
		pending:       make(map[string][]string),
		awaiting:      make(map[string]bool),
		logsRoot:      logsRoot,
		worktreesRoot: worktreesRoot,
	}
}

// beginAwait marks the agent as awaiting a user answer to a surfaced
// question/plan. It returns false if the agent is already awaiting one — which
// means claude is re-emitting the same question in a loop (claude-code --print
// can't get an interactive ExitPlanMode approval and retries endlessly), and the
// caller should ignore the duplicate.
func (runner *Runner) beginAwait(agentID string) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.awaiting[agentID] {
		return false
	}
	runner.awaiting[agentID] = true
	return true
}

// consumeAwaiting reports (and clears) whether the agent's subprocess was
// stopped on purpose to wait for a user answer, so run() can finalise it as
// "waiting" instead of treating the kill as a crash.
func (runner *Runner) consumeAwaiting(agentID string) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.awaiting[agentID] {
		delete(runner.awaiting, agentID)
		return true
	}
	return false
}

// stopForAwait halts the agent's work once a question/plan has been surfaced, so
// the backend can't keep running operations on an unanswerable tool call. A
// persistent claude session is interrupted (not killed) so the answer can be
// delivered as the next turn on the live process; a one-shot subprocess is
// killed and its answer delivered later via a resume.
func (runner *Runner) stopForAwait(agentID string) {
	runner.mu.Lock()
	cs := runner.claude[agentID]
	proc, ok := runner.procs[agentID]
	runner.mu.Unlock()
	if cs != nil {
		cs.interruptForAwait()
		return
	}
	if !ok || proc.cmd.Process == nil {
		return
	}
	if proc.stdin != nil {
		_ = proc.stdin.Close()
	}
	_ = proc.cmd.Process.Kill()
}

func (service *Service) WithRunner(r *Runner) *Service {
	service.runner = r
	return service
}

// agentLogEvent is the Wails event name carrying a new log line for a given
// agent. Subscribers should filter by `agentId`.
const agentLogEvent = "agent:log:appended"

// AgentLogEventName returns the event name the frontend should subscribe to for
// live log updates. Exported so the binding stays in one place.
func AgentLogEventName() string { return agentLogEvent }

// CreatePRForAgent pushes the agent's branch and opens a PR via `gh`. The
// agent must have been spawned in worktree mode (Branch + WorktreePath
// populated) and the worktree must contain at least one commit ahead of its
// upstream — otherwise we'd be asking GitHub to open an empty PR.
func (service *Service) CreatePRForAgent(agentID string) (string, error) {
	if service.store == nil {
		return "", errors.New("store not initialised")
	}
	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent %q not found", agentID)
	}
	if agent.Worktree.PRURL != "" {
		return agent.Worktree.PRURL, nil
	}
	if agent.Worktree.Branch == "" || agent.Worktree.Path == "" {
		return "", errors.New("agent has no branch/worktree to push")
	}
	if info, statErr := os.Stat(agent.Worktree.Path); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("worktree %q no longer on disk", agent.Worktree.Path)
	}
	hasCommits, err := git.HasCommitsAhead(agent.Worktree.Path)
	if err != nil {
		return "", fmt.Errorf("check commits: %w", err)
	}
	if !hasCommits {
		return "", errors.New("no commits to push yet")
	}
	if err := git.PushBranch(agent.Worktree.Path, agent.Worktree.Branch); err != nil {
		return "", fmt.Errorf("push branch: %w", err)
	}
	title, body := agentPRMessage(agent)
	url, err := repository.CreatePullRequest(agent.Worktree.Path, title, body)
	if err != nil {
		return "", err
	}
	agent.Worktree.PRURL = url
	if err := service.store.PatchAgent(agent.ID, map[string]any{"worktree": agent.Worktree}); err != nil {
		// PR is already open; surface the error but return the URL so the UI
		// can still link to it.
		return url, fmt.Errorf("persist pr url: %w", err)
	}
	return url, nil
}

// agentPRMessage builds the title and body for `gh pr create` from the agent
// record. Title prefers the ticket key + summary; falls back to the task line
// when the spawn wasn't ticket-driven.
func agentPRMessage(agent *Agent) (title, body string) {
	summary := strings.TrimSpace(agent.Summary)
	if agent.Worktree.IssueKey != "" {
		head := summary
		if idx := strings.IndexAny(head, "\r\n"); idx >= 0 {
			head = head[:idx]
		}
		head = strings.TrimSpace(head)
		if utf8.RuneCountInString(head) > 80 {
			head = truncateRunes(head, 77) + "..."
		}
		if head == "" {
			title = agent.Worktree.IssueKey
		} else {
			title = agent.Worktree.IssueKey + " · " + head
		}
	} else {
		title = summary
		if idx := strings.IndexAny(title, "\r\n"); idx >= 0 {
			title = title[:idx]
		}
		if utf8.RuneCountInString(title) > 80 {
			title = truncateRunes(title, 77) + "..."
		}
	}
	if title == "" {
		title = "Polaris agent · " + agent.Kind
	}
	body = summary
	if body == "" {
		body = "Opened by Polaris."
	}
	return title, body
}

// RemoveAgentWorktree drops the git worktree backing an agent, if any. Safe
// to call multiple times: missing paths or git metadata are not errors.
func (service *Service) RemoveAgentWorktree(agent *Agent) {
	if agent == nil || agent.Worktree.Path == "" {
		return
	}
	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil || project == nil || project.Path == "" {
		// No way to call `git worktree remove` from the project root, fall
		// back to a plain rm so we don't leak the directory.
		_ = os.RemoveAll(agent.Worktree.Path)
		return
	}
	_ = git.RemoveWorktree(project.Path, agent.Worktree.Path)
}

func (service *Service) Cancel(agentID string) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	return service.runner.cancel(agentID)
}

// ResetAll cancels every running agent, wipes the database, and deletes every
// log file under the runner's logs root. Used by the UI "delete all data"
// action.
func (service *Service) ResetAll() error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	if service.runner != nil {
		service.runner.cancelAll()
	}
	// Collect worktrees before the DB is wiped so we still know what to drop.
	var worktrees []*Agent
	if agents, err := service.store.ListAgents(""); err == nil {
		for i := range agents {
			if agents[i].Worktree.Path != "" {
				worktrees = append(worktrees, &agents[i])
			}
		}
	}
	if err := service.store.ResetAll(); err != nil {
		return err
	}
	for _, a := range worktrees {
		service.RemoveAgentWorktree(a)
	}
	if service.runner != nil {
		if err := service.runner.wipeLogs(); err != nil {
			return fmt.Errorf("wipe logs: %w", err)
		}
		if service.runner.worktreesRoot != "" {
			_ = os.RemoveAll(service.runner.worktreesRoot)
		}
	}
	return nil
}

func (service *Service) ReadLog(agentID string) (string, error) {
	if service.runner == nil {
		return "", errors.New("runner not initialised")
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// ClearLog truncates an agent's log file to empty. A missing file is not an
// error. The agent record's transcript lives entirely in this file, so this is
// the real "clear history" for a conversation.
func (service *Service) ClearLog(agentID string) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	if err := os.Truncate(path, 0); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func (service *Service) deleteAgentLog(agentID string) {
	if service.runner == nil || service.runner.logsRoot == "" {
		return
	}
	_ = os.Remove(filepath.Join(service.runner.logsRoot, agentID+".log"))
}

// ReadLogTail returns the last n non-empty lines of an agent's log file. n must
// be positive; for n <= 0 an empty slice is returned. Missing file is not an
// error — the agent may not have flushed any output yet.
func (service *Service) ReadLogTail(agentID string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	if service.runner == nil {
		return nil, errors.New("runner not initialised")
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	buf := make([]string, 0, n)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(buf) == n {
			buf = buf[1:]
		}
		buf = append(buf, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buf, nil
}

// buildSpawnCommand dispatches to the per-CLI argv builder (see claude.go,
// codex.go, gemini.go, cursor.go, copilot.go).
func buildSpawnCommand(kind, binary, model, sessionID, task, source string, allowedTools []string) (string, []string, error) {
	switch kind {
	case "claude-code":
		return claudeSpawnCommand(binary, model, sessionID, allowedTools)
	case "codex":
		return codexSpawnCommand(binary, model, task)
	case "copilot":
		return copilotSpawnCommand(binary, task)
	case "gemini":
		return geminiSpawnCommand(binary, model, task)
	case "cursor":
		return cursorSpawnCommand(binary, model, task)
	default:
		return "", nil, fmt.Errorf("unknown agent kind %q", kind)
	}
}

// buildResumeCommand dispatches to the per-CLI resume argv builder.
func buildResumeCommand(kind, binary, sessionID, message, source, model string, allowedTools []string) (string, []string, error) {
	switch kind {
	case "claude-code":
		return claudeResumeCommand(binary, sessionID, model, allowedTools)
	case "cursor":
		return cursorResumeCommand(binary, model, message)
	case "codex":
		return codexResumeCommand(binary, sessionID, model, message)
	case "gemini":
		return geminiResumeCommand(binary, model, message)
	default:
		return "", nil, fmt.Errorf("resume not supported for agent kind %q", kind)
	}
}

// buildResume produces the command + extra env for a follow-up turn. opencode
// runs over a persistent ACP session (handled in Send), so only claude reaches
// here; the env return is always nil.
func (service *Service) buildResume(agent *Agent, message string) (string, []string, []string, error) {
	binary, args, err := buildResumeCommand(agent.Kind, "", agent.SessionID, message, agent.Source, agent.Model, agent.AllowedTools)
	return binary, args, nil, err
}

func needsStdinPipe(kind string, writeInitial func(io.Writer) error) bool {
	if writeInitial != nil {
		return true
	}
	switch kind {
	case "claude-code", "copilot", "gemini":
		return true
	default:
		return false
	}
}

// newSessionUUID returns a RFC 4122 v4 UUID string. claude --session-id refuses
// anything else.
func newSessionUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (runner *Runner) cancel(agentID string) error {
	runner.mu.Lock()
	acp := runner.acp[agentID]
	cs := runner.claude[agentID]
	proc, ok := runner.procs[agentID]
	runner.mu.Unlock()
	if acp != nil {
		acp.cancel()
		acp.shutdown(runner)
		runner.clearPending(agentID)
		// ACP turns don't run through cmd.Wait (which is what finalises CLI agents
		// on kill), so move the agent out of "working" ourselves — otherwise the UI
		// keeps showing it as running and the stop button never clears.
		acp.svc.markAgentStopped(agentID)
		return nil
	}
	if cs != nil {
		// Like ACP, the persistent claude process is killed directly (its readLoop
		// stands down on closed), so finalise the row ourselves. shutdown clears the
		// queue, but clear here too in case it had already closed (respawn handoff).
		cs.shutdown()
		runner.clearPending(agentID)
		cs.svc.markAgentStopped(agentID)
		return nil
	}
	if !ok || proc.cmd.Process == nil {
		return errors.New("no running process for agent")
	}
	if proc.stdin != nil {
		_ = proc.stdin.Close()
	}
	return proc.cmd.Process.Kill()
}

func (runner *Runner) cancelAll() {
	runner.mu.Lock()
	procs := make([]*runningProc, 0, len(runner.procs))
	for _, proc := range runner.procs {
		procs = append(procs, proc)
	}
	claudes := make([]*claudeSession, 0, len(runner.claude))
	for _, cs := range runner.claude {
		claudes = append(claudes, cs)
	}
	runner.mu.Unlock()
	for _, proc := range procs {
		if proc.stdin != nil {
			_ = proc.stdin.Close()
		}
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
	}
	for _, cs := range claudes {
		cs.shutdown()
	}
}

// writeStdin sends a line to a non-claude agent currently waiting on stdin.
// Returns false when no process is running or stdin wasn't piped.
func (runner *Runner) writeStdin(agentID, message string) bool {
	runner.mu.Lock()
	proc, ok := runner.procs[agentID]
	runner.mu.Unlock()
	if !ok || proc.stdin == nil {
		return false
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	if _, err := io.WriteString(proc.stdin, message); err != nil {
		return false
	}
	return true
}

func (runner *Runner) wipeLogs() error {
	if runner.logsRoot == "" {
		return nil
	}
	entries, err := os.ReadDir(runner.logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(runner.logsRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (runner *Runner) isRunning(agentID string) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, ok := runner.procs[agentID]; ok {
		return ok
	}
	if _, ok := runner.claude[agentID]; ok {
		return true
	}
	_, ok := runner.acp[agentID]
	return ok
}

// queueIfRunning atomically checks whether the agent is currently running and
// either appends the message to its pending queue or reports back that the
// caller is free to spawn a fresh turn.
func (runner *Runner) queueIfRunning(agentID, message string) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	_, hasProc := runner.procs[agentID]
	_, hasACP := runner.acp[agentID]
	_, hasClaude := runner.claude[agentID]
	if !hasProc && !hasACP && !hasClaude {
		return false
	}
	runner.pending[agentID] = append(runner.pending[agentID], message)
	return true
}

// setPending replaces the agent's queue with a single message. claude-code keeps
// at most one pending follow-up: a second send supersedes the first rather than
// stacking, so the agent never silently runs a backlog of stale messages.
func (runner *Runner) setPending(agentID, message string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.pending[agentID] = []string{message}
}

// popPending removes and returns the next queued message for the agent. The
// second return value is false when the queue is empty.
func (runner *Runner) popPending(agentID string) (string, bool) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	q := runner.pending[agentID]
	if len(q) == 0 {
		return "", false
	}
	next := q[0]
	if len(q) == 1 {
		delete(runner.pending, agentID)
	} else {
		runner.pending[agentID] = q[1:]
	}
	return next, true
}

func (runner *Runner) clearPending(agentID string) []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	q := runner.pending[agentID]
	delete(runner.pending, agentID)
	return q
}

func (runner *Runner) run(svc *Service, agentID, kind, binary string, args []string, workDir, startMsg string, appendLog bool, extraEnv []string, writeInitial func(io.Writer) error) {
	if err := os.MkdirAll(runner.logsRoot, 0o755); err != nil {
		svc.markAgentError(agentID, fmt.Sprintf("logs dir: %v", err))
		return
	}
	logPath := filepath.Join(runner.logsRoot, agentID+".log")
	// Always open in append mode so an external ClearLog (truncate to 0) stays
	// safe: O_APPEND writes target the current end of file, so after a truncate
	// the next line is written at offset 0 instead of leaving a sparse hole. A
	// fresh (non-resume) run still starts empty via an explicit truncate.
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		svc.markAgentError(agentID, fmt.Sprintf("create log: %v", err))
		return
	}
	defer logFile.Close()
	if !appendLog {
		_ = logFile.Truncate(0)
	}
	// stdout and stderr are drained concurrently into the same file; serialise
	// every write so JSONL lines never interleave.
	sink := newLockedWriter(logFile)

	if startMsg != "" {
		evt := StreamEvent{Type: "user_message", Content: startMsg}
		emitEvent(sink, nil, evt)
		svc.emitLogEvent(agentID, evt)
	}

	onWaiting := func() { svc.markAgentWaiting(agentID) }

	logLine := func(line string) {
		evt := StreamEvent{Type: "system", Content: line}
		emitEvent(sink, nil, evt)
		svc.emitLogEvent(agentID, evt)
	}

	// Baseline so the live counter reflects the agent's running total across
	// turns; persistTurnStats adds the same per-turn figure when the turn ends.
	var baseTokens int
	var baseCost float64
	var baseParts usageParts
	if a, _ := svc.store.GetAgent(agentID); a != nil {
		baseTokens = a.Tokens.Total()
		baseCost = a.CostUSD
		baseParts = a.Tokens
	}

	// runAttempt spawns the subprocess once, streams its output, and waits for
	// exit. The returned bool reports whether a transient upstream API error
	// (overloaded / rate limit) was observed in the output, so the caller can
	// decide to re-run the turn against the same resumable session.
	runAttempt := func(binary string, args []string) (streamTurnStats, error, bool) {
		var stats streamTurnStats
		attemptStarted := time.Now()
		var codexThreadID string
		cmd := exec.Command(binary, args...)
		if workDir != "" {
			cmd.Dir = workDir
		}
		env := append(os.Environ(), extraEnv...)
		if kind == "claude-code" && os.Getenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS") == "" {
			env = append(env, "CLAUDE_CODE_MAX_OUTPUT_TOKENS=64000")
		}
		cmd.Env = env
		sysexec.Hide(cmd)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return stats, fmt.Errorf("stdout pipe: %w", err), false
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return stats, fmt.Errorf("stderr pipe: %w", err), false
		}
		var stdin io.WriteCloser
		if needsStdinPipe(kind, writeInitial) {
			var err error
			stdin, err = cmd.StdinPipe()
			if err != nil {
				return stats, fmt.Errorf("stdin pipe: %w", err), false
			}
		}

		if err := cmd.Start(); err != nil {
			return stats, fmt.Errorf("start: %w", err), false
		}

		runner.mu.Lock()
		runner.procs[agentID] = &runningProc{cmd: cmd, stdin: stdin}
		runner.mu.Unlock()
		_ = svc.store.PatchAgent(agentID, map[string]any{"pid": cmd.Process.Pid})

		if writeInitial != nil {
			if stdin == nil {
				return stats, errors.New("initial input requested without stdin pipe"), false
			}
			if err := writeInitial(stdin); err != nil {
				return stats, fmt.Errorf("send initial input: %w", err), false
			}
		}

		var retryableSeen atomic.Bool
		detect := func(evt StreamEvent) {
			if isRetryableAPIError(evt.Content) {
				retryableSeen.Store(true)
			}
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if kind == "claude-code" {
				stats = streamClaudeJSON(stdout, sink, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				}, func(toolUseID string, input map[string]any) {
					svc.emitAskUserQuestion(agentID, toolUseID, input)
				}, func(tokens int, parts usageParts, costUSD float64) {
					svc.emitTokens(agentID, baseTokens+tokens, baseParts.Add(parts), baseCost+costUSD)
				}, func(streamTurnStats) {
					runner.mu.Lock()
					proc, ok := runner.procs[agentID]
					runner.mu.Unlock()
					if ok && proc.stdin != nil {
						_ = proc.stdin.Close()
					}
				})
				return
			}
			if kind == "codex" {
				stats = streamCodexJSON(stdout, sink, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				}, func(threadID string) {
					if threadID == "" {
						return
					}
					codexThreadID = threadID
					_ = svc.store.PatchAgent(agentID, map[string]any{"sessionId": threadID})
				}, func(tokens int, parts usageParts, costUSD float64) {
					svc.emitTokens(agentID, baseTokens+tokens, baseParts.Add(parts), baseCost+costUSD)
				})
				return
			}
			if kind == "gemini" {
				stats = streamGeminiJSON(stdout, sink, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				}, func(toolUseID string, input map[string]any) {
					svc.emitAskUserQuestion(agentID, toolUseID, input)
				}, func(tokens int, parts usageParts, costUSD float64) {
					svc.emitTokens(agentID, baseTokens+tokens, baseParts.Add(parts), baseCost+costUSD)
				}, func() {
					runner.mu.Lock()
					proc, ok := runner.procs[agentID]
					runner.mu.Unlock()
					if ok && proc.stdin != nil {
						_ = proc.stdin.Close()
					}
				}, workDir)
				return
			}
			if kind == "cursor" {
				stats = streamCursorJSON(stdout, sink, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				}, func(tokens int, parts usageParts, costUSD float64) {
					svc.emitTokens(agentID, baseTokens+tokens, baseParts.Add(parts), baseCost+costUSD)
				})
				return
			}
			streamInteractive(stdout, sink, func(evt StreamEvent) {
				detect(evt)
				svc.emitLogEvent(agentID, evt)
			}, onWaiting)
		}()
		go func() {
			defer wg.Done()
			if kind == "claude-code" || kind == "cursor" || kind == "gemini" || kind == "codex" {
				if kind == "codex" {
					streamCodexStderr(stderr, sink, func(evt StreamEvent) {
						detect(evt)
						svc.emitLogEvent(agentID, evt)
					})
					return
				}
				streamLines(stderr, sink, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				})
				return
			}
			streamInteractive(stderr, sink, func(evt StreamEvent) {
				detect(evt)
				svc.emitLogEvent(agentID, evt)
			}, onWaiting)
		}()

		waitErr := cmd.Wait()
		wg.Wait()
		if kind == "codex" {
			stats.ToolsUsed += svc.mergeMissingCodexImageToolEvents(agentID, logPath, attemptStarted, codexThreadID)
		}

		runner.mu.Lock()
		if proc, ok := runner.procs[agentID]; ok && proc.stdin != nil {
			_ = proc.stdin.Close()
		}
		delete(runner.procs, agentID)
		runner.mu.Unlock()
		_ = svc.store.PatchAgent(agentID, map[string]any{"pid": 0})

		return stats, waitErr, retryableSeen.Load()
	}

	stats, waitErr, retryable := runAttempt(binary, args)

	// The subprocess was killed on purpose to wait for the user's answer to a
	// surfaced question/plan. The agent is already "waiting" with the pending
	// question persisted, so the kill is not a crash — finalise quietly.
	if runner.consumeAwaiting(agentID) {
		svc.persistTurnStats(agentID, stats)
		return
	}

	// A turn that hit a transient upstream error (overloaded / rate limit) and
	// did not complete cleanly is re-run with exponential backoff against the
	// same resumable session, rather than failing the agent immediately. After a
	// few same-model retries it falls back to a less-congested model (Opus →
	// Sonnet), mirroring standard claude. Only claude-code resumes by session
	// id, so the retry is scoped to it.
	if (kind == "claude-code" || kind == "gemini") && retryable && !stats.Succeeded {
		if a, _ := svc.store.GetAgent(agentID); a != nil {
			var fallback string
			if kind == "claude-code" {
				fallback = fallbackClaudeModel(a.Model)
			} else {
				fallback = fallbackGeminiModel(a.Model)
			}
			usingFallback := false
			resumeArgs := func(model string) []string {
				msg := ""
				// For gemini retry of initial spawn, we need the task.
				if kind == "gemini" && a.SessionID != "" {
					msg = a.Summary
				}
				_, ra, err := buildResumeCommand(kind, binary, a.SessionID, msg, a.Source, model, a.AllowedTools)
				if err != nil {
					return args
				}
				return ra
			}
			for attempt := 1; attempt <= maxAPIRetries && retryable && !stats.Succeeded; attempt++ {
				if !usingFallback && fallback != "" && attempt > overloadRetriesBeforeFallback {
					usingFallback = true
					logLine(fmt.Sprintf("↘ still overloaded — falling back to %q", fallback))
				}
				model := a.Model
				if usingFallback {
					model = fallback
				}
				delay := retryBackoff(attempt)
				logLine(fmt.Sprintf("⟳ upstream API overloaded — retrying in %s (attempt %d/%d, model %s)", delay, attempt, maxAPIRetries, strDefault(model, "auto")))
				time.Sleep(delay)
				stats, waitErr, retryable = runAttempt(binary, resumeArgs(model))
				if runner.consumeAwaiting(agentID) {
					svc.persistTurnStats(agentID, stats)
					return
				}
				// A fallback attempt that succeeded persists the model so the
				// switch sticks across turns until the user picks one via /model.
				if usingFallback && stats.Succeeded {
					_ = svc.store.PatchAgent(agentID, map[string]any{"model": fallback})
					logLine(fmt.Sprintf("✓ recovered on %q — keeping it for this agent", fallback))
				}
			}
		}
		if retryable && !stats.Succeeded {
			svc.persistTurnStats(agentID, stats)
			svc.runnerError(agentID, sink, fmt.Sprintf("upstream API unavailable after %d attempts (overloaded / rate limited); giving up", maxAPIRetries+1))
			if dropped := runner.clearPending(agentID); len(dropped) > 0 {
				_ = svc.appendAgentEvent(agentID, StreamEvent{Type: "system", Content: fmt.Sprintf("(dropped %d queued message(s))", len(dropped))})
			}
			return
		}
	}

	if waitErr != nil {
		svc.persistTurnStats(agentID, stats)
		svc.runnerError(agentID, sink, fmt.Sprintf("exited: %v", waitErr))
		// Drop any queued follow-ups since the chain is broken.
		if dropped := runner.clearPending(agentID); len(dropped) > 0 {
			_ = svc.appendAgentEvent(agentID, StreamEvent{Type: "system", Content: fmt.Sprintf("(dropped %d queued message(s))", len(dropped))})
		}
		return
	}
	svc.persistTurnStats(agentID, stats)
	svc.onTurnFinished(agentID)
}

func streamLines(reader io.Reader, sink io.Writer, onEvent func(StreamEvent)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if text := strings.TrimSpace(scanner.Text()); text != "" {
			emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: text})
		}
	}
	if err := scanner.Err(); err != nil {
		emitEvent(sink, onEvent, StreamEvent{Type: "system", Content: "✗ output stream truncated: " + err.Error()})
	}
}

// logEmitDebounce caps how often an agent's log notification reaches the
// frontend. A fast-streaming agent appends many lines per second; without this
// each append crossed the Wails bridge individually and the renderer re-read
// and re-rendered the entire transcript, eventually starving the UI thread.
const logEmitDebounce = 120 * time.Millisecond

// emitLogEvent notifies the frontend that the agent's log grew. The event
// carries only the agentId: subscribers read the new tail incrementally via
// ReadLogEventsFrom, so the per-notification cost stays proportional to the new
// output rather than the full log. Notifications are coalesced to at most one
// per logEmitDebounce window per agent.
func (service *Service) emitLogEvent(agentID string, _ StreamEvent) {
	if service.store == nil {
		return
	}
	service.logEmitMu.Lock()
	defer service.logEmitMu.Unlock()
	if service.logEmitTimer == nil {
		service.logEmitTimer = make(map[string]*time.Timer)
	}
	if _, scheduled := service.logEmitTimer[agentID]; scheduled {
		return
	}
	service.logEmitTimer[agentID] = time.AfterFunc(logEmitDebounce, func() {
		service.logEmitMu.Lock()
		delete(service.logEmitTimer, agentID)
		service.logEmitMu.Unlock()
		service.store.Emit(agentLogEvent, map[string]any{"agentId": agentID})
	})
}

func (service *Service) emitLogResetEvent(agentID string) {
	if service.store == nil {
		return
	}
	service.logEmitMu.Lock()
	if timer := service.logEmitTimer[agentID]; timer != nil {
		timer.Stop()
		delete(service.logEmitTimer, agentID)
	}
	service.logEmitMu.Unlock()
	service.store.Emit(agentLogEvent, map[string]any{"agentId": agentID, "reset": true})
}

// appendAgentEvent writes a StreamEvent as JSONL to the agent's log file and
// broadcasts it live. Used by ACP sessions and error reporting paths.
func (service *Service) appendAgentEvent(agentID string, evt StreamEvent) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	if err := os.MkdirAll(service.runner.logsRoot, 0o755); err != nil {
		return err
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, werr := fmt.Fprintln(f, marshalEvent(evt)); werr != nil {
		return werr
	}
	service.emitLogEvent(agentID, evt)
	return nil
}

func marshalEvent(evt StreamEvent) string {
	if evt.Ts == "" {
		evt.Ts = time.Now().Format("15:04:05")
	}
	data, _ := json.Marshal(evt)
	return string(data)
}

// ReadLogEvents reads an agent's log file and returns parsed StreamEvents.
// New-format lines (JSONL) are decoded directly; legacy text lines are wrapped
// as {type:"text"} for backward compatibility with pre-migration sessions.
func (service *Service) ReadLogEvents(agentID string) ([]StreamEvent, error) {
	if service.runner == nil {
		return nil, errors.New("runner not initialised")
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []StreamEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if evt, ok := parseLogLine(line); ok {
			events = append(events, evt)
		}
	}
	return events, scanner.Err()
}

// LogTail is the incremental slice of an agent's log returned by
// ReadLogEventsFrom: the events appended after the requested offset plus the
// byte offset to pass on the next call.
type LogTail struct {
	Events []StreamEvent `json:"events"`
	Offset int64         `json:"offset"`
}

// ReadLogEventsFrom returns the StreamEvents written to the agent's log after
// the given byte offset, along with the offset to resume from next time. Only
// complete lines (terminated by a newline) are consumed, so a partially written
// trailing line is left for the following read. This lets the live view append
// just the new output instead of re-reading and re-parsing the whole file.
func (service *Service) ReadLogEventsFrom(agentID string, offset int64) (LogTail, error) {
	if service.runner == nil {
		return LogTail{Offset: offset}, errors.New("runner not initialised")
	}
	if offset < 0 {
		offset = 0
	}
	path := filepath.Join(service.runner.logsRoot, agentID+".log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LogTail{Offset: offset}, nil
		}
		return LogTail{Offset: offset}, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return LogTail{Offset: offset}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return LogTail{Offset: offset}, err
	}
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return LogTail{Offset: offset}, nil
	}
	consumed := data[:end+1]
	var events []StreamEvent
	for _, raw := range bytes.Split(consumed, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if evt, ok := parseLogLine(string(raw)); ok {
			events = append(events, evt)
		}
	}
	return LogTail{Events: events, Offset: offset + int64(len(consumed))}, nil
}

var legacyTimestampRe = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2})\]\s*`)

// parseLogLine converts a single log file line to a StreamEvent.
// JSONL format (starts with '{') is decoded directly. Legacy timestamp-prefixed
// text lines are wrapped as {type:"text"} for backward compatibility.
func parseLogLine(line string) (StreamEvent, bool) {
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		var evt StreamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &evt); err == nil {
			return evt, true
		}
	}
	ts := ""
	content := line
	if m := legacyTimestampRe.FindStringSubmatch(line); m != nil {
		ts = m[1]
		content = strings.TrimSpace(line[len(m[0]):])
	}
	if content == "" {
		return StreamEvent{}, false
	}
	return StreamEvent{Type: "text", Ts: ts, Content: content}, true
}
