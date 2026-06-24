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

	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/providers/repository"
	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type SpawnAgentInput struct {
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	Task      string `json:"task"`
	Model     string `json:"model,omitempty"`
	Binary    string `json:"binary,omitempty"`
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

func (service *Service) Spawn(in SpawnAgentInput) (*Agent, error) {
	if service.store == nil {
		return nil, errors.New("store not initialised")
	}
	if service.runner == nil {
		return nil, errors.New("runner not initialised")
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return nil, errors.New("task is required")
	}
	if in.ProjectID == "" {
		return nil, errors.New("projectId is required")
	}

	project, err := service.store.GetProject(in.ProjectID)
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

	// If the spawn carries a ticket and the project is a git repo, run the
	// agent inside a fresh worktree on a dedicated branch. Failures here are
	// non-fatal: the agent still runs in the project root so a misconfigured
	// repo doesn't block automations.
	branch, worktreePath, worktreeErr := service.prepareWorktree(in, project, workDir)
	if worktreeErr != nil {
		// Surface the failure as an error notification but keep going.
		_, _ = service.Notify(NotifyInput{
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

	// For non-isolated agents running in the project root, record the current
	// HEAD plus a snapshot of the working tree so PromoteAgentToWorktree can later
	// derive exactly this agent's changes, including shell-driven edits the log
	// never records.
	var baseCommit, baseTree string
	if worktreePath == "" && git.IsRepo(workDir) {
		baseCommit, _ = git.HeadCommit(workDir)
		baseTree, _ = git.SnapshotTree(workDir)
	}

	sessionID := newSessionUUID()
	cleanupWorktree := func() {
		if worktreePath != "" {
			_ = git.RemoveWorktree(workDir, worktreePath)
		}
	}

	// A custom provider always runs through the opencode harness.
	if in.ProviderID != "" {
		in.Kind = "opencode"
	}
	isACP := in.Kind == "opencode" || in.Kind == "mistral"
	var binary string
	var args []string
	var acpRT acpRuntime
	var acpEnv []string
	if isACP {
		var perr error
		if strings.TrimSpace(in.Model) == "" {
			perr = errors.New("a model must be selected")
		} else {
			acpEnv, perr = service.buildACPEnv(in.Kind, in.ProviderID, in.Model)
		}
		if perr == nil {
			acpRT, perr = service.acpRuntime(in.Kind, in.Binary)
		}
		if perr != nil {
			cleanupWorktree()
			return nil, perr
		}
	} else {
		var berr error
		binary, args, berr = buildSpawnCommand(in.Kind, in.Binary, in.Model, sessionID, task, source, in.AllowedTools)
		if berr != nil {
			cleanupWorktree()
			return nil, berr
		}
	}
	now := time.Now()
	created, err := service.store.UpsertAgent(Agent{
		ProjectID: in.ProjectID,
		Kind:      in.Kind,
		Summary:   "",
		Status:    "working",
		StartedAt: now.Unix(),
		SessionID: sessionID,
		Source:    source,
		Worktree: Worktree{
			Branch:     branch,
			Path:       worktreePath,
			IssueKey:   in.IssueKey,
			BaseCommit: baseCommit,
			BaseTree:   baseTree,
		},
		Model:        in.Model,
		ProviderID:   in.ProviderID,
		AllowedTools: in.AllowedTools,
	})
	if err != nil {
		cleanupWorktree()
		return nil, fmt.Errorf("create agent record: %w", err)
	}

	// The summary starts empty and is filled by an AI-generated title describing
	// the request. Async so the spawn returns immediately; the UI updates live
	// when PatchAgent emits. Generated once, at launch only. If generation fails,
	// the truncated first line is used as a fallback — but only on failure, never
	// while it is still pending.
	go service.applyGeneratedTitle(created.ID, task, summaryFromTask(task))

	if isACP {
		_ = service.appendAgentEvent(created.ID, StreamEvent{Type: "user_message", Content: task})
		env := append(os.Environ(), acpEnv...)
		if err := service.runner.startACPSession(service, created.ID, acpRT, runDir, task, env, created.Tokens.Total(), created.CostUSD, ""); err != nil {
			service.markAgentError(created.ID, fmt.Sprintf("start %s acp: %v", acpRT.label, err))
		}
		return &created, nil
	}

	// claude-code runs as a long-lived persistent session (see claudeSession);
	// the spawn args are rebuilt there from the agent record.
	if in.Kind == "claude-code" {
		if err := service.runner.startClaudeSession(service, &created, runDir, task, task, false, false); err != nil {
			service.markAgentError(created.ID, fmt.Sprintf("start claude-code: %v", err))
		}
		return &created, nil
	}

	go service.runner.run(service, created.ID, in.Kind, binary, args, runDir, task, false, nil, nil)
	return &created, nil
}

// prepareWorktree creates an isolated git worktree for a spawn that opts into
// isolation. Two flows trigger it:
//
//   - Ticket-style (IssueKey != ""): branch name derived from issue type/key/summary.
//   - Manual isolated (Isolated == true): branch name from BranchName if
//     provided, else `{project.BranchPrefix}{slug(task)}-{shortid}`.
//
// Returns ("", "", nil) when no isolation was requested or the project isn't
// a git repo — both are silent fallbacks that let the agent run in the
// project root. Returns a non-nil error only when isolation was asked for but
// git operations failed.
func (service *Service) prepareWorktree(in SpawnAgentInput, project *Project, projectPath string) (branch, worktreePath string, err error) {
	if in.IssueKey == "" && !in.Isolated {
		return "", "", nil
	}
	if projectPath == "" || !git.IsRepo(projectPath) {
		return "", "", nil
	}
	if service.runner == nil || service.runner.worktreesRoot == "" {
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

	worktreePath = filepath.Join(service.runner.worktreesRoot, in.ProjectID, leaf)
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

// PromoteWorktreeInput carries the parameters for PromoteAgentToWorktree.
type PromoteWorktreeInput struct {
	AgentID    string `json:"agentId"`
	BranchName string `json:"branchName"`
}

// PromoteAgentToWorktree moves a non-isolated agent into a fresh worktree on
// the given branch. It scopes the transfer to only the files the agent has
// touched (derived from its log), so unrelated working-tree changes are left
// in place. The agent's Worktree record is updated so subsequent turns run
// inside the new worktree.
func (service *Service) PromoteAgentToWorktree(in PromoteWorktreeInput) error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	branchName := strings.TrimSpace(in.BranchName)
	if branchName == "" {
		return errors.New("branch name is required")
	}

	agent, err := service.store.GetAgent(in.AgentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", in.AgentID)
	}
	if agent.Worktree.Path != "" {
		return errors.New("agent is already in a worktree")
	}
	if agent.PID != 0 && sysexec.ProcessAlive(agent.PID) {
		return errors.New("cannot promote a running agent")
	}

	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	if project == nil {
		return errors.New("project not found")
	}
	if !git.IsRepo(project.Path) {
		return errors.New("project is not a git repository")
	}
	if service.runner == nil || service.runner.worktreesRoot == "" {
		return errors.New("worktrees root not configured")
	}

	baseRef := agent.Worktree.BaseCommit
	if baseRef == "" {
		baseRef = "HEAD"
	}

	// Derive the set of repo-relative paths this agent changed. The spawn snapshot
	// captures every change (including shell-driven edits the log misses); fall
	// back to log scoping for agents spawned before snapshots existed.
	baseTree := agent.Worktree.BaseTree
	var scopedPaths []string
	if baseTree != "" {
		scopedPaths, err = git.SnapshotScopedPaths(project.Path, baseTree)
		if err != nil {
			return fmt.Errorf("derive agent changes: %w", err)
		}
	} else {
		logContent, logErr := service.ReadLog(in.AgentID)
		if logErr != nil {
			return fmt.Errorf("read agent log: %w", logErr)
		}
		scopedPaths = git.RepoRelativePaths(project.Path, git.ExtractLogFilePaths(logContent))
	}

	leaf := sanitizeLeaf(branchName) + "-" + shortRand()[:6]
	worktreePath := filepath.Join(service.runner.worktreesRoot, agent.ProjectID, leaf)

	if err := git.CreateWorktreeAt(project.Path, worktreePath, branchName, baseRef); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	if err := git.ApplyScopedChanges(project.Path, worktreePath, baseRef, scopedPaths); err != nil {
		_ = git.RemoveWorktree(project.Path, worktreePath)
		return fmt.Errorf("apply changes: %w", err)
	}

	newWorktree := agent.Worktree
	newWorktree.Branch = branchName
	newWorktree.Path = worktreePath

	if err := service.store.PatchAgent(agent.ID, map[string]any{"worktree": newWorktree}); err != nil {
		_ = git.RemoveWorktree(project.Path, worktreePath)
		return fmt.Errorf("persist worktree: %w", err)
	}

	// Claude Code resolves session storage from CWD: ~/.claude/projects/<cwd-encoded>/<uuid>.jsonl.
	// Move the transcript so --resume works after the CWD shifts to the worktree. Done only
	// once the worktree is persisted, so a failed promotion never strands the transcript.
	if agent.SessionID != "" {
		relocateClaudeSession(project.Path, worktreePath, agent.SessionID)
	}

	// The changes now live in the worktree; rewind the project root to the spawn
	// snapshot so the agent's work is moved rather than copied, leaving any
	// pre-existing changes in the root untouched.
	if baseTree != "" {
		if err := git.RestorePathsToTree(project.Path, baseTree, scopedPaths); err != nil {
			return fmt.Errorf("revert project root: %w", err)
		}
	}

	return nil
}

// sanitizeLeaf turns a branch name into a filesystem-safe leaf for the
// worktree directory. Slashes become dashes, the rest is left alone since
// branch names already pass git check-ref-format.
func sanitizeLeaf(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

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

// agentRunDir picks the working directory for a follow-up turn. When the
// agent was spawned with an isolated worktree we honour it (the live tree
// still exists on disk); otherwise we fall back to the project root.
func agentRunDir(agent *Agent, project *Project) string {
	if agent != nil && agent.Worktree.Path != "" {
		if info, err := os.Stat(agent.Worktree.Path); err == nil && info.IsDir() {
			return agent.Worktree.Path
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
func (service *Service) RecoverInterruptedAgents() error {
	if service.store == nil {
		return errors.New("store not initialised")
	}
	autoResume := false
	if settings, err := service.store.GetGeneralSettings(); err == nil {
		autoResume = settings.AutoResumeSessions
	}
	stale := []Agent{}
	for _, status := range []string{"working", "waiting"} {
		batch, err := service.store.ListAgentsByStatus(status)
		if err != nil {
			return err
		}
		stale = append(stale, batch...)
	}
	for _, a := range stale {
		// An agent with an unanswered AskUserQuestion stays "waiting": the
		// subprocess is gone but the persisted question still gives the user
		// a chance to answer, which kicks a resume on submit.
		if a.PendingQuestion != nil {
			_ = service.appendAgentEvent(a.ID, StreamEvent{Type: "system", Content: "(waiting for your answer after app restart)"})
			_ = service.store.PatchAgent(a.ID, map[string]any{"status": "waiting"})
			continue
		}
		// The agent's subprocess may still be running in another live app
		// instance sharing this database (e.g. an installed build alongside a
		// dev build). Don't touch it — only its owning instance streams its
		// output, and flipping the row to "error" here would falsely close a
		// session that is still progressing.
		if a.PID != 0 && sysexec.ProcessAlive(a.PID) {
			continue
		}
		if autoResume && a.SessionID != "" && a.Kind == "claude-code" {
			if err := service.autoResumeAgent(a); err == nil {
				continue
			}
		}
		_ = service.appendAgentEvent(a.ID, StreamEvent{Type: "system", Content: "(interrupted by app restart)"})
		_ = service.store.PatchAgent(a.ID, map[string]any{"status": "error"})
	}
	return nil
}

// autoResumeAgent kicks a neutral continuation turn for an interrupted agent.
// The prompt "..." is language-agnostic: Claude will continue in whatever
// language the session was already using.
func (service *Service) autoResumeAgent(a Agent) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	if a.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}
	project, err := service.store.GetProject(a.ProjectID)
	if err != nil {
		return err
	}
	const resumePrompt = "..."
	if err := service.store.PatchAgent(a.ID, map[string]any{"status": "working"}); err != nil {
		return err
	}
	_ = service.appendAgentEvent(a.ID, StreamEvent{Type: "system", Content: "(auto-resumed after app restart)"})
	workDir := agentRunDir(&a, project)
	// RecoverInterruptedAgents only calls this for claude-code (the one kind that
	// resumes by session id), which now runs as a persistent session.
	return service.runner.startClaudeSession(service, &a, workDir, resumePrompt, "", true, true)
}

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
		if len(head) > 80 {
			head = head[:77] + "..."
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

func buildSpawnCommand(kind, binary, model, sessionID, task, source string, allowedTools []string) (string, []string, error) {
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
		// stdin lets us deliver the initial task and each follow-up as a new user
		// turn on the same process.
		args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", permissionMode}
		if sessionID != "" {
			args = append(args, "--session-id", sessionID)
		}
		// "auto" (or empty) lets claude pick the model itself — omit --model.
		if model != "" && model != "auto" {
			args = append(args, "--model", model)
		}
		appendToolArgs(&args, allowedTools)
		_ = task // task is now sent on stdin as a JSON event by run()
		return bin, args, nil
	case "codex":
		bin := binary
		if bin == "" {
			bin = "codex"
		}
		args := []string{"exec", "--json"}
		if model != "" {
			args = append(args, "--model", model)
		}
		if task != "" {
			args = append(args, "--")
		}
		args = append(args, task)
		return bin, args, nil
	case "copilot":
		bin := binary
		if bin == "" {
			bin = "copilot"
		}
		return bin, []string{task}, nil
	case "gemini":
		bin := binary
		if bin == "" {
			bin = "gemini"
		}
		args := []string{"--output-format", "stream-json"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, "--prompt", task)
		return bin, args, nil
	case "cursor":
		bin := binary
		if bin == "" {
			bin = "agent"
		}
		args := []string{"--print", "--output-format", "stream-json", "--stream-partial-output", "--trust"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, task)
		return bin, args, nil
	default:
		return "", nil, fmt.Errorf("unknown agent kind %q", kind)
	}
}

// buildResumeCommand returns the command used to resume an existing session
// with a follow-up message. The permission mode mirrors the original spawn so
// automation-driven sessions keep their bypassPermissions setting across follow-ups.
func buildResumeCommand(kind, binary, sessionID, message, source, model string, allowedTools []string) (string, []string, error) {
	switch kind {
	case "claude-code":
		bin := binary
		if bin == "" {
			bin = "claude"
		}
		permissionMode := "bypassPermissions"
		_ = message // message is now sent on stdin as a JSON event by run()
		args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", permissionMode, "--resume", sessionID}
		// Pin the model explicitly on resume so a model the user (or an
		// overload fallback) switched to is honoured, instead of defaulting to
		// whatever the session was created with.
		if model != "" && model != "auto" {
			args = append(args, "--model", model)
		}
		appendToolArgs(&args, allowedTools)
		return bin, args, nil
	case "cursor":
		bin := binary
		if bin == "" {
			bin = "agent"
		}
		args := []string{"--print", "--output-format", "stream-json", "--stream-partial-output", "--trust", "--continue"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, message)
		return bin, args, nil
	case "codex":
		bin := binary
		if bin == "" {
			bin = "codex"
		}
		args := []string{"exec", "resume", "--json"}
		if model != "" {
			args = append(args, "--model", model)
		}
		args = append(args, sessionID)
		if message != "" {
			args = append(args, "--", message)
		}
		return bin, args, nil
	case "gemini":
		bin := binary
		if bin == "" {
			bin = "gemini"
		}
		args := []string{"--output-format", "stream-json"}
		if model != "" {
			args = append(args, "--model", model)
		}
		if message != "" {
			args = append(args, "--prompt", message)
		}
		return bin, args, nil
	default:
		return "", nil, fmt.Errorf("resume not supported for agent kind %q", kind)
	}
}

// appendToolArgs appends the appropriate --tools or --allowed-tools flag to
// args based on the stored AllowedTools slice. The sentinel ["__no_tools__"]
// uses --tools "" which claude-code recognises as "disable all tools".
// A non-empty list of specific tool names uses --allowed-tools.
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
				stats = streamCodexJSON(stdout, logFile, func(evt StreamEvent) {
					detect(evt)
					svc.emitLogEvent(agentID, evt)
				}, func(threadID string) {
					if threadID == "" {
						return
					}
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
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if text := strings.TrimSpace(scanner.Text()); text != "" {
			emitEvent(sink, onEvent, StreamEvent{Type: "text", Content: text})
		}
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

// agentTokensEvent carries a live, mid-turn token/cost snapshot so cards and
// the detail modal can update before the end-of-turn result is persisted. It is
// intentionally not written to the DB — persistTurnStats records the
// authoritative figure when the turn finishes.
const agentTokensEvent = "agent:tokens:updated"

// AgentTokensEventName returns the event the frontend should subscribe to for
// live token updates.
func AgentTokensEventName() string { return agentTokensEvent }

func (service *Service) emitTokens(agentID string, tokens int, parts usageParts, costUSD float64) {
	if service.store == nil {
		return
	}
	service.store.Emit(agentTokensEvent, map[string]any{
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

func (service *Service) emitAskUserQuestion(agentID, toolUseID string, input map[string]any) {
	if service.store == nil {
		return
	}
	// Surface only the first question/plan of a turn, then stop the subprocess.
	// claude-code in --print mode can't get an interactive answer mid-turn, so
	// left running it re-emits the same ExitPlanMode / AskUserQuestion in an
	// infinite loop that burns tokens. beginAwait returns false for those
	// duplicates so we ignore them; the user answers the surfaced one via a
	// resume turn.
	if service.runner != nil && !service.runner.beginAwait(agentID) {
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
	_ = service.store.PatchAgent(agentID, map[string]any{
		"status":          "waiting",
		"pendingQuestion": &PendingQuestion{ToolUseID: toolUseID, Input: inputJSON},
	})
	service.store.Emit(askUserQuestionEvent, map[string]any{
		"agentId":   agentID,
		"toolUseId": toolUseID,
		"input":     input,
	})
	service.notifyAgentEvent(agentID, "waiting", "")
	// Stop the subprocess now that the question is persisted and surfaced; the
	// answer is delivered later on a resumed session.
	if service.runner != nil {
		service.runner.stopForAwait(agentID)
	}
}

// RespondToAgentQuestion delivers the user's answer to a pending
// AskUserQuestion / ExitPlanMode tool call. The persisted question is dropped
// first and unconditionally so the panel never reappears. The answer is then
// delivered as a tool_result on the live subprocess's stdin when it's still
// alive; otherwise (the common claude-code case: it exits in --print mode after
// emitting the tool_use) it is delivered as the next user message on a resumed
// session — claude-code rejects a tool_result on resume (the API treats it as an
// assistant-message prefill).
func (service *Service) RespondToAgentQuestion(agentID, toolUseID, answer string) error {
	if service.runner == nil {
		return errors.New("runner not initialised")
	}
	// Read the agent once: we need PendingQuestion.Input before clearing it
	// (to craft the resume message) and the session/project info for the spawn.
	agent, err := service.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("find agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", agentID)
	}
	var pendingInput json.RawMessage
	if agent.PendingQuestion != nil {
		pendingInput = agent.PendingQuestion.Input
	}
	// Drop the persisted question immediately and unconditionally, before
	// attempting delivery. This guarantees the panel never reappears — on this
	// view, after an agent switch, or after a restart — no matter which delivery
	// path runs below or whether it succeeds.
	_ = service.store.PatchAgent(agentID, map[string]any{"pendingQuestion": nil})

	// Resolve the tool call in the log so its dot stops pulsing blue, and preserve
	// the exchange: claude auto-dismisses the question and its real tool_result is
	// suppressed, so this marker is the only record. RenderedContent keeps the full
	// question + chosen answer reviewable as the call's expandable preview.
	_ = service.appendAgentEvent(agentID, StreamEvent{Type: "tool_result", ID: toolUseID, Content: summarizeAnswer(answer), RenderedContent: questionAnswerRecap(pendingInput, answer)})

	working := map[string]any{"status": "working"}

	// Clear the await flag so a later question in the same persistent session is
	// surfaced again instead of being deduped as a re-emission.
	service.runner.consumeAwaiting(agentID)

	// opencode: the answer is the chosen option label of a pending ACP
	// permission request; deliver it to the live session.
	service.runner.mu.Lock()
	acp := service.runner.acp[agentID]
	cs := service.runner.claude[agentID]
	service.runner.mu.Unlock()
	if acp != nil {
		if acp.answerPermission(toolUseID, parseAUQAnswerLabel(answer)) {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
	}

	// claude-code persistent session: claude auto-dismissed the interactive tool
	// and ended the turn, so the process is alive but idle. The answer is delivered
	// as the next user turn (a context-aware message, not a tool_result for the
	// already-closed tool), continuing in place without a resume.
	if cs != nil {
		if cs.answerQuestion(planResumeMessage(pendingInput, answer)) {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
	}

	service.runner.mu.Lock()
	proc, ok := service.runner.procs[agentID]
	service.runner.mu.Unlock()
	if ok && proc.stdin != nil {
		if err := writeClaudeToolResult(proc.stdin, toolUseID, answer, false); err == nil {
			_ = service.store.PatchAgent(agentID, working)
			return nil
		}
		// Stdin write failed (e.g. broken pipe: process already exited in
		// --print mode). Fall through to the resume path below.
	}

	// Subprocess gone: resume the session with a context-aware user message.
	// We bypass Send() intentionally — the approval/rejection message is an
	// internal implementation detail and must not appear as a chat bubble in
	// the conversation. The tool_result log entry above already records the
	// user's choice visually.
	// (tool_result is rejected on resume — the API treats it as an assistant
	// prefill because the incomplete turn was stripped from the session.)
	if agent.SessionID == "" {
		return errors.New("agent has no session id; cannot resume")
	}
	project, err := service.store.GetProject(agent.ProjectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	_ = service.store.PatchAgent(agentID, working)
	msg := planResumeMessage(pendingInput, answer)
	workDir := agentRunDir(agent, project)

	// claude-code resumes as a fresh persistent session (the live-stdin path above
	// is the normal case; this only runs if its process had already exited).
	if agent.Kind == "claude-code" {
		return service.runner.startClaudeSession(service, agent, workDir, msg, "", true, true)
	}

	binary, args, env, err := service.buildResume(agent, "")
	if err != nil {
		return fmt.Errorf("build resume: %w", err)
	}
	go service.runner.run(service, agentID, agent.Kind, binary, args, workDir, "", true, env, nil)
	return nil
}

// planResumeMessage formats a user-turn message for the resume path after the
// agent subprocess was killed mid-ExitPlanMode / AskUserQuestion. The raw JSON
// payload must never be sent verbatim: the "question" fields can contain the
// full plan text or question wording, which Claude may mistake for new input
// and re-trigger the same tool call.
//
// For ExitPlanMode (detected by the "Plan" header on the first question) we
// emit a clear approval/rejection sentence with the plan as context.
// For AskUserQuestion we emit Q+A pairs so Claude has full context even if the
// tool_use turn was stripped from the session on resume.
func planResumeMessage(input json.RawMessage, answer string) string {
	label := summarizeAnswer(answer)
	if label == "" {
		return answer
	}
	var payload struct {
		Questions []struct {
			Header   string `json:"header"`
			Question string `json:"question"`
		} `json:"questions"`
	}
	if json.Unmarshal(input, &payload) != nil || len(payload.Questions) == 0 {
		return label
	}
	if payload.Questions[0].Header == "Plan" {
		plan := payload.Questions[0].Question
		switch label {
		case "Approve & proceed":
			return fmt.Sprintf("The user approved the plan. Please implement it now. Do not call ExitPlanMode or re-enter plan mode.\n\nApproved plan:\n%s", plan)
		case "Reject":
			return fmt.Sprintf("The user rejected the plan. Please reconsider your approach.\n\nRejected plan:\n%s", plan)
		default:
			return fmt.Sprintf("The user responded to your plan with: %s\n\nPlan:\n%s", label, plan)
		}
	}
	// AskUserQuestion: pair each question with its answer.
	var answers []struct {
		Answer json.RawMessage `json:"answer"`
	}
	if json.Unmarshal([]byte(answer), &answers) != nil {
		return label
	}
	var sb strings.Builder
	for i, q := range payload.Questions {
		if i >= len(answers) {
			break
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("Q: ")
		sb.WriteString(q.Question)
		sb.WriteString("\nA: ")
		var s string
		var arr []string
		switch {
		case json.Unmarshal(answers[i].Answer, &s) == nil:
			sb.WriteString(s)
		case json.Unmarshal(answers[i].Answer, &arr) == nil:
			sb.WriteString(strings.Join(arr, ", "))
		default:
			sb.Write(answers[i].Answer)
		}
	}
	if sb.Len() == 0 {
		return label
	}
	return sb.String()
}

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
// resumable (sessionId is kept), so a stop lands on "completed" rather than
// "error"; any pending question is cleared so its panel disappears.
func (service *Service) markAgentStopped(agentID string) {
	if service.store == nil {
		return
	}
	_ = service.appendAgentEvent(agentID, StreamEvent{Type: "system", Content: "(stopped by user)"})
	_ = service.store.PatchAgent(agentID, map[string]any{"status": "completed", "pendingQuestion": nil})
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
