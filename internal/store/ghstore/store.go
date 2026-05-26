// Package ghstore is the single source of truth for GitHub repo data
// (pull requests, issues, recent workflow runs). All callers — the Wails
// HTTP-style endpoints feeding the UI and the AutomationManager — read
// through this store instead of calling the gh provider directly, so a
// single poll services every consumer of a given (owner, repo).
package ghstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/repository"
)

// Snapshot is the cached view of one repo at a point in time. Empty slices
// mean "fetched, nothing found"; a nil snapshot means "never fetched".
type Snapshot struct {
	PRs       []repository.PullRequest
	Issues    []repository.Issue
	Runs      []repository.WorkflowRun
	FetchedAt time.Time
	// PartialErr is set when one of the three sub-fetches failed while the
	// others succeeded. Callers can still use whichever slices are populated.
	PartialErr error
}

// Diff is what subscribers receive after a refresh. Slices contain the
// items that changed between Before and After.
type Diff struct {
	Owner, Repo string
	Before      Snapshot
	After       Snapshot

	PRsOpened     []repository.PullRequest
	PRsClosed     []repository.PullRequest
	PRsUpdated    []repository.PullRequest
	IssuesAdded   []repository.Issue
	IssuesAssigned []IssueAssignment
	RunsAdded     []repository.WorkflowRun
	// RunsCompleted is the subset of runs whose status just transitioned to
	// "completed" between Before and After. Useful for triggers that care
	// about terminal CI outcomes (pr_build_failed, pr_build_success).
	RunsCompleted []repository.WorkflowRun
}

// IssueAssignment records that `Assignee` newly appears on `Issue` (was not
// in the previous snapshot's assignees list).
type IssueAssignment struct {
	Issue    repository.Issue
	Assignee string
}

// Subscriber receives every diff after a successful refresh. Implementations
// must not block — the store calls them on its own refresh goroutine and a
// slow subscriber stalls the next tick.
type Subscriber func(Diff)

// Persistence persists snapshots across process restarts. Nil persistence is
// fine: the store works in-memory only.
type Persistence interface {
	Load() (map[string]Snapshot, error)
	Save(owner, repo string, snap Snapshot) error
}

type Store struct {
	mu      sync.RWMutex
	states  map[string]*state
	persist Persistence

	subMu sync.RWMutex
	subs  map[int]Subscriber
	subID int
}

type state struct {
	mu      sync.Mutex
	snap    Snapshot
	refs    map[string]time.Duration
	cancel  context.CancelFunc
	current time.Duration
	flight  chan struct{}
}

func key(owner, repo string) string {
	return owner + "/" + repo
}

func New(persist Persistence) *Store {
	s := &Store{
		states:  map[string]*state{},
		persist: persist,
		subs:    map[int]Subscriber{},
	}
	if persist != nil {
		if loaded, err := persist.Load(); err == nil {
			for k, snap := range loaded {
				s.states[k] = &state{snap: snap, refs: map[string]time.Duration{}}
			}
		}
	}
	return s
}

