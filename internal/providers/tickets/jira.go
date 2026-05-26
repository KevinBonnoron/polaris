// Package tickets provides integrations with issue tracking platforms
// like Jira, Linear, and Azure Boards. We translate each platform's verbose
// schema into small DTOs so the frontend doesn't depend on any single
// platform's full payloads.
package tickets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrMissingConfig is returned when required credentials are absent.
	ErrMissingConfig = errors.New("tickets: missing baseUrl, email, token or projectKey")
	// ErrNoBoard means no agile board was found for the given project.
	ErrNoBoard = errors.New("tickets: no agile board found for project")
	// ErrNoSprint means the project's board has no active sprint.
	ErrNoSprint = errors.New("tickets: no active sprint on board")
)

const apiTimeout = 15 * time.Second

type Config struct {
	BaseURL    string `json:"baseUrl"`
	Email      string `json:"email"`
	Token      string `json:"token"`
	ProjectKey string `json:"projectKey"`
	BoardID    int64  `json:"boardId,omitempty"`
}

type BoardInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Issue struct {
	Key            string   `json:"key"`
	Summary        string   `json:"summary"`
	IssueType      string   `json:"issueType"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	StatusID       string   `json:"statusId"`
	StatusCategory string   `json:"statusCategory"`
	Assignee       string   `json:"assignee"`
	AssigneeEmail  string   `json:"assigneeEmail"`
	Labels         []string `json:"labels"`
	URL            string   `json:"url"`
	UpdatedAt      int64    `json:"updatedAt"`
}

// Column statuses are Jira status IDs (the agile board config only exposes
// IDs, not names — the frontend matches issues by Issue.StatusID).
type Column struct {
	Name       string   `json:"name"`
	StatusIDs  []string `json:"statusIds"`
}

type Sprint struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	BoardID int64    `json:"boardId"`
	Columns []Column `json:"columns"`
	Issues  []Issue  `json:"issues"`
}

// FetchActiveSprint resolves the project's first scrum board, picks its
// active sprint, and returns every issue in that sprint along with the
// board's column-to-status mapping. Falls back to the board itself (no
// sprint) when the board is Kanban-style and ErrNoSprint is hit.
func FetchActiveSprint(cfg Config) (*Sprint, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" || cfg.ProjectKey == "" {
		return nil, ErrMissingConfig
	}
	base := strings.TrimRight(cfg.BaseURL, "/")

	var board *boardInfo
	var err error
	if cfg.BoardID != 0 {
		board, err = boardByID(base, cfg, cfg.BoardID)
	} else {
		board, err = firstBoard(base, cfg, cfg.ProjectKey)
	}
	if err != nil {
		return nil, err
	}

	columns, err := boardColumns(base, cfg, board.ID)
	if err != nil {
		columns = nil
	}

	sprint, err := activeSprint(base, cfg, board.ID)
	if err != nil && !errors.Is(err, ErrNoSprint) {
		return nil, err
	}

	var issues []Issue
	var sprintID int64
	var sprintName string

	if sprint != nil {
		sprintID = sprint.ID
		sprintName = sprint.Name
		issues, err = sprintIssues(base, cfg, sprint.ID)
		if err != nil {
			return nil, err
		}
	} else {
		// Kanban / no active sprint: surface board backlog instead.
		sprintName = board.Name
		issues, err = backlogIssues(base, cfg, board.ID)
		if err != nil {
			return nil, err
		}
	}

	return &Sprint{
		ID:      sprintID,
		Name:    sprintName,
		BoardID: board.ID,
		Columns: columns,
		Issues:  issues,
	}, nil
}

type boardInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ListBoards returns every agile board visible for the project, so the user
// can pick which one the sprint view & automations should target.
func ListBoards(cfg Config) ([]BoardInfo, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" || cfg.ProjectKey == "" {
		return nil, ErrMissingConfig
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	var raw struct {
		Values []boardInfo `json:"values"`
	}
	q := url.Values{}
	q.Set("projectKeyOrId", cfg.ProjectKey)
	if err := getJSON(base+"/rest/agile/1.0/board?"+q.Encode(), cfg, &raw); err != nil {
		return nil, err
	}
	out := make([]BoardInfo, 0, len(raw.Values))
	for _, b := range raw.Values {
		out = append(out, BoardInfo{ID: b.ID, Name: b.Name, Type: b.Type})
	}
	return out, nil
}

func boardByID(base string, cfg Config, boardID int64) (*boardInfo, error) {
	var raw boardInfo
	if err := getJSON(fmt.Sprintf("%s/rest/agile/1.0/board/%d", base, boardID), cfg, &raw); err != nil {
		return nil, err
	}
	if raw.ID == 0 {
		return nil, ErrNoBoard
	}
	return &raw, nil
}

func firstBoard(base string, cfg Config, projectKey string) (*boardInfo, error) {
	var raw struct {
		Values []boardInfo `json:"values"`
	}
	q := url.Values{}
	q.Set("projectKeyOrId", projectKey)
	if err := getJSON(base+"/rest/agile/1.0/board?"+q.Encode(), cfg, &raw); err != nil {
		return nil, err
	}
	if len(raw.Values) == 0 {
		return nil, ErrNoBoard
	}
	for i := range raw.Values {
		if raw.Values[i].Type == "scrum" {
			return &raw.Values[i], nil
		}
	}
	return &raw.Values[0], nil
}

func boardColumns(base string, cfg Config, boardID int64) ([]Column, error) {
	var raw struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	if err := getJSON(fmt.Sprintf("%s/rest/agile/1.0/board/%d/configuration", base, boardID), cfg, &raw); err != nil {
		return nil, err
	}
	out := make([]Column, 0, len(raw.ColumnConfig.Columns))
	for _, c := range raw.ColumnConfig.Columns {
		ids := make([]string, 0, len(c.Statuses))
		for _, s := range c.Statuses {
			ids = append(ids, s.ID)
		}
		out = append(out, Column{Name: c.Name, StatusIDs: ids})
	}
	return out, nil
}

type sprintInfo struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func activeSprint(base string, cfg Config, boardID int64) (*sprintInfo, error) {
	var raw struct {
		Values []sprintInfo `json:"values"`
	}
	if err := getJSON(fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active", base, boardID), cfg, &raw); err != nil {
		// Kanban boards return 400 for /sprint queries; treat as no sprint.
		if isClientError(err) {
			return nil, ErrNoSprint
		}
		return nil, err
	}
	if len(raw.Values) == 0 {
		return nil, ErrNoSprint
	}
	return &raw.Values[0], nil
}

func sprintIssues(base string, cfg Config, sprintID int64) ([]Issue, error) {
	q := url.Values{}
	q.Set("maxResults", "100")
	q.Set("fields", "summary,status,priority,issuetype,assignee,labels,updated")
	return fetchIssues(fmt.Sprintf("%s/rest/agile/1.0/sprint/%d/issue?%s", base, sprintID, q.Encode()), cfg, base)
}

func backlogIssues(base string, cfg Config, boardID int64) ([]Issue, error) {
	q := url.Values{}
	q.Set("maxResults", "100")
	q.Set("fields", "summary,status,priority,issuetype,assignee,labels,updated")
	return fetchIssues(fmt.Sprintf("%s/rest/agile/1.0/board/%d/issue?%s", base, boardID, q.Encode()), cfg, base)
}

func fetchIssues(endpoint string, cfg Config, base string) ([]Issue, error) {
	var raw struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary   string `json:"summary"`
				Updated   string `json:"updated"`
				Labels    []string `json:"labels"`
				IssueType struct {
					Name string `json:"name"`
				} `json:"issuetype"`
				Priority struct {
					Name string `json:"name"`
				} `json:"priority"`
				Status struct {
					ID             string `json:"id"`
					Name           string `json:"name"`
					StatusCategory struct {
						Key string `json:"key"`
					} `json:"statusCategory"`
				} `json:"status"`
				Assignee struct {
					DisplayName  string `json:"displayName"`
					EmailAddress string `json:"emailAddress"`
				} `json:"assignee"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := getJSON(endpoint, cfg, &raw); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(raw.Issues))
	for _, i := range raw.Issues {
		updated := parseJiraTime(i.Fields.Updated)
		out = append(out, Issue{
			Key:            i.Key,
			Summary:        i.Fields.Summary,
			IssueType:      i.Fields.IssueType.Name,
			Priority:       i.Fields.Priority.Name,
			Status:         i.Fields.Status.Name,
			StatusID:       i.Fields.Status.ID,
			StatusCategory: i.Fields.Status.StatusCategory.Key,
			Assignee:       i.Fields.Assignee.DisplayName,
			AssigneeEmail:  i.Fields.Assignee.EmailAddress,
			Labels:         i.Fields.Labels,
			URL:            fmt.Sprintf("%s/browse/%s", base, i.Key),
			UpdatedAt:      unixOrZero(updated),
		})
	}
	return out, nil
}

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("tickets: HTTP %d: %s", e.status, e.body)
}

