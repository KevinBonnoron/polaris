package main

import (
	"fmt"
	"os"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
)

func (app *App) CreatePRForAgent(agentID string) (string, error) {
	if app.svc == nil {
		return "", fmt.Errorf("service not ready")
	}
	return app.svc.CreatePRForAgent(agentID)
}

func (app *App) PromoteAgentToWorktree(agentID, branchName string) error {
	if app.svc == nil {
		return fmt.Errorf("service not ready")
	}
	return app.svc.PromoteAgentToWorktree(polaris.PromoteWorktreeInput{
		AgentID:    agentID,
		BranchName: branchName,
	})
}

func (app *App) GetAgentDiff(agentID string) (string, error) {
	if app.store == nil {
		return "", fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			return git.AgentDiff(agent.Worktree.Path)
		}
	}

	if agent.Worktree.Branch != "" {
		proj, projErr := app.store.GetProject(agent.ProjectID)
		if projErr == nil && proj != nil && proj.Path != "" {
			return git.BranchDiff(proj.Path, agent.Worktree.Branch)
		}
	}

	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr == nil && proj != nil && proj.Path != "" {
		if agent.Worktree.BaseTree != "" {
			return git.SnapshotDiff(proj.Path, agent.Worktree.BaseTree)
		}
		if app.svc != nil {
			logContent, logErr := app.svc.ReadLog(agentID)
			if logErr == nil && logContent != "" {
				return git.LogDiff(proj.Path, agent.StartedAt, logContent)
			}
		}
	}

	return "", nil
}

func (app *App) GetAgentFileStatuses(agentID string) ([]git.FileChangeStatus, error) {
	if app.store == nil {
		return nil, fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			return git.AgentFileStatuses(agent.Worktree.Path, nil)
		}
	}

	scope := app.agentScopedPaths(agentID)
	if proj, projErr := app.store.GetProject(agent.ProjectID); projErr == nil && proj != nil && proj.Path != "" {
		scope = git.RepoRelativePaths(proj.Path, scope)
	}

	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return nil, fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return []git.FileChangeStatus{}, nil
	}
	if len(scope) == 0 {
		return []git.FileChangeStatus{}, nil
	}
	return git.AgentFileStatuses(proj.Path, scope)
}

func (app *App) agentScopedPaths(agentID string) []string {
	if app.store != nil {
		if agent, err := app.store.GetAgent(agentID); err == nil && agent != nil && agent.Worktree.BaseTree != "" {
			if proj, projErr := app.store.GetProject(agent.ProjectID); projErr == nil && proj != nil && proj.Path != "" {
				if paths, sErr := git.SnapshotScopedPaths(proj.Path, agent.Worktree.BaseTree); sErr == nil {
					return paths
				}
			}
		}
	}
	if app.svc == nil {
		return nil
	}
	logContent, err := app.svc.ReadLog(agentID)
	if err != nil || logContent == "" {
		return nil
	}
	return git.ExtractLogFilePaths(logContent)
}

func (app *App) GetAgentGitState(agentID string) (git.AgentState, error) {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return git.AgentState{}, err
	}
	return git.CollectAgentState(dir)
}

func (app *App) CommitAgent(agentID, message string, amend bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Commit(dir, message, amend)
}

func (app *App) PushAgent(agentID string, force bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Push(dir, force)
}

func (app *App) SyncAgent(agentID string, force bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.Sync(dir, force)
}

func (app *App) StageAgentFile(agentID, path string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, []string{path})
}

func (app *App) StageAgentFiles(agentID string, paths []string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, paths)
}

func (app *App) UnstageAgentFile(agentID, path string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, []string{path})
}

func (app *App) UnstageAgentFiles(agentID string, paths []string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, paths)
}

func (app *App) DiscardAgentFile(agentID, path string, untracked bool) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	tracked, untrackedPaths := []string{}, []string{}
	if untracked {
		untrackedPaths = []string{path}
	} else {
		tracked = []string{path}
	}
	return git.DiscardPaths(dir, tracked, untrackedPaths)
}

func (app *App) DiscardAgentFiles(agentID string, tracked, untracked []string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}
	return git.DiscardPaths(dir, tracked, untracked)
}

func (app *App) GenerateCommitMessageForAgent(agentID string) (string, error) {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return "", err
	}
	diff, err := git.AgentDiff(dir)
	if err != nil {
		return "", fmt.Errorf("get diff: %w", err)
	}
	return app.svc.GenerateCommitMessage(diff)
}

func (app *App) ArchiveAgent(agentID string) error {
	return app.svc.ArchiveAgent(agentID)
}

func (app *App) agentGitDir(agentID string) (string, error) {
	if app.store == nil {
		return "", fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return "", fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found")
	}
	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			return agent.Worktree.Path, nil
		}
	}
	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return "", fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return "", fmt.Errorf("project path unavailable")
	}
	return proj.Path, nil
}

func (app *App) UnstageAgentChanges(agentID string) error {
	dir, err := app.agentGitDir(agentID)
	if err != nil {
		return err
	}

	scope := app.agentScopedPaths(agentID)
	if proj, projErr := app.store.GetProject(app.mustAgentProjectID(agentID)); projErr == nil && proj != nil && proj.Path != "" {
		scope = git.RepoRelativePaths(proj.Path, scope)
	}
	if len(scope) == 0 {
		return git.UnstagePaths(dir, []string{"."})
	}
	return git.UnstagePaths(dir, scope)
}

func (app *App) StageAgentChanges(agentID string) error {
	if app.store == nil {
		return fmt.Errorf("store not ready")
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found")
	}

	if agent.Worktree.Path != "" {
		if info, statErr := os.Stat(agent.Worktree.Path); statErr == nil && info.IsDir() {
			paths, pathErr := git.AgentChangedPaths(agent.Worktree.Path)
			if pathErr != nil {
				return pathErr
			}
			if len(paths) == 0 {
				return fmt.Errorf("no changes to stage")
			}
			return git.StagePaths(agent.Worktree.Path, paths)
		}
	}

	proj, projErr := app.store.GetProject(agent.ProjectID)
	if projErr != nil {
		return fmt.Errorf("get project: %w", projErr)
	}
	if proj == nil || proj.Path == "" {
		return fmt.Errorf("project path unavailable")
	}

	if app.svc != nil {
		if logContent, logErr := app.svc.ReadLog(agentID); logErr == nil && logContent != "" {
			paths := git.FilterStageable(proj.Path, git.RepoRelativePaths(proj.Path, git.ExtractLogFilePaths(logContent)))
			if len(paths) > 0 {
				return git.StagePaths(proj.Path, paths)
			}
		}
	}
	return fmt.Errorf("no changes to stage")
}

func (app *App) mustAgentProjectID(agentID string) string {
	if app.store == nil {
		return ""
	}
	agent, err := app.store.GetAgent(agentID)
	if err != nil || agent == nil {
		return ""
	}
	return agent.ProjectID
}
