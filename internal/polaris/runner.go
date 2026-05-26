package polaris

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/gh"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type SpawnAgentInput struct {
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	Task      string `json:"task"`
	Model     string `json:"model,omitempty"`
	Binary    string `json:"binary,omitempty"`
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
	// Isolated opts the spawn into the worktree workflow even without a Jira
	// ticket. When true (manual spawn with project IsolatedDefault on, or
	// explicit override), a fresh branch is created off project HEAD with
	// BranchName if provided, else `{project.BranchPrefix}{slug(task)}-{id}`.
	Isolated bool `json:"isolated,omitempty"`
	// BranchName forces the worktree branch name when Isolated is true. Used by
	// callers that know the desired name (e.g. automation builders with custom
	// prefixes). Empty triggers auto-generation.
	BranchName string `json:"branchName,omitempty"`
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
	pending       map[string][]string
	logsRoot      string
	worktreesRoot string
}

func NewRunner(logsRoot, worktreesRoot string) *Runner {
	return &Runner{
		procs:         make(map[string]*runningProc),
		pending:       make(map[string][]string),
		logsRoot:      logsRoot,
		worktreesRoot: worktreesRoot,
	}
}

func (s *Service) WithRunner(r *Runner) *Service {
	s.runner = r
	return s
}

// agentLogEvent is the Wails event name carrying a new log line for a given
// agent. Subscribers should filter by `agentId`.
const agentLogEvent = "agent:log:appended"

// AgentLogEventName returns the event name the frontend should subscribe to for
// live log updates. Exported so the binding stays in one place.
func AgentLogEventName() string { return agentLogEvent }

func (s *Service) Spawn(in SpawnAgentInput) (*Agent, error) {
	if s.store == nil {
		return nil, errors.New("store not initialised")
	}
	if s.runner == nil {
		return nil, errors.New("runner not initialised")
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return nil, errors.New("task is required")
	}
	if in.ProjectID == "" {
		return nil, errors.New("projectId is required")
	}

	project, err := s.store.GetProject(in.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("find project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project %q not found", in.ProjectID)
	}
	workDir := project.Path
	if workDir != "" {
		if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("project path %q is not a directory", workDir)
		}
	}

	source := in.Source
	if source == "" {
		source = "manual"
	}

	// If the spawn carries a Jira ticket and the project is a git repo, run the
	// agent inside a fresh worktree on a dedicated branch. Failures here are
	// non-fatal: the agent still runs in the project root so a misconfigured
	// repo doesn't block automations.
	branch, worktreePath, worktreeErr := s.prepareWorktree(in, project, workDir)
	if worktreeErr != nil {
		// Surface the failure as an error notification but keep going.
		_, _ = s.Notify(NotifyInput{
			ProjectID:     in.ProjectID,
			Type:          NotifTypeUser,
			Severity:      SeverityError,
			TitleTemplate: fmt.Sprintf("Could not create worktree for %s: %v — falling back to project root", in.IssueKey, worktreeErr),
		})
	}
	runDir := workDir
	if worktreePath != "" {
		runDir = worktreePath
	}

	sessionID := newSessionUUID()
	binary, args, err := buildSpawnCommand(in.Kind, in.Binary, in.Model, sessionID, task, source)
	if err != nil {
		if worktreePath != "" {
			_ = git.RemoveWorktree(workDir, worktreePath)
		}
		return nil, err
	}
	summary := task
	if idx := strings.IndexAny(summary, "\r\n"); idx >= 0 {
		summary = summary[:idx]
	}
	if len(summary) > 200 {
		summary = summary[:197] + "..."
	}
	now := time.Now()
	created, err := s.store.UpsertAgent(Agent{
		ProjectID:    in.ProjectID,
		Kind:         in.Kind,
		Summary:      summary,
		Status:       "working",
		StartedAt:    now.Unix(),
		SessionID:    sessionID,
		Source:       source,
		Branch:       branch,
		WorktreePath: worktreePath,
		IssueKey:     in.IssueKey,
		Model:        in.Model,
	})
	if err != nil {
		if worktreePath != "" {
			_ = git.RemoveWorktree(workDir, worktreePath)
		}
		return nil, fmt.Errorf("create agent record: %w", err)
	}

	startMsg := fmt.Sprintf("[%s] > %s", now.Format("15:04:05"), task)
	initialInput := ""
	if in.Kind == "claude-code" {
		initialInput = task
	}
	go s.runner.run(s, created.ID, in.Kind, binary, args, runDir, startMsg, initialInput, false)
	return &created, nil
}

