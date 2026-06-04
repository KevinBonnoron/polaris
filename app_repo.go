package main

import (
	"fmt"

	"github.com/KevinBonnoron/polaris/internal/polaris"
	"github.com/KevinBonnoron/polaris/internal/providers/git"
)

func (app *App) projectGitDir(projectID string) (string, error) {
	if app.store == nil {
		return "", fmt.Errorf("store not ready")
	}
	proj, err := app.store.GetProject(projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if proj == nil || proj.Path == "" {
		return "", fmt.Errorf("project path unavailable")
	}
	return proj.Path, nil
}

func (app *App) GetProjectDiff(projectID string) (string, error) {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return "", err
	}
	return git.ProjectDiff(dir)
}

func (app *App) GenerateCommitMessageForProject(projectID string) (string, error) {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return "", err
	}
	diff, err := git.ProjectDiff(dir)
	if err != nil {
		return "", fmt.Errorf("get diff: %w", err)
	}
	return polaris.GenerateCommitMessage(diff)
}

func (app *App) GetProjectFileStatuses(projectID string) ([]git.FileChangeStatus, error) {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return nil, err
	}
	return git.ProjectFileStatuses(dir)
}

func (app *App) GetProjectGitState(projectID string) (git.AgentState, error) {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return git.AgentState{}, err
	}
	return git.CollectAgentState(dir)
}

func (app *App) StageProjectFile(projectID, path string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, []string{path})
}

func (app *App) StageProjectFiles(projectID string, paths []string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.StagePaths(dir, paths)
}

func (app *App) UnstageProjectFile(projectID, path string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, []string{path})
}

func (app *App) UnstageProjectFiles(projectID string, paths []string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.UnstagePaths(dir, paths)
}

func (app *App) StageProjectChanges(projectID string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	statuses, err := git.AgentFileStatuses(dir, nil)
	if err != nil {
		return err
	}
	toStage := []string{}
	for _, p := range statuses {
		if !p.Staged {
			toStage = append(toStage, p.Path)
		}
	}
	if len(toStage) == 0 {
		return nil
	}
	return git.StagePaths(dir, toStage)
}

func (app *App) UnstageProjectChanges(projectID string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	statuses, err := git.AgentFileStatuses(dir, nil)
	if err != nil {
		return err
	}
	toUnstage := []string{}
	for _, p := range statuses {
		if p.Staged {
			toUnstage = append(toUnstage, p.Path)
		}
	}
	if len(toUnstage) == 0 {
		return nil
	}
	return git.UnstagePaths(dir, toUnstage)
}

func (app *App) CommitProject(projectID, message string, amend bool) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.Commit(dir, message, amend)
}

func (app *App) PushProject(projectID string, force bool) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.Push(dir, force)
}

func (app *App) SyncProject(projectID string, force bool) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.Sync(dir, force)
}

func (app *App) ListProjectBranches(projectID string) ([]git.BranchInfo, error) {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return nil, err
	}
	return git.ListBranches(dir)
}

func (app *App) SwitchProjectBranch(projectID, branch string) error {
	dir, err := app.projectGitDir(projectID)
	if err != nil {
		return err
	}
	return git.CheckoutBranch(dir, branch)
}