func isClientError(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.status >= 400 && he.status < 500
	}
	return false
}

// IssueType is one of a project's allowed issue types (Task, Bug, ...).
type IssueType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListProjectIssueTypes returns the issue types available for the configured
// project. Used to populate the "Create issue" modal.
func ListProjectIssueTypes(cfg Config) ([]IssueType, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" || cfg.ProjectKey == "" {
		return nil, ErrMissingConfig
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	var raw struct {
		Projects []struct {
			IssueTypes []IssueType `json:"issuetypes"`
		} `json:"projects"`
	}
	q := url.Values{}
	q.Set("projectKeys", cfg.ProjectKey)
	if err := getJSON(base+"/rest/api/3/issue/createmeta?"+q.Encode(), cfg, &raw); err != nil {
		return nil, err
	}
	if len(raw.Projects) == 0 {
		return nil, nil
	}
	return raw.Projects[0].IssueTypes, nil
}

// CreateIssueInput is the payload accepted by CreateIssue.
type CreateIssueInput struct {
	Summary     string `json:"summary"`
	IssueTypeID string `json:"issueTypeId"`
}

// CreateIssue creates a new issue in the configured project and returns its
// key (e.g. AUTH-128).
func CreateIssue(cfg Config, in CreateIssueInput) (string, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" || cfg.ProjectKey == "" {
		return "", ErrMissingConfig
	}
	if strings.TrimSpace(in.Summary) == "" {
		return "", errors.New("tickets: summary is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	payload := map[string]any{
		"fields": map[string]any{
			"project":   map[string]string{"key": cfg.ProjectKey},
			"summary":   in.Summary,
			"issuetype": map[string]string{"id": in.IssueTypeID},
		},
	}
	var resp struct {
		Key string `json:"key"`
	}
	if err := sendJSON(http.MethodPost, base+"/rest/api/3/issue", cfg, payload, &resp); err != nil {
		return "", err
	}
	return resp.Key, nil
}