// prepareWorktree creates an isolated git worktree for a spawn that opts into
// isolation. Two flows trigger it:
//
//   - Jira-style (IssueKey != ""): branch name derived from issue type/key/summary.
//   - Manual isolated (Isolated == true): branch name from BranchName if
//     provided, else `{project.BranchPrefix}{slug(task)}-{shortid}`.
//
// Returns ("", "", nil) when no isolation was requested or the project isn't
// a git repo — both are silent fallbacks that let the agent run in the
// project root. Returns a non-nil error only when isolation was asked for but
// git operations failed.
func (s *Service) prepareWorktree(in SpawnAgentInput, project *Project, projectPath string) (branch, worktreePath string, err error) {
	if in.IssueKey == "" && !in.Isolated {
		return "", "", nil
	}
	if projectPath == "" || !git.IsRepo(projectPath) {
		return "", "", nil
	}
	if s.runner == nil || s.runner.worktreesRoot == "" {
		return "", "", errors.New("worktrees root not configured")
	}

	var leaf string
	switch {
	case in.IssueKey != "":
		branch = git.BranchNameForIssue(in.IssueType, in.IssueKey, in.IssueSummary)
		safeKey := strings.ReplaceAll(strings.ReplaceAll(in.IssueKey, "/", "-"), string(os.PathSeparator), "-")
		leaf = safeKey + "-" + shortRand()
	case in.BranchName != "":
		branch = in.BranchName
		leaf = sanitizeLeaf(in.BranchName) + "-" + shortRand()
	default:
		branch = manualBranchName(project, in.Task)
		leaf = sanitizeLeaf(branch) + "-" + shortRand()
	}

	worktreePath = filepath.Join(s.runner.worktreesRoot, in.ProjectID, leaf)
	if err := git.CreateWorktree(projectPath, worktreePath, branch); err != nil {
		return "", "", err
	}
	return branch, worktreePath, nil
}

// manualBranchName generates `{prefix}{slug(task)}-{shortid}` for a manual
// isolated spawn. Falls back to "polaris/" prefix when the project doesn't
// have one configured. The shortid keeps names unique across concurrent
// spawns on similar tasks.
func manualBranchName(project *Project, task string) string {
	prefix := "polaris/"
	if project != nil && project.BranchPrefix != "" {
		prefix = project.BranchPrefix
	}
	slug := git.Slug(task)
	if slug == "" {
		slug = "agent"
	}
	const maxSlug = 40
	if len(slug) > maxSlug {
		slug = strings.TrimRight(slug[:maxSlug], "-")
	}
	return prefix + slug + "-" + shortRand()[:6]
}

// sanitizeLeaf turns a branch name into a filesystem-safe leaf for the
// worktree directory. Slashes become dashes, the rest is left alone since
// branch names already pass git check-ref-format.
func sanitizeLeaf(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

// agentRunDir picks the working directory for a follow-up turn. When the
// agent was spawned with an isolated worktree we honour it (the live tree
// still exists on disk); otherwise we fall back to the project root.
func agentRunDir(agent *Agent, project *Project) string {
	if agent != nil && agent.WorktreePath != "" {
		if info, err := os.Stat(agent.WorktreePath); err == nil && info.IsDir() {
			return agent.WorktreePath
		}
	}
	if project != nil {
		return project.Path
	}
	return ""
}

func shortRand() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%08x", b)
}

// RecoverInterruptedAgents handles agents left in an in-flight state at
// startup — typically processes killed by an app crash or reboot. When the
// GeneralSettings.AutoResumeSessions preference is enabled and the agent has
// a session id, we spawn a neutral continuation turn (claude --resume with
// "..." as the prompt) so the conversation picks up where it left off.
// Otherwise we flip the row to "error" so the UI reflects reality and the
// user can resume manually.
func (s *Service) RecoverInterruptedAgents() error {
	if s.store == nil {
		return errors.New("store not initialised")
	}
	autoResume := false
	if settings, err := s.store.GetGeneralSettings(); err == nil {
		autoResume = settings.AutoResumeSessions
	}
	stale := []Agent{}
	for _, status := range []string{"working", "waiting"} {
		batch, err := s.store.ListAgentsByStatus(status)
		if err != nil {
			return err
		}
		stale = append(stale, batch...)
	}
	for _, a := range stale {
		// An agent with an unanswered AskUserQuestion stays "waiting": the
		// subprocess is gone but the persisted question still gives the user
		// a chance to answer, which kicks a resume on submit.
		if a.PendingQuestionID != "" {
			_ = s.appendAgentLog(a.ID, fmt.Sprintf("[%s] (waiting for your answer after app restart)", time.Now().Format("15:04:05")))
			_ = s.store.PatchAgent(a.ID, map[string]any{"status": "waiting"})
			continue
		}
		if autoResume && a.SessionID != "" && a.Kind == "claude-code" {
			if err := s.autoResumeAgent(a); err == nil {
				continue
			}
		}
		_ = s.appendAgentLog(a.ID, fmt.Sprintf("[%s] (interrupted by app restart)", time.Now().Format("15:04:05")))
		_ = s.store.PatchAgent(a.ID, map[string]any{"status": "error"})
	}
	return nil
}

