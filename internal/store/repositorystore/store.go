// Package repositorystore is the single source of truth for repository hosting
// data (pull requests, issues, recent workflow runs). All callers — the Wails
// HTTP-style endpoints feeding the UI and the AutomationManager — read through
// this store instead of calling the repository provider directly, so a single
// poll services every consumer of a given (owner, repo). The poll/diff engine
// is shared with the other provider caches (see internal/store/pollstore).
package repositorystore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/repository"
	"github.com/KevinBonnoron/polaris/internal/store/pollstore"
)

// Snapshot is the cached view of one repo at a point in time. Empty slices
// mean "fetched, nothing found"; a zero FetchedAt means "never fetched".
type Snapshot struct {
	PRs       []repository.PullRequest
	Issues    []repository.Issue
	Runs      []repository.WorkflowRun
	FetchedAt time.Time
	// PartialErr is set when one of the three sub-fetches failed while the
	// others succeeded. Callers can still use whichever slices are populated.
	PartialErr error
}

// Diff is what subscribers receive after a refresh. Slices contain the items
// that changed between Before and After.
type Diff struct {
	Owner, Repo string
	Before      Snapshot
	After       Snapshot

	PRsOpened      []repository.PullRequest
	PRsClosed      []repository.PullRequest
	PRsUpdated     []repository.PullRequest
	IssuesAdded    []repository.Issue
	IssuesAssigned []IssueAssignment
	RunsAdded      []repository.WorkflowRun
	// RunsCompleted is the subset of runs whose status just transitioned to
	// "completed" between Before and After. Useful for triggers that care about
	// terminal CI outcomes (pr_build_failed, pr_build_success).
	RunsCompleted []repository.WorkflowRun
}

// IssueAssignment records that Assignee newly appears on Issue (was not in the
// previous snapshot's assignees list).
type IssueAssignment struct {
	Issue    repository.Issue
	Assignee string
}

// Subscriber receives every diff after a successful refresh. Implementations
// must not block — the store calls them on its own refresh goroutine.
type Subscriber func(Diff)

// Persistence persists snapshots across process restarts. Nil persistence is
// fine: the store works in-memory only.
type Persistence interface {
	Load() (map[string]Snapshot, error)
	Save(owner, repo string, snap Snapshot) error
}

// key identifies a watched repository. There are no per-key credentials: the
// repository provider reads them from the environment, so the pollstore config
// type is empty.
type key struct {
	Owner, Repo string
}

type driver struct {
	persist Persistence
}

func (driver) KeyString(k key) string  { return k.Owner + "/" + k.Repo }
func (driver) HasData(s Snapshot) bool { return !s.FetchedAt.IsZero() }

func (d driver) Persist(k key, snap Snapshot) error {
	if d.persist != nil {
		return d.persist.Save(k.Owner, k.Repo, snap)
	}
	return nil
}

func (driver) Refresh(k key, _ struct{}, before Snapshot) (Snapshot, Diff, error) {
	after, err := fetchAll(k.Owner, k.Repo)
	if err != nil && after.FetchedAt.IsZero() {
		return Snapshot{}, Diff{}, err
	}
	return after, Diff{
		Owner:          k.Owner,
		Repo:           k.Repo,
		Before:         before,
		After:          after,
		PRsOpened:      newPRs(before.PRs, after.PRs),
		PRsClosed:      removedPRs(before.PRs, after.PRs),
		PRsUpdated:     updatedPRs(before.PRs, after.PRs),
		IssuesAdded:    newIssues(before.Issues, after.Issues),
		IssuesAssigned: newAssignments(before.Issues, after.Issues),
		RunsAdded:      newRuns(before.Runs, after.Runs),
		RunsCompleted:  completedRuns(before.Runs, after.Runs),
	}, nil
}

type Store struct {
	*pollstore.Store[key, struct{}, Snapshot, Diff]
}

func New(persist Persistence) *Store {
	var preloaded map[string]Snapshot
	if persist != nil {
		if loaded, err := persist.Load(); err == nil {
			preloaded = loaded
		}
	}
	return &Store{pollstore.New[key, struct{}, Snapshot, Diff]("repositorystore", driver{persist: persist}, preloaded)}
}

// GetSnapshot returns the cached snapshot, fetching inline if cold.
func (s *Store) GetSnapshot(ctx context.Context, owner, repo string) (Snapshot, error) {
	return s.Get(ctx, key{owner, repo}, struct{}{})
}