// TransitionIssue moves an issue toward any of targetStatusIDs (typically all
// statuses mapped to the dropped board column). Picks the first workflow
// transition whose destination status matches one of the targets. Returns
// ErrNoTransition when no available transition lands on any of those statuses.
func TransitionIssue(cfg Config, issueKey string, targetStatusIDs []string) error {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" {
		return ErrMissingConfig
	}
	if issueKey == "" || len(targetStatusIDs) == 0 {
		return errors.New("tickets: issueKey and targetStatusIDs are required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")

	wanted := make(map[string]struct{}, len(targetStatusIDs))
	for _, id := range targetStatusIDs {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return errors.New("tickets: targetStatusIDs are empty")
	}

	var raw struct {
		Transitions []struct {
			ID string `json:"id"`
			To struct {
				ID string `json:"id"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := getJSON(fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", base, issueKey), cfg, &raw); err != nil {
		return err
	}
	var transitionID string
	for _, tr := range raw.Transitions {
		if _, ok := wanted[tr.To.ID]; ok {
			transitionID = tr.ID
			break
		}
	}
	if transitionID == "" {
		return ErrNoTransition
	}
	payload := map[string]any{
		"transition": map[string]string{"id": transitionID},
	}
	return sendJSON(http.MethodPost, fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", base, issueKey), cfg, payload, nil)
}

// ErrNoTransition is returned when the target status isn't reachable from
// the issue's current state via any available workflow transition.
var ErrNoTransition = errors.New("tickets: no available transition to that status")

// FetchLastComment returns the plain-text body of the most recent comment on
// an issue, or "" if there are none. Used by the automation manager to feed
// the {{lastComment}} placeholder when an agent gets spawned on a QA bounce.
func FetchLastComment(cfg Config, issueKey string) (string, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" {
		return "", ErrMissingConfig
	}
	if issueKey == "" {
		return "", errors.New("tickets: issueKey is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	// orderBy=created and the simple `expand=renderedBody` keep the payload small.
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/comment?orderBy=-created&maxResults=1", base, issueKey)
	var raw struct {
		Comments []struct {
			Body any `json:"body"`
		} `json:"comments"`
	}
	if err := getJSON(endpoint, cfg, &raw); err != nil {
		return "", err
	}
	if len(raw.Comments) == 0 {
		return "", nil
	}
	return adfToPlainText(raw.Comments[0].Body), nil
}

// IssueDetail is the full payload for the issue-detail modal: extends the
// board-card Issue with description, reporter, and creation timestamp.
type IssueDetail struct {
	Key            string   `json:"key"`
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	IssueType      string   `json:"issueType"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	StatusCategory string   `json:"statusCategory"`
	Assignee       string   `json:"assignee"`
	AssigneeEmail  string   `json:"assigneeEmail"`
	Reporter       string   `json:"reporter"`
	ReporterEmail  string   `json:"reporterEmail"`
	Labels         []string `json:"labels"`
	URL            string   `json:"url"`
	CreatedAt      int64    `json:"createdAt"`
	UpdatedAt      int64    `json:"updatedAt"`
}

// Comment is one entry from the issue comment thread, with ADF body flattened
// to plain text.
type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// FetchIssueDetail returns the full detail for one issue.
func FetchIssueDetail(cfg Config, issueKey string) (*IssueDetail, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, ErrMissingConfig
	}
	if issueKey == "" {
		return nil, errors.New("tickets: issueKey is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s", base, issueKey)
	var raw struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string   `json:"summary"`
			Description any      `json:"description"`
			Created     string   `json:"created"`
			Updated     string   `json:"updated"`
			Labels      []string `json:"labels"`
			IssueType   struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Priority struct {
				Name string `json:"name"`
			} `json:"priority"`
			Status struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			} `json:"status"`
			Assignee struct {
				DisplayName  string `json:"displayName"`
				EmailAddress string `json:"emailAddress"`
			} `json:"assignee"`
			Reporter struct {
				DisplayName  string `json:"displayName"`
				EmailAddress string `json:"emailAddress"`
			} `json:"reporter"`
		} `json:"fields"`
	}
	if err := getJSON(endpoint, cfg, &raw); err != nil {
		return nil, err
	}
	created := parseJiraTime(raw.Fields.Created)
	updated := parseJiraTime(raw.Fields.Updated)
	labels := raw.Fields.Labels
	if labels == nil {
		labels = []string{}
	}
	return &IssueDetail{
		Key:            raw.Key,
		Summary:        raw.Fields.Summary,
		Description:    adfToMarkdown(raw.Fields.Description),
		IssueType:      raw.Fields.IssueType.Name,
		Priority:       raw.Fields.Priority.Name,
		Status:         raw.Fields.Status.Name,
		StatusCategory: raw.Fields.Status.StatusCategory.Key,
		Assignee:       raw.Fields.Assignee.DisplayName,
		AssigneeEmail:  raw.Fields.Assignee.EmailAddress,
		Reporter:       raw.Fields.Reporter.DisplayName,
		ReporterEmail:  raw.Fields.Reporter.EmailAddress,
		Labels:         labels,
		URL:            fmt.Sprintf("%s/browse/%s", base, raw.Key),
		CreatedAt:      unixOrZero(created),
		UpdatedAt:      unixOrZero(updated),
	}, nil
}