// autoResumeAgent kicks a neutral continuation turn for an interrupted agent.
// The prompt "..." is language-agnostic: Claude will continue in whatever
// language the session was already using.
func (s *Service) autoResumeAgent(a Agent) error {
	if s.runner == nil {
		return errors.New("runner not initialised")
	}
	project, err := s.store.GetProject(a.ProjectID)
	if err != nil {
		return err
	}
	const resumePrompt = "..."
	binary, args, err := buildResumeCommand(a.Kind, "", a.SessionID, resumePrompt, a.Source)
	if err != nil {
		return err
	}
	if err := s.store.PatchAgent(a.ID, map[string]any{"status": "working"}); err != nil {
		return err
	}
	stamped := fmt.Sprintf("[%s] (auto-resumed after app restart)", time.Now().Format("15:04:05"))
	_ = s.appendAgentLog(a.ID, stamped)
	workDir := agentRunDir(&a, project)
	initialInput := ""
	if a.Kind == "claude-code" {
		initialInput = resumePrompt
	}
	go s.runner.run(s, a.ID, a.Kind, binary, args, workDir, "", initialInput, true)
	return nil
}

func (s *Service) Send(agentID, message string) error {
	if s.store == nil {
		return errors.New("store not initialised")
	}
	if s.runner == nil {
		return errors.New("runner not initialised")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("message is required")
	}
	if agentID == "" {
		return errors.New("agentId is required")
	}

	agent, err := s.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}

	// A new user message supersedes any pending AskUserQuestion: the user
	// chose to answer via free text instead of the choices, so drop the
	// persisted question so the panel disappears.
	if agent.PendingQuestionID != "" {
		_ = s.store.PatchAgent(agentID, map[string]any{"pendingQuestionId": "", "pendingQuestionInput": ""})
	}

	// Non-claude agents are interactive CLIs: a follow-up message is a reply
	// piped to the live subprocess on stdin. The process must still be running
	// (we don't restart copilot/codex sessions from scratch).
	if agent.Kind != "claude-code" {
		if !s.runner.isRunning(agentID) {
			return fmt.Errorf("agent %q is no longer running", agentID)
		}
		stamped := fmt.Sprintf("[%s] > %s", time.Now().Format("15:04:05"), message)
		if err := s.appendAgentLog(agentID, stamped); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
		if !s.runner.writeStdin(agentID, message) {
			return fmt.Errorf("cannot write to %q stdin", agent.Kind)
		}
		// User answered the prompt: the subprocess is busy again until the
		// next prompt (or exit).
		_ = s.store.PatchAgent(agentID, map[string]any{"status": "working"})
		return nil
	}

	if agent.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}

	stamped := fmt.Sprintf("[%s] > %s", time.Now().Format("15:04:05"), message)
	if err := s.appendAgentLog(agentID, stamped); err != nil {
		return fmt.Errorf("write log: %w", err)
	}

	// Already running: queue and let the in-flight turn pick it up.
	if s.runner.queueIfRunning(agentID, message) {
		return nil
	}

	project, err := s.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	workDir := agentRunDir(agent, project)

	binary, args, err := buildResumeCommand(agent.Kind, "", agent.SessionID, message, agent.Source)
	if err != nil {
		return err
	}

	if err := s.store.PatchAgent(agentID, map[string]any{"status": "working"}); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	initialInput := ""
	if agent.Kind == "claude-code" {
		initialInput = message
	}
	// startMsg empty: we already wrote it above before queueing/spawning.
	go s.runner.run(s, agentID, agent.Kind, binary, args, workDir, "", initialInput, true)
	return nil
}

// appendAgentLog writes a single line to the agent's log file in append mode
// and broadcasts it live. Used to register a queued or about-to-spawn message
// without going through the runner's main pipeline.
func (s *Service) appendAgentLog(agentID, line string) error {
	if s.runner == nil {
		return errors.New("runner not initialised")
	}
	if err := os.MkdirAll(s.runner.logsRoot, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.runner.logsRoot, agentID+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, line); err != nil {
		return err
	}
	s.emitLogLine(agentID, line)
	return nil
}

