// Package ticketsstore caches tickets sprint snapshots keyed by the tuple
// (baseUrl, projectKey, boardId). Multiple polaris projects pointing at the
// same board share a single poll loop (see internal/store/pollstore).
package ticketsstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/tickets"
	"github.com/KevinBonnoron/polaris/internal/store/pollstore"
)

type Key struct {
	BaseURL    string
	ProjectKey string
	BoardID    int64
}

func (k Key) String() string {
	return fmt.Sprintf("%s|%s|%d", k.BaseURL, k.ProjectKey, k.BoardID)
}

type Snapshot struct {
	Sprint    *tickets.Sprint
	FetchedAt time.Time
	Err       error
}

type Diff struct {
	Key    Key
	Before Snapshot
	After  Snapshot

	IssuesTransitioned []IssueTransition
	IssuesReassigned   []IssueReassignment
	IssuesAdded        []tickets.Issue
}

// IssueTransition captures a status change between two ticks. From may be
// empty when the issue wasn't seen on the previous tick.
type IssueTransition struct {
	Issue tickets.Issue
	From  string // status id
	To    string // status id
}

type IssueReassignment struct {
	Issue tickets.Issue
	From  string
	To    string
}

type Subscriber func(Diff)

type Persistence interface {
	Load() (map[string]Snapshot, error)
	Save(k Key, snap Snapshot) error
}

type driver struct {
	persist Persistence
}

func (driver) KeyString(k Key) string { return k.String() }

// HasData treats a failed fetch as cold so Get re-fetches instead of serving an
// error snapshot as cache.
func (driver) HasData(s Snapshot) bool { return !s.FetchedAt.IsZero() && s.Err == nil }

func (d driver) Persist(k Key, snap Snapshot) error {
	// Never overwrite the last good cache with a failed fetch.
	if snap.Err != nil || d.persist == nil {
		return nil
	}
	return d.persist.Save(k, snap)
}

func (driver) Refresh(k Key, cfg tickets.Config, before Snapshot) (Snapshot, Diff, error) {
	sprint, err := tickets.FetchActiveSprint(cfg)
	after := Snapshot{Sprint: sprint, FetchedAt: time.Now(), Err: err}
	return after, Diff{
		Key:                k,
		Before:             before,
		After:              after,
		IssuesTransitioned: transitions(before, after),
		IssuesReassigned:   reassignments(before, after),
		IssuesAdded:        addedIssues(before, after),
	}, nil
}

type Store struct {
	*pollstore.Store[Key, tickets.Config, Snapshot, Diff]
}

func New(persist Persistence) *Store {
	var preloaded map[string]Snapshot
	if persist != nil {
		if loaded, err := persist.Load(); err == nil {
			preloaded = loaded
		}
	}
	return &Store{pollstore.New[Key, tickets.Config, Snapshot, Diff]("ticketsstore", driver{persist: persist}, preloaded)}
}

// GetSnapshot returns the cached sprint snapshot, fetching inline if cold.
func (s *Store) GetSnapshot(ctx context.Context, k Key, cfg tickets.Config) (Snapshot, error) {
	return s.Get(ctx, k, cfg)
}

func issueMap(snap Snapshot) map[string]tickets.Issue {
	out := map[string]tickets.Issue{}
	if snap.Sprint == nil {
		return out
	}
	for _, i := range snap.Sprint.Issues {
		out[i.Key] = i
	}
	return out
}

func transitions(before, after Snapshot) []IssueTransition {
	prev := issueMap(before)
	var out []IssueTransition
	if after.Sprint == nil {
		return out
	}
	for _, cur := range after.Sprint.Issues {
		old, ok := prev[cur.Key]
		if !ok {
			out = append(out, IssueTransition{Issue: cur, From: "", To: cur.StatusID})
			continue
		}
		if old.StatusID != cur.StatusID {
			out = append(out, IssueTransition{Issue: cur, From: old.StatusID, To: cur.StatusID})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Issue.Key < out[b].Issue.Key })
	return out
}

func reassignments(before, after Snapshot) []IssueReassignment {
	prev := issueMap(before)
	var out []IssueReassignment
	if after.Sprint == nil {
		return out
	}
	for _, cur := range after.Sprint.Issues {
		old, ok := prev[cur.Key]
		if !ok {
			continue
		}
		oldKey := assigneeKey(old)
		newKey := assigneeKey(cur)
		if oldKey != newKey {
			out = append(out, IssueReassignment{Issue: cur, From: oldKey, To: newKey})
		}
	}
	return out
}

func addedIssues(before, after Snapshot) []tickets.Issue {
	prev := issueMap(before)
	var out []tickets.Issue
	if after.Sprint == nil {
		return out
	}
	for _, cur := range after.Sprint.Issues {
		if _, ok := prev[cur.Key]; !ok {
			out = append(out, cur)
		}
	}
	return out
}

func assigneeKey(i tickets.Issue) string {
	if i.AssigneeEmail != "" {
		return i.AssigneeEmail
	}
	return i.Assignee
}