// ListIssueComments returns the issue comment thread (oldest to newest), with
// ADF bodies converted to Markdown.
func ListIssueComments(cfg Config, issueKey string) ([]Comment, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, ErrMissingConfig
	}
	if issueKey == "" {
		return nil, errors.New("tickets: issueKey is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/comment?orderBy=created&maxResults=100", base, issueKey)
	var raw struct {
		Comments []struct {
			ID     string `json:"id"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body    any    `json:"body"`
			Created string `json:"created"`
			Updated string `json:"updated"`
		} `json:"comments"`
	}
	if err := getJSON(endpoint, cfg, &raw); err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(raw.Comments))
	for _, c := range raw.Comments {
		created := parseJiraTime(c.Created)
		updated := parseJiraTime(c.Updated)
		out = append(out, Comment{
			ID:        c.ID,
			Author:    c.Author.DisplayName,
			Body:      adfToMarkdown(c.Body),
			CreatedAt: unixOrZero(created),
			UpdatedAt: unixOrZero(updated),
		})
	}
	return out, nil
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

// ListIssueHistory returns the issue changelog (newest first), one entry per
// changeset, with per-field from/to strings.
func ListIssueHistory(cfg Config, issueKey string) ([]HistoryEntry, error) {
	if cfg.BaseURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, ErrMissingConfig
	}
	if issueKey == "" {
		return nil, errors.New("tickets: issueKey is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/changelog?maxResults=100", base, issueKey)
	var raw struct {
		Values []struct {
			ID     string `json:"id"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Created string `json:"created"`
			Items   []struct {
				Field      string `json:"field"`
				FromString string `json:"fromString"`
				ToString   string `json:"toString"`
			} `json:"items"`
		} `json:"values"`
	}
	if err := getJSON(endpoint, cfg, &raw); err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, 0, len(raw.Values))
	for _, v := range raw.Values {
		created := parseJiraTime(v.Created)
		changes := make([]HistoryChange, 0, len(v.Items))
		for _, it := range v.Items {
			changes = append(changes, HistoryChange{
				Field: it.Field,
				From:  it.FromString,
				To:    it.ToString,
			})
		}
		out = append(out, HistoryEntry{
			ID:        v.ID,
			Author:    v.Author.DisplayName,
			CreatedAt: unixOrZero(created),
			Changes:   changes,
		})
	}
	// Newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// adfToPlainText walks a (possibly nested) Atlassian Document Format payload
