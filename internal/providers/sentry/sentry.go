// Package sentry fetches issue data from Sentry's REST API using a bearer
// auth token. We map Sentry's verbose issue payload into small DTOs so the
// frontend doesn't depend on the full upstream schema.
package sentry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrMissingConfig is returned when required credentials are absent.
var ErrMissingConfig = errors.New("sentry: missing token, org or project")

const (
	apiTimeout = 15 * time.Second
	defaultURL = "https://sentry.io"
)

type Config struct {
	Token   string `json:"token"`
	Org     string `json:"org"`
	Project string `json:"project"`
	URL     string `json:"url,omitempty"`
}

type Issue struct {
	ID        string `json:"id"`
	ShortID   string `json:"shortId"`
	Project   string `json:"project"`
	Title     string `json:"title"`
	Culprit   string `json:"culprit"`
	Level     string `json:"level"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Count     int64  `json:"count"`
	UserCount int64  `json:"userCount"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	Permalink string `json:"permalink"`
}

type rawIssue struct {
	ID        string `json:"id"`
	ShortID   string `json:"shortId"`
	Title     string `json:"title"`
	Culprit   string `json:"culprit"`
	Level     string `json:"level"`
	Status    string `json:"status"`
	Count     string `json:"count"`
	UserCount int64  `json:"userCount"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	Permalink string `json:"permalink"`
	Metadata  struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"metadata"`
}

// FetchIssues lists the project's unresolved issues, most recently seen first.
func FetchIssues(cfg Config) ([]Issue, error) {
	if cfg.Token == "" || cfg.Org == "" || cfg.Project == "" {
		return nil, ErrMissingConfig
	}
	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = defaultURL
	}

	endpoint := fmt.Sprintf("%s/api/0/projects/%s/%s/issues/", base, url.PathEscape(cfg.Org), url.PathEscape(cfg.Project))
	q := url.Values{}
	q.Set("query", "is:unresolved")
	q.Set("statsPeriod", "14d")
	q.Set("limit", "100")

	req, err := http.NewRequest(http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sentry: %s — %s", resp.Status, string(raw))
	}

	var raws []rawIssue
	if err := json.NewDecoder(resp.Body).Decode(&raws); err != nil {
		return nil, err
	}

	issues := make([]Issue, 0, len(raws))
	for _, r := range raws {
		issues = append(issues, Issue{
			ID:        r.ID,
			ShortID:   r.ShortID,
			Project:   cfg.Project,
			Title:     r.Title,
			Culprit:   r.Culprit,
			Level:     r.Level,
			Status:    r.Status,
			Type:      r.Metadata.Type,
			Value:     r.Metadata.Value,
			Count:     parseInt(r.Count),
			UserCount: r.UserCount,
			FirstSeen: parseTime(r.FirstSeen),
			LastSeen:  parseTime(r.LastSeen),
			Permalink: r.Permalink,
		})
	}
	return issues, nil
}

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Frame struct {
	Filename string `json:"filename"`
	Function string `json:"function"`
	LineNo   int    `json:"lineNo"`
	InApp    bool   `json:"inApp"`
}

type Exception struct {
	Type   string  `json:"type"`
	Value  string  `json:"value"`
	Frames []Frame `json:"frames"`
}

type EventDetail struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Culprit     string      `json:"culprit"`
	Message     string      `json:"message"`
	DateCreated int64       `json:"dateCreated"`
	Tags        []Tag       `json:"tags"`
	Exceptions  []Exception `json:"exceptions"`
}

type rawEvent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Culprit     string `json:"culprit"`
	Message     string `json:"message"`
	DateCreated string `json:"dateCreated"`
	Tags        []Tag  `json:"tags"`
	Entries     []struct {
		Type string `json:"type"`
		Data struct {
			Values []struct {
				Type       string `json:"type"`
				Value      string `json:"value"`
				Stacktrace struct {
					Frames []struct {
						Filename string `json:"filename"`
						Function string `json:"function"`
						LineNo   int    `json:"lineNo"`
						InApp    bool   `json:"inApp"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"data"`
	} `json:"entries"`
}

// FetchLatestEvent returns the most recent event for an issue, with its
// exception chain and tags, so the UI can show details without leaving Polaris.
func FetchLatestEvent(cfg Config, issueID string) (*EventDetail, error) {
	if cfg.Token == "" || issueID == "" || cfg.Org == "" {
		return nil, ErrMissingConfig
	}
	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = defaultURL
	}

	endpoint := fmt.Sprintf("%s/api/0/organizations/%s/issues/%s/events/latest/", base, url.PathEscape(cfg.Org), url.PathEscape(issueID))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sentry: %s — %s", resp.Status, string(raw))
	}

	var r rawEvent
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}

	detail := &EventDetail{
		ID:          r.ID,
		Title:       r.Title,
		Culprit:     r.Culprit,
		Message:     r.Message,
		DateCreated: parseTime(r.DateCreated),
		Tags:        r.Tags,
		Exceptions:  []Exception{},
	}
	for _, entry := range r.Entries {
		if entry.Type != "exception" {
			continue
		}
		for _, v := range entry.Data.Values {
			ex := Exception{Type: v.Type, Value: v.Value, Frames: []Frame{}}
			for _, f := range v.Stacktrace.Frames {
				ex.Frames = append(ex.Frames, Frame{Filename: f.Filename, Function: f.Function, LineNo: f.LineNo, InApp: f.InApp})
			}
			detail.Exceptions = append(detail.Exceptions, ex)
		}
	}
	return detail, nil
}

// UpdateIssueStatus changes an issue's status (e.g. "resolved", "ignored",
// "unresolved") so issues can be triaged without leaving Polaris.
func UpdateIssueStatus(cfg Config, issueID, status string) error {
	if cfg.Token == "" || cfg.Org == "" || issueID == "" || status == "" {
		return ErrMissingConfig
	}
	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = defaultURL
	}

	endpoint := fmt.Sprintf("%s/api/0/organizations/%s/issues/%s/", base, url.PathEscape(cfg.Org), url.PathEscape(issueID))
	body := strings.NewReader(fmt.Sprintf(`{"status":%q}`, status))
	req, err := http.NewRequest(http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sentry: %s — %s", resp.Status, string(raw))
	}
	return nil
}

func parseInt(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func parseTime(s string) int64 {
	if s == "" {
		return 0
	}
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return tm.UnixMilli()
}