// onTurnFinished drains the next queued message (if any) and chains a new
// turn, or marks the agent as completed when the queue is empty.
func (s *Service) onTurnFinished(agentID string) {
	next, has := s.runner.popPending(agentID)
	if !has {
		s.markAgentCompleted(agentID)
		return
	}

	agent, err := s.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}
	project, _ := s.store.GetProject(agent.ProjectID)
	workDir := agentRunDir(agent, project)
	binary, args, err := buildResumeCommand(agent.Kind, "", agent.SessionID, next, agent.Source)
	if err != nil {
		s.markAgentError(agentID, fmt.Sprintf("resume queued message: %v", err))
		return
	}
	initialInput := ""
	if agent.Kind == "claude-code" {
		initialInput = next
	}
	// startMsg empty: the message was already written to the log when Send
	// enqueued it.
	go s.runner.run(s, agentID, agent.Kind, binary, args, workDir, "", initialInput, true)
}

// CreatePRForAgent pushes the agent's branch and opens a PR via `gh`. The
// agent must have been spawned in worktree mode (Branch + WorktreePath
// populated) and the worktree must contain at least one commit ahead of its
// upstream — otherwise we'd be asking GitHub to open an empty PR.
func (s *Service) CreatePRForAgent(agentID string) (string, error) {
	if s.store == nil {
		return "", errors.New("store not initialised")
	}
	agent, err := s.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent %q not found", agentID)
	}
	if agent.PRURL != "" {
		return agent.PRURL, nil
	}
	if agent.Branch == "" || agent.WorktreePath == "" {
		return "", errors.New("agent has no branch/worktree to push")
	}
	if info, statErr := os.Stat(agent.WorktreePath); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("worktree %q no longer on disk", agent.WorktreePath)
	}
	hasCommits, err := git.HasCommitsAhead(agent.WorktreePath)
	if err != nil {
		return "", fmt.Errorf("check commits: %w", err)
	}
	if !hasCommits {
		return "", errors.New("no commits to push yet")
	}
	if err := git.PushBranch(agent.WorktreePath, agent.Branch); err != nil {
		return "", fmt.Errorf("push branch: %w", err)
	}
	title, body := agentPRMessage(agent)
	url, err := gh.CreatePullRequest(agent.WorktreePath, title, body)
	if err != nil {
		return "", err
	}
	if err := s.store.PatchAgent(agent.ID, map[string]any{"prUrl": url}); err != nil {
		// PR is already open; surface the error but return the URL so the UI
		// can still link to it.
		return url, fmt.Errorf("persist pr url: %w", err)
	}
	return url, nil
}

