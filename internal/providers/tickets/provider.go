package tickets

import "fmt"

// Provider is the behaviour every ticket backend (Jira, Linear, …) implements.
// All shared DTOs live in this package, so a new backend only has to satisfy
// this interface — no caller outside the package depends on a concrete one.
type Provider interface {
	FetchActiveSprint(cfg Config) (*Sprint, error)
	ListBoards(cfg Config) ([]BoardInfo, error)
	ListProjectIssueTypes(cfg Config) ([]IssueType, error)
	CreateIssue(cfg Config, in CreateIssueInput) (string, error)
	TransitionIssue(cfg Config, issueKey string, targetStatusIDs []string) error
	GetCurrentUser(cfg Config) (*User, error)
	AssignIssue(cfg Config, issueKey, accountID string) error
	FetchLastComment(cfg Config, issueKey string) (string, error)
	FetchIssueDetail(cfg Config, issueKey string) (*IssueDetail, error)
	ListIssueComments(cfg Config, issueKey string) ([]Comment, error)
	ListIssueHistory(cfg Config, issueKey string) ([]HistoryEntry, error)
}

// jiraProvider is the Jira implementation of Provider (methods in jira.go).
type jiraProvider struct{}

// providerFor resolves the concrete backend from the config. An empty provider
// defaults to Jira so configs predating the multi-provider field keep working.
func providerFor(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "", "jira":
		return jiraProvider{}, nil
	default:
		return nil, fmt.Errorf("tickets: unsupported provider %q", cfg.Provider)
	}
}

// The functions below are the package's public API. They dispatch to the
// configured provider so callers never name a concrete backend.

func FetchActiveSprint(cfg Config) (*Sprint, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.FetchActiveSprint(cfg)
}

func ListBoards(cfg Config) ([]BoardInfo, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.ListBoards(cfg)
}

func ListProjectIssueTypes(cfg Config) ([]IssueType, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.ListProjectIssueTypes(cfg)
}

func CreateIssue(cfg Config, in CreateIssueInput) (string, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return "", err
	}
	return p.CreateIssue(cfg, in)
}

func TransitionIssue(cfg Config, issueKey string, targetStatusIDs []string) error {
	p, err := providerFor(cfg)
	if err != nil {
		return err
	}
	return p.TransitionIssue(cfg, issueKey, targetStatusIDs)
}

func GetCurrentUser(cfg Config) (*User, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.GetCurrentUser(cfg)
}

func AssignIssue(cfg Config, issueKey, accountID string) error {
	p, err := providerFor(cfg)
	if err != nil {
		return err
	}
	return p.AssignIssue(cfg, issueKey, accountID)
}

func FetchLastComment(cfg Config, issueKey string) (string, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return "", err
	}
	return p.FetchLastComment(cfg, issueKey)
}

func FetchIssueDetail(cfg Config, issueKey string) (*IssueDetail, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.FetchIssueDetail(cfg, issueKey)
}

func ListIssueComments(cfg Config, issueKey string) ([]Comment, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.ListIssueComments(cfg, issueKey)
}

func ListIssueHistory(cfg Config, issueKey string) ([]HistoryEntry, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	return p.ListIssueHistory(cfg, issueKey)
}
