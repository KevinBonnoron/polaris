// Package tickets provides integrations with issue tracking platforms
// like Jira, Linear, and Azure Boards. We translate each platform's verbose
// schema into small DTOs so the frontend doesn't depend on any single
// platform's full payloads.
package tickets

import "errors"

// Shared sentinel errors returned by every provider.
var (
	// ErrMissingConfig is returned when required credentials are absent.
	ErrMissingConfig = errors.New("tickets: missing baseUrl, email, token or projectKey")
	// ErrNoBoard means no agile board was found for the given project.
	ErrNoBoard = errors.New("tickets: no agile board found for project")
	// ErrNoSprint means the project's board has no active sprint.
	ErrNoSprint = errors.New("tickets: no active sprint on board")
	// ErrNoTransition is returned when the target status isn't reachable from
	// the issue's current state via any available workflow transition.
	ErrNoTransition = errors.New("tickets: no available transition to that status")
)

// CustomFieldConfig is one user-selected Jira custom field stored in the
// project integration config.
type CustomFieldConfig struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// CustomFieldValue is a resolved custom field value returned inside IssueDetail.
type CustomFieldValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// JiraField is one entry from the /rest/api/3/field endpoint, used to let the
// user pick which custom fields to show in the issue detail.
type JiraField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Config struct {
	// Provider selects the concrete ticket backend ("jira", "linear", …).
	// Empty defaults to "jira" for backward compatibility.
	Provider     string              `json:"provider,omitempty"`
	BaseURL      string              `json:"baseUrl"`
	Email        string              `json:"email"`
	Token        string              `json:"token"`
	ProjectKey   string              `json:"projectKey"`
	BoardID      int64               `json:"boardId,omitempty"`
	CustomFields []CustomFieldConfig `json:"customFields,omitempty"`
}

type BoardInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Issue struct {
	Key                  string   `json:"key"`
	Summary              string   `json:"summary"`
	IssueType            string   `json:"issueType"`
	Priority             string   `json:"priority"`
	Status               string   `json:"status"`
	StatusID             string   `json:"statusId"`
	StatusCategory       string   `json:"statusCategory"`
	Assignee             string   `json:"assignee"`
	AssigneeEmail        string   `json:"assigneeEmail"`
	Labels               []string `json:"labels"`
	URL                  string   `json:"url"`
	UpdatedAt            int64    `json:"updatedAt"`
	OriginalEstimateSec  *int64   `json:"originalEstimateSec,omitempty"`
	RemainingEstimateSec *int64   `json:"remainingEstimateSec,omitempty"`
}

// Column statuses are provider status IDs (the board config exposes IDs, not
// names — the frontend matches issues by Issue.StatusID).
type Column struct {
	Name      string   `json:"name"`
	StatusIDs []string `json:"statusIds"`
}

type Sprint struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	BoardID int64  `json:"boardId"`
	// BoardURL is the provider-built deep link to the board, so the frontend
	// never has to know any provider's URL scheme.
	BoardURL string   `json:"boardUrl,omitempty"`
	Columns  []Column `json:"columns"`
	Issues   []Issue  `json:"issues"`
}

type IssueType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateIssueInput is the payload accepted by CreateIssue.
type CreateIssueInput struct {
	Summary     string `json:"summary"`
	IssueTypeID string `json:"issueTypeId"`
}

// User is the authenticated account behind the configured credentials.
type User struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

// IssueDetail is the full payload for the issue-detail modal: extends the
// board-card Issue with description, reporter, and creation timestamp.
type IssueDetail struct {
	Key                  string             `json:"key"`
	Summary              string             `json:"summary"`
	Description          string             `json:"description"`
	IssueType            string             `json:"issueType"`
	Priority             string             `json:"priority"`
	Status               string             `json:"status"`
	StatusCategory       string             `json:"statusCategory"`
	Assignee             string             `json:"assignee"`
	AssigneeEmail        string             `json:"assigneeEmail"`
	Reporter             string             `json:"reporter"`
	ReporterEmail        string             `json:"reporterEmail"`
	Labels               []string           `json:"labels"`
	URL                  string             `json:"url"`
	CreatedAt            int64              `json:"createdAt"`
	UpdatedAt            int64              `json:"updatedAt"`
	OriginalEstimateSec  *int64             `json:"originalEstimateSec,omitempty"`
	RemainingEstimateSec *int64             `json:"remainingEstimateSec,omitempty"`
	CustomFields         []CustomFieldValue `json:"customFields,omitempty"`
}

// Comment is one entry from the issue comment thread, body flattened to plain
// text.
type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// HistoryChange is one field-level change inside a HistoryEntry.
type HistoryChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// HistoryEntry is a single changelog event: who changed what at when.
type HistoryEntry struct {
	ID        string          `json:"id"`
	Author    string          `json:"author"`
	CreatedAt int64           `json:"createdAt"`
	Changes   []HistoryChange `json:"changes"`
}