// agentPRMessage builds the title and body for `gh pr create` from the agent
// record. Title prefers the Jira key + summary; falls back to the task line
// when the spawn wasn't ticket-driven.
func agentPRMessage(agent *Agent) (title, body string) {
	summary := strings.TrimSpace(agent.Summary)
	if agent.IssueKey != "" {
		head := summary
		if idx := strings.IndexAny(head, "\r\n"); idx >= 0 {
			head = head[:idx]
		}
		head = strings.TrimSpace(head)
		if len(head) > 80 {
			head = head[:77] + "..."
		}
		if head == "" {
			title = agent.IssueKey
		} else {
			title = agent.IssueKey + " · " + head
		}
	} else {
		title = summary
		if idx := strings.IndexAny(title, "\r\n"); idx >= 0 {
			title = title[:idx]
		}
		if len(title) > 80 {
			title = title[:77] + "..."
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
func (s *Service) RemoveAgentWorktree(agent *Agent) {
	if agent == nil || agent.WorktreePath == "" {
		return
	}
	project, err := s.store.GetProject(agent.ProjectID)
	if err != nil || project == nil || project.Path == "" {
		// No way to call `git worktree remove` from the project root, fall
		// back to a plain rm so we don't leak the directory.
		_ = os.RemoveAll(agent.WorktreePath)
		return
	}
	_ = git.RemoveWorktree(project.Path, agent.WorktreePath)
}

func (s *Service) Cancel(agentID string) error {
	if s.runner == nil {
		return errors.New("runner not initialised")
	}
	return s.runner.cancel(agentID)
}

// ResetAll cancels every running agent, wipes the database, and deletes every
// log file under the runner's logs root. Used by the UI "delete all data"
// action.
func (s *Service) ResetAll() error {
	if s.store == nil {
		return errors.New("store not initialised")
	}
	if s.runner != nil {
		s.runner.cancelAll()
	}
	// Collect worktrees before the DB is wiped so we still know what to drop.
	var worktrees []*Agent
	if agents, err := s.store.ListAgents(""); err == nil {
		for i := range agents {
			if agents[i].WorktreePath != "" {
				worktrees = append(worktrees, &agents[i])
			}
		}
	}
	if err := s.store.ResetAll(); err != nil {
		return err
	}
	for _, a := range worktrees {
		s.RemoveAgentWorktree(a)
	}
	if s.runner != nil {
		if err := s.runner.wipeLogs(); err != nil {
			return fmt.Errorf("wipe logs: %w", err)
		}
		if s.runner.worktreesRoot != "" {
			_ = os.RemoveAll(s.runner.worktreesRoot)
		}
	}
	return nil
}

func (s *Service) ReadLog(agentID string) (string, error) {
	if s.runner == nil {
		return "", errors.New("runner not initialised")
	}
	path := filepath.Join(s.runner.logsRoot, agentID+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// ReadLogTail returns the last n non-empty lines of an agent's log file. n must
// be positive; for n <= 0 an empty slice is returned. Missing file is not an
// error — the agent may not have flushed any output yet.
func (s *Service) ReadLogTail(agentID string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	if s.runner == nil {
		return nil, errors.New("runner not initialised")
	}
	path := filepath.Join(s.runner.logsRoot, agentID+".log")
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

func buildSpawnCommand(kind, binary, model, sessionID, task, source string) (string, []string, error) {
	switch kind {
	case "claude-code":
		bin := binary
		if bin == "" {
			bin = "claude"
		}
		// In --print mode the agent has no TTY, so any permission prompt
		// stalls the run indefinitely. Always bypass.
		permissionMode := "bypassPermissions"
		// stream-json (both directions) gives us a full-duplex JSON channel:
		// stdout emits one event per assistant message / tool_use / result, and
		// stdin lets us deliver the initial task and answer tool_use events
		// like AskUserQuestion with a proper tool_result mid-turn.
		args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", permissionMode}
		if sessionID != "" {
			args = append(args, "--session-id", sessionID)
		}
		if model != "" {
			args = append(args, "--model", model)
		}
		_ = task // task is now sent on stdin as a JSON event by run()
		return bin, args, nil
	case "codex":
		bin := binary
		if bin == "" {
			bin = "codex"
		}
		return bin, []string{"exec", task}, nil
	case "copilot":
		bin := binary
		if bin == "" {
			bin = "copilot"
		}
		return bin, []string{task}, nil
	default:
		return "", nil, fmt.Errorf("unknown agent kind %q", kind)
	}
}

// buildResumeCommand returns the command used to resume an existing session
// with a follow-up message. Only claude-code supports this for now. The
// permission mode mirrors the original spawn so automation-driven sessions
// keep their bypassPermissions setting across follow-ups.
func buildResumeCommand(kind, binary, sessionID, message, source string) (string, []string, error) {
	switch kind {
	case "claude-code":
		bin := binary
		if bin == "" {
			bin = "claude"
		}
		permissionMode := "bypassPermissions"
		_ = message // message is now sent on stdin as a JSON event by run()
		return bin, []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", permissionMode, "--resume", sessionID}, nil
	default:
		return "", nil, fmt.Errorf("resume not supported for agent kind %q", kind)
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

func (r *Runner) cancel(agentID string) error {
	r.mu.Lock()
	proc, ok := r.procs[agentID]
	r.mu.Unlock()
	if !ok || proc.cmd.Process == nil {
		return errors.New("no running process for agent")
	}
	if proc.stdin != nil {
		_ = proc.stdin.Close()
	}
	return proc.cmd.Process.Kill()
}

func (r *Runner) cancelAll() {
	r.mu.Lock()
	procs := make([]*runningProc, 0, len(r.procs))
	for _, proc := range r.procs {
		procs = append(procs, proc)
	}
	r.mu.Unlock()
	for _, proc := range procs {
		if proc.stdin != nil {
			_ = proc.stdin.Close()
		}
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
	}
}

// writeClaudeUserText sends a plain user message as a stream-json event on
// claude's stdin.
func writeClaudeUserText(w io.Writer, text string) error {
	evt := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": text,
		},
	}
	return writeJSONLine(w, evt)
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

// writeStdin sends a line to a non-claude agent currently waiting on stdin.
// Returns false when no process is running or stdin wasn't piped.
func (r *Runner) writeStdin(agentID, message string) bool {
	r.mu.Lock()
	proc, ok := r.procs[agentID]
	r.mu.Unlock()
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

func (r *Runner) wipeLogs() error {
	if r.logsRoot == "" {
		return nil
	}
	entries, err := os.ReadDir(r.logsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(r.logsRoot, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) isRunning(agentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.procs[agentID]
	return ok
}

// queueIfRunning atomically checks whether the agent is currently running and
// either appends the message to its pending queue or reports back that the
// caller is free to spawn a fresh turn.
func (r *Runner) queueIfRunning(agentID, message string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.procs[agentID]; !ok {
		return false
	}
	r.pending[agentID] = append(r.pending[agentID], message)
	return true
}

// popPending removes and returns the next queued message for the agent. The
// second return value is false when the queue is empty.
func (r *Runner) popPending(agentID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.pending[agentID]
	if len(q) == 0 {
		return "", false
	}
	next := q[0]
	if len(q) == 1 {
		delete(r.pending, agentID)
	} else {
		r.pending[agentID] = q[1:]
	}
	return next, true
}

func (r *Runner) clearPending(agentID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.pending[agentID]
	delete(r.pending, agentID)
	return q
}

func (r *Runner) run(svc *Service, agentID, kind, binary string, args []string, workDir, startMsg, initialInput string, appendLog bool) {
	if err := os.MkdirAll(r.logsRoot, 0o755); err != nil {
		svc.markAgentError(agentID, fmt.Sprintf("logs dir: %v", err))
		return
	}
	logPath := filepath.Join(r.logsRoot, agentID+".log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendLog {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	logFile, err := os.OpenFile(logPath, flags, 0o644)
	if err != nil {
		svc.markAgentError(agentID, fmt.Sprintf("create log: %v", err))
		return
	}
	defer logFile.Close()

	if startMsg != "" {
		writeLogLine(logFile, startMsg)
		svc.emitLogLine(agentID, startMsg)
	}

	cmd := exec.Command(binary, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = os.Environ()
	sysexec.Hide(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		svc.runnerError(agentID, logFile, fmt.Sprintf("stdout pipe: %v", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		svc.runnerError(agentID, logFile, fmt.Sprintf("stderr pipe: %v", err))
		return
	}
	// All agent kinds now pipe stdin. Non-claude agents (copilot/codex) read
	// prompt answers from it; claude-code (in --input-format stream-json) reads
	// the initial user message and any mid-turn tool_result events from it.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		svc.runnerError(agentID, logFile, fmt.Sprintf("stdin pipe: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		svc.runnerError(agentID, logFile, fmt.Sprintf("start: %v", err))
		return
	}

	r.mu.Lock()
	r.procs[agentID] = &runningProc{cmd: cmd, stdin: stdin}
	r.mu.Unlock()

	if kind == "claude-code" && initialInput != "" {
		if err := writeClaudeUserText(stdin, initialInput); err != nil {
			svc.runnerError(agentID, logFile, fmt.Sprintf("send initial input: %v", err))
			return
		}
	}

	onWaiting := func() { svc.markAgentWaiting(agentID) }

	// Baseline so the live counter reflects the agent's running total across
	// turns; persistTurnStats adds the same per-turn figure when the turn ends.
	var baseTokens int
	var baseCost float64
	var baseParts usageParts
	if a, _ := svc.store.GetAgent(agentID); a != nil {
		baseTokens = a.Tokens
		baseCost = a.CostUSD
		baseParts = usageParts{Input: a.TokensInput, Output: a.TokensOutput, CacheCreation: a.TokensCacheCreate, CacheRead: a.TokensCacheRead}
	}

	var stats streamTurnStats
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if kind == "claude-code" {
			stats = streamClaudeJSON(stdout, logFile, func(line string) {
				svc.emitLogLine(agentID, line)
			}, func(toolUseID string, input map[string]any) {
				svc.emitAskUserQuestion(agentID, toolUseID, input)
			}, func(tokens int, parts usageParts, costUSD float64) {
				svc.emitTokens(agentID, baseTokens+tokens, baseParts.add(parts), baseCost+costUSD)
			}, func() {
				r.mu.Lock()
				proc, ok := r.procs[agentID]
				r.mu.Unlock()
				if ok && proc.stdin != nil {
					_ = proc.stdin.Close()
				}
			})
			return
		}
		streamInteractive(stdout, logFile, func(line string) {
			svc.emitLogLine(agentID, line)
		}, onWaiting)
	}()
	go func() {
		defer wg.Done()
		if kind == "claude-code" {
			streamLines(stderr, logFile, func(line string) {
				svc.emitLogLine(agentID, line)
			})
			return
		}
		streamInteractive(stderr, logFile, func(line string) {
			svc.emitLogLine(agentID, line)
		}, onWaiting)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	r.mu.Lock()
	if proc, ok := r.procs[agentID]; ok && proc.stdin != nil {
		_ = proc.stdin.Close()
	}
	delete(r.procs, agentID)
	r.mu.Unlock()

	if waitErr != nil {
		svc.persistTurnStats(agentID, stats)
		svc.runnerError(agentID, logFile, fmt.Sprintf("exited: %v", waitErr))
		// Drop any queued follow-ups since the chain is broken.
		if dropped := r.clearPending(agentID); len(dropped) > 0 {
			_ = svc.appendAgentLog(agentID, fmt.Sprintf("(dropped %d queued message(s))", len(dropped)))
		}
		return
	}
	svc.persistTurnStats(agentID, stats)
	svc.onTurnFinished(agentID)
}

func streamLines(reader io.Reader, sink io.Writer, onLine func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" {
			continue
		}
		stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), trimmed)
		fmt.Fprintln(sink, stamped)
		onLine(stamped)
	}
}

func writeLogLine(w io.Writer, line string) {
	fmt.Fprintln(w, line)
}

// emitLogLine pushes a single trimmed line to the frontend over the Wails event
// bus. Each card subscribes once and filters by agentId.
func (s *Service) emitLogLine(agentID, line string) {
	if s.store == nil {
		return
	}
	s.store.Emit(agentLogEvent, map[string]any{"agentId": agentID, "line": line})
}

// agentTokensEvent carries a live, mid-turn token/cost snapshot so cards and
// the detail modal can update before the end-of-turn result is persisted. It is
// intentionally not written to the DB — persistTurnStats records the
// authoritative figure when the turn finishes.
const agentTokensEvent = "agent:tokens:updated"

// AgentTokensEventName returns the event the frontend should subscribe to for
// live token updates.
func AgentTokensEventName() string { return agentTokensEvent }

func (s *Service) emitTokens(agentID string, tokens int, parts usageParts, costUSD float64) {
	if s.store == nil {
		return
	}
	s.store.Emit(agentTokensEvent, map[string]any{
		"agentId": agentID,
		"tokens":  tokens,
		"costUsd": costUSD,
		"parts":   parts,
	})
}

// askUserQuestionEvent is fired when the agent calls the AskUserQuestion tool.
// The frontend renders a dialog, then replies via RespondToAgentQuestion to
// deliver the answer as a proper tool_result on the agent's stdin.
const askUserQuestionEvent = "agent:ask-user-question"

// AskUserQuestionEventName returns the event the frontend should subscribe to.
func AskUserQuestionEventName() string { return askUserQuestionEvent }

func (s *Service) emitAskUserQuestion(agentID, toolUseID string, input map[string]any) {
	if s.store == nil {
		return
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		inputJSON = []byte("{}")
	}
	// Persist before emitting so a viewer that opens the modal later (or
	// after an app restart) can still see the choices instead of finding
	// only a failed agent with no context. Flip to "waiting" so cards show
	// the amber dot — the subprocess is alive but blocked on user input.
	_ = s.store.PatchAgent(agentID, map[string]any{
		"status":               "waiting",
		"pendingQuestionId":    toolUseID,
		"pendingQuestionInput": string(inputJSON),
	})
	s.store.Emit(askUserQuestionEvent, map[string]any{
		"agentId":   agentID,
		"toolUseId": toolUseID,
		"input":     input,
	})
	s.notifyAgentEvent(agentID, "waiting", "")
}

// RespondToAgentQuestion delivers the user's answer to a pending
// AskUserQuestion tool call. When the agent's subprocess is still alive it's
// sent as a proper tool_result on stdin. When the process is gone (e.g. the
// app restarted while a question was pending) we fall back to a resume
// turn, delivering the answer as the next user message so the session can
// continue from where it stopped.
func (s *Service) RespondToAgentQuestion(agentID, toolUseID, answer string) error {
	if s.runner == nil {
		return errors.New("runner not initialised")
	}
	// Clearing the persisted question + flipping status back to "working"
	// so the dot returns to blue while claude finishes its turn. The resume
	// path below leans on Send() which already patches "working".
	clear := map[string]any{"status": "working", "pendingQuestionId": "", "pendingQuestionInput": ""}
	s.runner.mu.Lock()
	proc, ok := s.runner.procs[agentID]
	s.runner.mu.Unlock()
	if ok && proc.stdin != nil {
		if err := writeClaudeToolResult(proc.stdin, toolUseID, answer, false); err != nil {
			return err
		}
		_ = s.store.PatchAgent(agentID, clear)
		return nil
	}
	// Subprocess gone: resume the session with the answer as the next user
	// message. Send handles the resume command construction for claude-code.
	if err := s.Send(agentID, answer); err != nil {
		return fmt.Errorf("resume to deliver answer: %w", err)
	}
	_ = s.store.PatchAgent(agentID, clear)
	return nil
}

// runnerError records a failure once the log file is already open: the message
// is appended to disk and emitted live, then the status flips to "error".
func (s *Service) runnerError(agentID string, logFile io.Writer, msg string) {
	writeLogLine(logFile, msg)
	s.emitLogLine(agentID, msg)
	_ = s.store.PatchAgent(agentID, map[string]any{"status": "error"})
	s.notifyAgentEvent(agentID, "error", msg)
}

// markAgentError handles failures that happen before the log file exists
// (e.g. cannot create logs dir). It only updates status and emits the message;
// nothing to write to disk because there is no file yet.
func (s *Service) markAgentError(agentID, msg string) {
	if s.store == nil {
		return
	}
	s.emitLogLine(agentID, msg)
	_ = s.store.PatchAgent(agentID, map[string]any{"status": "error"})
	s.notifyAgentEvent(agentID, "error", msg)
}

// persistTurnStats folds the per-turn stream-json stats into the agent row.
// Tokens and cost are additive; filesModified is re-derived from the full log
// to avoid double-counting files touched in multiple turns.
func (s *Service) persistTurnStats(agentID string, stats streamTurnStats) {
	if s.store == nil {
		return
	}
	if stats.Tokens == 0 && stats.CostUSD == 0 && stats.FilesModified == 0 && stats.ToolsUsed == 0 {
		return
	}
	agent, err := s.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	filesModified := agent.FilesModified + stats.FilesModified
	if logContent, logErr := s.ReadLog(agentID); logErr == nil && logContent != "" {
		filesModified = len(git.ExtractLogFilePaths(logContent))
	}

	patch := map[string]any{
		"tokens":            agent.Tokens + stats.Tokens,
		"tokensInput":       agent.TokensInput + stats.Parts.Input,
		"tokensOutput":      agent.TokensOutput + stats.Parts.Output,
		"tokensCacheCreate": agent.TokensCacheCreate + stats.Parts.CacheCreation,
		"tokensCacheRead":   agent.TokensCacheRead + stats.Parts.CacheRead,
		"costUsd":           agent.CostUSD + stats.CostUSD,
		"filesModified":     filesModified,
		"toolsUsed":         agent.ToolsUsed + stats.ToolsUsed,
	}
	_ = s.store.PatchAgent(agentID, patch)
}

func (s *Service) markAgentCompleted(agentID string) {
	if s.store == nil {
		return
	}
	// claude-code in --print mode exits after emitting the AskUserQuestion
	// tool_use without waiting for the tool_result, so cmd.Wait returns
	// cleanly and we land here while a question is still pending. Treating
	// that as "completed" would hide the choices behind a green dot — stay
	// in "waiting" until RespondToAgentQuestion clears the question (which
	// then drives a resume turn that finishes normally).
	agent, _ := s.store.GetAgent(agentID)
	if agent != nil && agent.PendingQuestionID != "" {
		return
	}
	_ = s.store.PatchAgent(agentID, map[string]any{"status": "completed"})
	s.notifyAgentEvent(agentID, "completed", "")
}

// markAgentWaiting flips the agent to "waiting" when the subprocess emits an
// interactive prompt (e.g. `[y/N]`). The status reverts to "working" the next
// time Send writes to stdin. Re-emitting from the stream parser on every byte
// would spam: we only patch when the recorded status isn't already "waiting".
func (s *Service) markAgentWaiting(agentID string) {
	if s.store == nil {
		return
	}
	agent, err := s.store.GetAgent(agentID)
	if err != nil || agent == nil || agent.Status == "waiting" {
		return
	}
	_ = s.store.PatchAgent(agentID, map[string]any{"status": "waiting"})
	s.notifyAgentEvent(agentID, "waiting", "")
}

// notifyAgentEvent emits a Notification tied to an agent's lifecycle. The
// title falls back to the agent's task summary so the inbox row stays
// meaningful even when the underlying reason is empty. The kind ("completed",
// "error", "waiting") is mapped to an AgentEvent + severity; users who don't
// want a given category can mute it via NotificationSettings.
func (s *Service) notifyAgentEvent(agentID, kind, detail string) {
	if s.store == nil {
		return
	}
	agent, err := s.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}
	event, severity := agentEventFromKind(kind)
	_, _ = s.Notify(NotifyInput{
		ProjectID:     agent.ProjectID,
		Type:          NotifTypeAgent,
		Severity:      severity,
		TitleTemplate: agentNotificationTitle(agent, event, severity, detail),
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

func agentNotificationTitle(agent *Agent, event AgentEvent, severity NotificationSeverity, detail string) string {
	task := strings.TrimSpace(agent.Summary)
	if len(task) > 80 {
		task = task[:77] + "..."
	}
	if event == AgentEventCompleted && severity == SeverityError {
		base := task
		if base == "" {
			base = agent.Kind
		}
		if detail != "" {
			return fmt.Sprintf("%s · %s", base, detail)
		}
		return fmt.Sprintf("%s · failed", base)
	}
	if event == AgentEventCompleted {
		if task == "" {
			return fmt.Sprintf("%s · task completed", agent.Kind)
		}
		return fmt.Sprintf("%s · %s", agent.Kind, task)
	}
	if task != "" {
		return task
	}
	return agent.Kind
}