// and concatenates every "text" leaf with paragraph separators. Returns "" for
// unknown / nil shapes — callers should treat empty as "no extractable text".
func adfToPlainText(node any) string {
	var b strings.Builder
	var walk func(any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		if t, _ := m["type"].(string); t == "text" {
			if s, _ := m["text"].(string); s != "" {
				b.WriteString(s)
			}
			return
		}
		if content, ok := m["content"].([]any); ok {
			for _, c := range content {
				walk(c)
			}
			if t, _ := m["type"].(string); t == "paragraph" {
				b.WriteString("\n")
			}
		}
	}
	walk(node)
	return strings.TrimSpace(b.String())
}

// Jira REST returns timestamps like "2024-01-15T10:30:00.000+0000" — the
// timezone has no colon, so time.RFC3339 can't parse it. parseJiraTime tries
// a small list of common Jira layouts and returns the zero time on failure.
func parseJiraTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// unixOrZero returns t.Unix() or 0 when t is the zero time, so the frontend
// can treat 0 as "unknown" rather than receive the garbage epoch value.
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// adfToMarkdown converts an Atlassian Document Format tree into a best-effort
// Markdown string. Handles paragraphs, headings, lists, code blocks, blockquotes,
// hard breaks, rules, and the common text marks (strong, em, code, strike, link).
// Unknown node types fall through to their text content.
func adfToMarkdown(node any) string {
	var b strings.Builder
	var walk func(n any, depth int)

	renderMarks := func(text string, marks []any) string {
		out := text
		for _, mark := range marks {
			m, ok := mark.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := m["type"].(string); t {
			case "strong":
				out = "**" + out + "**"
			case "em":
				out = "*" + out + "*"
			case "code":
				out = "`" + out + "`"
			case "strike":
				out = "~~" + out + "~~"
			case "link":
				attrs, _ := m["attrs"].(map[string]any)
				href, _ := attrs["href"].(string)
				if href != "" {
					out = "[" + out + "](" + href + ")"
				}
			}
		}
		return out
	}

	var childrenString func(content []any) string
	childrenString = func(content []any) string {
		var sb strings.Builder
		for _, c := range content {
			m, ok := c.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := m["type"].(string); t {
			case "text":
				s, _ := m["text"].(string)
				marks, _ := m["marks"].([]any)
				sb.WriteString(renderMarks(s, marks))
			case "hardBreak":
				sb.WriteString("  \n")
			default:
				// inline-but-unknown: walk into it so we don't lose nested text
				if nested, ok := m["content"].([]any); ok {
					sb.WriteString(childrenString(nested))
				}
			}
		}
		return sb.String()
	}

	walk = func(n any, depth int) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		nodeType, _ := m["type"].(string)
		content, _ := m["content"].([]any)

		switch nodeType {
		case "doc":
			for _, c := range content {
				walk(c, depth)
			}
		case "paragraph":
			b.WriteString(childrenString(content))
			b.WriteString("\n\n")
		case "heading":
			attrs, _ := m["attrs"].(map[string]any)
			level := 1
			if lv, ok := attrs["level"].(float64); ok {
				level = int(lv)
			}
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			b.WriteString(strings.Repeat("#", level))
			b.WriteString(" ")
			b.WriteString(childrenString(content))
			b.WriteString("\n\n")
		case "bulletList":
			for _, c := range content {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				b.WriteString(strings.Repeat("  ", depth))
				b.WriteString("- ")
				inner, _ := cm["content"].([]any)
				renderListItem(&b, inner, depth, walk, childrenString)
			}
			b.WriteString("\n")
		case "orderedList":
			for i, c := range content {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				b.WriteString(strings.Repeat("  ", depth))
				fmt.Fprintf(&b, "%d. ", i+1)
				inner, _ := cm["content"].([]any)
				renderListItem(&b, inner, depth, walk, childrenString)
			}
			b.WriteString("\n")
		case "codeBlock":
			attrs, _ := m["attrs"].(map[string]any)
			lang, _ := attrs["language"].(string)
			b.WriteString("```")
			b.WriteString(lang)
			b.WriteString("\n")
			b.WriteString(plainChildren(content))
			b.WriteString("\n```\n\n")
		case "blockquote":
			var inner strings.Builder
			for _, c := range content {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := cm["type"].(string); t == "paragraph" {
					if cc, ok := cm["content"].([]any); ok {
						inner.WriteString(childrenString(cc))
						inner.WriteString("\n")
					}
				}
			}
			for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
				b.WriteString("> ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		case "rule":
			b.WriteString("---\n\n")
		default:
			// unknown block: render children inline best-effort
			if len(content) > 0 {
				b.WriteString(childrenString(content))
				b.WriteString("\n")
			}
		}
	}

	walk(node, 0)
	return strings.TrimSpace(b.String())
}

