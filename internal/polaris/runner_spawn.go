package polaris

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/git"
	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

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
	// in.ID is client-controlled (a draft being promoted in place) and becomes the
	// agent's log filename, so reject anything that could escape logsRoot and only
	// allow reusing a draft id from the same project.
	if in.ID != "" {
		if in.ID != strings.TrimSpace(in.ID) || strings.ContainsAny(in.ID, `/\`) {
			return nil, errors.New("invalid agent id")
		}
		existing, err := service.store.GetAgent(in.ID)
		if err != nil {
			return nil, fmt.Errorf("find existing agent: %w", err)
		}
		// A provided id must name an existing draft in this project — a reused id is
		// a promotion, never a way to create a fresh agent with a client-chosen id.
		if existing == nil {
			return nil, fmt.Errorf("draft agent %q not found", in.ID)
		}
		if existing.ProjectID != in.ProjectID || existing.Status != "draft" {
			return nil, fmt.Errorf("agent %q cannot be promoted", in.ID)
		}
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
		ID:        in.ID,
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
	go service.applyGeneratedTitle(created, task, summaryFromTask(task))

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