// GetPRs is sugar over GetSnapshot for callers that only need PRs.
func (s *Store) GetPRs(ctx context.Context, owner, repo string) ([]repository.PullRequest, error) {
	snap, err := s.GetSnapshot(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return snap.PRs, nil
}

func (s *Store) GetIssues(ctx context.Context, owner, repo string) ([]repository.Issue, error) {
	snap, err := s.GetSnapshot(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return snap.Issues, nil
}

func (s *Store) GetRuns(ctx context.Context, owner, repo string) ([]repository.WorkflowRun, error) {
	snap, err := s.GetSnapshot(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return snap.Runs, nil
}

// Refresh forces a fresh fetch and emits a diff to subscribers.
func (s *Store) Refresh(ctx context.Context, owner, repo string) error {
	k := key{owner, repo}
	s.SetConfig(k, struct{}{})
	return s.Store.Refresh(ctx, k)
}

// Register signals that refKey wants this repo polled at interval.
func (s *Store) Register(ctx context.Context, owner, repo, refKey string, interval time.Duration) {
	s.Store.Register(ctx, key{owner, repo}, struct{}{}, refKey, interval)
}

func (s *Store) Unregister(owner, repo, refKey string) {
	s.Store.Unregister(key{owner, repo}, refKey)
}

// fetchAll calls the three repository endpoints in parallel and bundles the
// result into a single Snapshot. A failure on one of the three doesn't kill the
// snapshot — the field stays nil and PartialErr carries the cause. Only a total
// failure (all three) returns an error.
func fetchAll(owner, repo string) (Snapshot, error) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		snap Snapshot
	)
	now := time.Now()

	wg.Add(3)
	go func() {
		defer wg.Done()
		prs, err := repository.ListPullRequests(owner, repo)
		mu.Lock()
		if err != nil {
			errs = append(errs, fmt.Errorf("prs: %w", err))
		} else {
			snap.PRs = prs
		}
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		issues, err := repository.ListIssues(owner, repo)
		mu.Lock()
		if err != nil {
			errs = append(errs, fmt.Errorf("issues: %w", err))
		} else {
			snap.Issues = issues
		}
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		page, err := repository.ListWorkflowRuns(owner, repo, 1)
		mu.Lock()
		if err != nil {
			errs = append(errs, fmt.Errorf("runs: %w", err))
		} else if page != nil {
			snap.Runs = page.Runs
		}
		mu.Unlock()
	}()
	wg.Wait()

	snap.FetchedAt = now
	if len(errs) == 3 {
		return Snapshot{}, errors.Join(errs...)
	}
	if len(errs) > 0 {
		snap.PartialErr = errors.Join(errs...)
	}
	return snap, nil
}

func newPRs(before, after []repository.PullRequest) []repository.PullRequest {
	seen := map[int]struct{}{}
	for _, p := range before {
		seen[p.Number] = struct{}{}
	}
	var out []repository.PullRequest
	for _, p := range after {
		if _, ok := seen[p.Number]; !ok {
			out = append(out, p)
		}
	}
	return out
}

func removedPRs(before, after []repository.PullRequest) []repository.PullRequest {
	seen := map[int]struct{}{}
	for _, p := range after {
		seen[p.Number] = struct{}{}
	}
	var out []repository.PullRequest
	for _, p := range before {
		if _, ok := seen[p.Number]; !ok {
			out = append(out, p)
		}
	}
	return out
}

func updatedPRs(before, after []repository.PullRequest) []repository.PullRequest {
	byNum := map[int]repository.PullRequest{}
	for _, p := range before {
		byNum[p.Number] = p
	}
	var out []repository.PullRequest
	for _, p := range after {
		old, ok := byNum[p.Number]
		if !ok {
			continue
		}
		if old.UpdatedAt != p.UpdatedAt || old.ReviewDecision != p.ReviewDecision || old.Draft != p.Draft {
			out = append(out, p)
		}
	}
	return out
}

func newIssues(before, after []repository.Issue) []repository.Issue {
	seen := map[int]struct{}{}
	for _, i := range before {
		seen[i.Number] = struct{}{}
	}
	var out []repository.Issue
	for _, i := range after {
		if _, ok := seen[i.Number]; !ok {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Number < out[b].Number })
	return out
}

func newRuns(before, after []repository.WorkflowRun) []repository.WorkflowRun {
	seen := map[int64]struct{}{}
	for _, r := range before {
		seen[r.ID] = struct{}{}
	}
	var out []repository.WorkflowRun
	for _, r := range after {
		if _, ok := seen[r.ID]; !ok {
			out = append(out, r)
		}
	}
	return out
}

// completedRuns returns runs whose status is "completed" in after AND either
// (a) didn't exist in before, or (b) existed in before with a non-completed
// status. This is what "newly finished" means for build-result triggers.
func completedRuns(before, after []repository.WorkflowRun) []repository.WorkflowRun {
	prev := map[int64]repository.WorkflowRun{}
	for _, r := range before {
		prev[r.ID] = r
	}
	var out []repository.WorkflowRun
	for _, r := range after {
		if r.Status != "completed" {
			continue
		}
		old, ok := prev[r.ID]
		if !ok || old.Status != "completed" {
			out = append(out, r)
		}
	}
	return out
}

func newAssignments(before, after []repository.Issue) []IssueAssignment {
	prev := map[int]map[string]struct{}{}
	for _, i := range before {
		set := map[string]struct{}{}
		for _, a := range i.Assignees {
			set[strings.ToLower(a)] = struct{}{}
		}
		prev[i.Number] = set
	}
	var out []IssueAssignment
	for _, i := range after {
		old := prev[i.Number]
		for _, a := range i.Assignees {
			k := strings.ToLower(a)
			if _, ok := old[k]; ok {
				continue
			}
			out = append(out, IssueAssignment{Issue: i, Assignee: a})
		}
	}
	return out
}