// plainChildren extracts only the raw text from a content array (used for code
// blocks where marks should NOT be applied).
func plainChildren(content []any) string {
	var sb strings.Builder
	for _, c := range content {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "text" {
			s, _ := m["text"].(string)
			sb.WriteString(s)
		} else if nested, ok := m["content"].([]any); ok {
			sb.WriteString(plainChildren(nested))
		}
	}
	return sb.String()
}

// renderListItem handles a single list item's block content: first paragraph
// goes inline after the bullet, nested lists indent under it.
func renderListItem(b *strings.Builder, content []any, depth int, walk func(any, int), childrenString func([]any) string) {
	for i, c := range content {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := cm["type"].(string)
		if i == 0 && t == "paragraph" {
			inner, _ := cm["content"].([]any)
			b.WriteString(childrenString(inner))
			b.WriteString("\n")
			continue
		}
		if t == "bulletList" || t == "orderedList" {
			walk(cm, depth+1)
			continue
		}
		// fallback: render block
		walk(cm, depth+1)
	}
}

func getJSON(endpoint string, cfg Config, out any) error {
	return doRequest(http.MethodGet, endpoint, cfg, nil, out)
}

func sendJSON(method, endpoint string, cfg Config, body any, out any) error {
	return doRequest(method, endpoint, cfg, body, out)
}

func doRequest(method, endpoint string, cfg Config, body any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return err
	}
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.Email + ":" + cfg.Token))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if len(msg) > 240 {
			msg = msg[:240] + "..."
		}
		return &httpError{status: resp.StatusCode, body: msg}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
