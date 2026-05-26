package main

import (
	"github.com/KevinBonnoron/polaris/internal/providers/repository"
)

func (app *App) ListRepoPullRequests(owner, repo string) ([]repository.PullRequest, error) {
	if app.repositoryStore != nil {
		if err := app.repositoryStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.repositoryStore.GetPRs(app.ctx, owner, repo)
	}
	return repository.ListPullRequests(owner, repo)
}

func (app *App) ListRepoIssues(owner, repo string) ([]repository.Issue, error) {
	if app.repositoryStore != nil {
		if err := app.repositoryStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		return app.repositoryStore.GetIssues(app.ctx, owner, repo)
	}
	return repository.ListIssues(owner, repo)
}

func (app *App) ListRepoWorkflowRuns(owner, repo string, page int) (*repository.WorkflowRunsPage, error) {
	if page > 1 {
		return repository.ListWorkflowRuns(owner, repo, page)
	}
	if app.repositoryStore != nil {
		if err := app.repositoryStore.Refresh(app.ctx, owner, repo); err != nil {
			return nil, err
		}
		runs, err := app.repositoryStore.GetRuns(app.ctx, owner, repo)
		if err != nil {
			return nil, err
		}
		return &repository.WorkflowRunsPage{Runs: runs, HasMore: len(runs) >= 30}, nil
	}
	return repository.ListWorkflowRuns(owner, repo, page)
}

func (app *App) ListRepoBranches(owner, repo string) ([]string, error) {
	return repository.ListBranches(owner, repo)
}

func (app *App) GetRepositoryCurrentUser() (string, error) {
	return repository.GetCurrentUser()
}

func (app *App) GetRepoWorkflowDispatch(owner, repo string, workflowID int64) (*repository.WorkflowDispatchSpec, error) {
	return repository.GetWorkflowDispatch(owner, repo, workflowID)
}

func (app *App) TriggerRepoWorkflow(owner, repo string, workflowID int64, ref string, inputs map[string]string) error {
	return repository.TriggerWorkflowDispatch(owner, repo, workflowID, ref, inputs)
}

func (app *App) CancelRepoWorkflowRun(owner, repo string, runID int64) error {
	return repository.CancelWorkflowRun(owner, repo, runID)
}

func (app *App) RerunRepoWorkflowRun(owner, repo string, runID int64) error {
	return repository.RerunWorkflowRun(owner, repo, runID)
}
