package main

import (
	"fmt"
	"strings"

	"github.com/KevinBonnoron/polaris/internal/providers/tickets"
	"github.com/KevinBonnoron/polaris/internal/store/ticketsstore"
)

func (app *App) FetchTicketsSprint(cfg tickets.Config) (*tickets.Sprint, error) {
	if app.ticketsStore != nil {
		k := ticketsstore.Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), ProjectKey: cfg.ProjectKey, BoardID: cfg.BoardID}
		app.ticketsStore.SetConfig(k, cfg)
		if err := app.ticketsStore.Refresh(app.ctx, k); err != nil {
			return nil, err
		}
		snap, err := app.ticketsStore.GetSnapshot(app.ctx, k, cfg)
		if err != nil {
			return nil, err
		}
		if snap.Err != nil {
			return nil, snap.Err
		}
		return snap.Sprint, nil
	}
	return tickets.FetchActiveSprint(cfg)
}

func (app *App) ListTicketsBoards(cfg tickets.Config) ([]tickets.BoardInfo, error) {
	return tickets.ListBoards(cfg)
}

func (app *App) ListTicketsIssueTypes(cfg tickets.Config) ([]tickets.IssueType, error) {
	return tickets.ListProjectIssueTypes(cfg)
}

func (app *App) CreateTicketsIssue(cfg tickets.Config, in tickets.CreateIssueInput) (string, error) {
	return tickets.CreateIssue(cfg, in)
}

func (app *App) TransitionTicketsIssue(cfg tickets.Config, issueKey string, targetStatusIDs []string) error {
	return tickets.TransitionIssue(cfg, issueKey, targetStatusIDs)
}

func (app *App) FetchTicketsIssueDetail(cfg tickets.Config, issueKey string) (*tickets.IssueDetail, error) {
	return tickets.FetchIssueDetail(cfg, issueKey)
}

func (app *App) ListTicketsIssueComments(cfg tickets.Config, issueKey string) ([]tickets.Comment, error) {
	return tickets.ListIssueComments(cfg, issueKey)
}

func (app *App) ListTicketsIssueHistory(cfg tickets.Config, issueKey string) ([]tickets.HistoryEntry, error) {
	return tickets.ListIssueHistory(cfg, issueKey)
}

func (app *App) GetTicketsCurrentUser(cfg tickets.Config) (*tickets.User, error) {
	return tickets.GetCurrentUser(cfg)
}

func (app *App) AssignTicketsIssue(cfg tickets.Config, issueKey, accountID string) error {
	return tickets.AssignIssue(cfg, issueKey, accountID)
}

// ListTicketsFields returns the custom fields available on the Jira instance.
// Only custom fields with display-friendly types (number, string, option, …)
// are returned. Returns an error for non-Jira providers.
func (app *App) ListTicketsFields(cfg tickets.Config) ([]tickets.JiraField, error) {
	if cfg.Provider != "" && cfg.Provider != "jira" {
		return nil, fmt.Errorf("tickets: field listing is only supported for jira, got %q", cfg.Provider)
	}
	return tickets.ListJiraFields(cfg)
}