func (s *Store) getOrCreateState(k string) *state {
	s.mu.RLock()
	st, ok := s.states[k]
	s.mu.RUnlock()
	if ok {
		return st
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.states[k]; ok {
		return st
	}
	st = &state{refs: map[string]time.Duration{}}
	s.states[k] = st
	return st
}

// GetSnapshot returns the cached snapshot. If nothing's cached yet it
// triggers a refresh inline and waits for it.
func (s *Store) GetSnapshot(ctx context.Context, owner, repo string) (Snapshot, error) {
	k := key(owner, repo)
	st := s.getOrCreateState(k)
	st.mu.Lock()
	cached := st.snap
	hasData := !cached.FetchedAt.IsZero()
	st.mu.Unlock()
	if hasData {
		return cached, nil
	}
	if err := s.Refresh(ctx, owner, repo); err != nil {
		return Snapshot{}, err
	}
	st.mu.Lock()
	out := st.snap
	st.mu.Unlock()
	return out, nil
}

// GetPRs is a sugar over GetSnapshot for callers that only need PRs.
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

// Refresh forces a fresh fetch and emits a diff to subscribers. Concurrent
// calls for the same key coalesce: the second caller waits for the first
// fetch to finish instead of issuing a duplicate API call.
func (s *Store) Refresh(ctx context.Context, owner, repo string) error {
	k := key(owner, repo)
	st := s.getOrCreateState(k)

	st.mu.Lock()
	if st.flight != nil {
		ch := st.flight
		st.mu.Unlock()
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	st.flight = make(chan struct{})
	before := st.snap
	st.mu.Unlock()

	defer func() {
		st.mu.Lock()
		close(st.flight)
		st.flight = nil
		st.mu.Unlock()
	}()

	after, err := fetchAll(owner, repo)
	if err != nil && after.FetchedAt.IsZero() {
		return err
	}

	st.mu.Lock()
	st.snap = after
	st.mu.Unlock()

	if s.persist != nil {
		_ = s.persist.Save(owner, repo, after)
	}

	s.notify(Diff{
		Owner:          owner,
		Repo:           repo,
		Before:         before,
		After:          after,
		PRsOpened:      newPRs(before.PRs, after.PRs),
		PRsClosed:      removedPRs(before.PRs, after.PRs),
		PRsUpdated:     updatedPRs(before.PRs, after.PRs),
		IssuesAdded:    newIssues(before.Issues, after.Issues),
		IssuesAssigned: newAssignments(before.Issues, after.Issues),
		RunsAdded:      newRuns(before.Runs, after.Runs),
		RunsCompleted:  completedRuns(before.Runs, after.Runs),
	})
	return nil
}

// Register signals that refKey wants this repo polled at interval. The
// effective polling cadence is the minimum across all live registrations.
// Calling Register a second time with the same refKey updates that
// registration's interval.
func (s *Store) Register(ctx context.Context, owner, repo, refKey string, interval time.Duration) {
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	k := key(owner, repo)
	st := s.getOrCreateState(k)

	st.mu.Lock()
	st.refs[refKey] = interval
	next := minInterval(st.refs)
	needRestart := st.cancel == nil || next != st.current
	st.mu.Unlock()

	if needRestart {
		s.restartTicker(ctx, owner, repo, st, next)
	}
}

func (s *Store) Unregister(owner, repo, refKey string) {
	k := key(owner, repo)
	s.mu.RLock()
	st, ok := s.states[k]
	s.mu.RUnlock()
	if !ok {
		return
	}
	st.mu.Lock()
	delete(st.refs, refKey)
	empty := len(st.refs) == 0
	cancel := st.cancel
	if empty {
		st.cancel = nil
		st.current = 0
	}
	next := minInterval(st.refs)
	needRestart := !empty && next != st.current
	st.mu.Unlock()

	if empty && cancel != nil {
		cancel()
		return
	}
	if needRestart {
		s.restartTicker(context.Background(), owner, repo, st, next)
	}
}

func (s *Store) restartTicker(ctx context.Context, owner, repo string, st *state, interval time.Duration) {
	st.mu.Lock()
	if st.cancel != nil {
		st.cancel()
	}
	tickerCtx, cancel := context.WithCancel(ctx)
	st.cancel = cancel
	st.current = interval
	st.mu.Unlock()

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-t.C:
				_ = s.Refresh(tickerCtx, owner, repo)
			}
		}
	}()
}

func minInterval(refs map[string]time.Duration) time.Duration {
	var min time.Duration
	for _, d := range refs {
		if min == 0 || d < min {
			min = d
		}
	}
	return min
}

// Subscribe registers a handler called after every successful refresh.
// Returns an unsubscribe function. Handlers must not block.
func (s *Store) Subscribe(sub Subscriber) func() {
	s.subMu.Lock()
	s.subID++
	id := s.subID
	s.subs[id] = sub
	s.subMu.Unlock()
	return func() {
		s.subMu.Lock()
		delete(s.subs, id)
		s.subMu.Unlock()
	}
}

func (s *Store) notify(d Diff) {
	s.subMu.RLock()
	subs := make([]Subscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subMu.RUnlock()
	for _, sub := range subs {
		sub(d)
	}
}

// Stop cancels every active ticker. Safe to call multiple times.
func (s *Store) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.states {
		st.mu.Lock()
		if st.cancel != nil {
			st.cancel()
			st.cancel = nil
		}
		st.mu.Unlock()
	}
}

// fetchAll calls the three gh endpoints in parallel and bundles the result
// into a single Snapshot. A failure on one of the three doesn't kill the
// snapshot — the field stays nil and PartialErr carries the cause.
func fetchAll(owner, repo string) (Snapshot, error) {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []error
		snap  Snapshot
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
	// Stable order is helpful for tests; the API already returns sorted but
	// nothing in this package guarantees it.
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
			key := strings.ToLower(a)
			if _, ok := old[key]; ok {
				continue
			}
			out = append(out, IssueAssignment{Issue: i, Assignee: a})
		}
	}
	return out
}
