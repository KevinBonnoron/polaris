// Package jirastore caches Jira sprint snapshots keyed by the tuple
// (baseUrl, projectKey, boardId). Multiple polaris projects pointing at
// the same Jira board share a single poll loop.
package jirastore

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/tickets"
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

type Store struct {
	mu      sync.RWMutex
	states  map[string]*state
	persist Persistence

	subMu sync.RWMutex
	subs  map[int]Subscriber
	subID int

	// configs maps key → the tickets.Config the store should use when fetching.
	// Set on Register and consulted on every refresh.
	configMu sync.RWMutex
	configs  map[string]tickets.Config
}

type state struct {
	mu      sync.Mutex
	snap    Snapshot
	refs    map[string]time.Duration
	cancel  context.CancelFunc
	current time.Duration
	flight  chan struct{}
}

func New(persist Persistence) *Store {
	s := &Store{
		states:  map[string]*state{},
		persist: persist,
		subs:    map[int]Subscriber{},
		configs: map[string]tickets.Config{},
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

// GetSnapshot returns the cached sprint snapshot, fetching inline if the
// cache is cold. cfg is consulted only when no cached config exists for the
// key (e.g. cold start before any Register call).
func (s *Store) GetSnapshot(ctx context.Context, k Key, cfg tickets.Config) (Snapshot, error) {
	s.setConfig(k, cfg)
	st := s.getOrCreateState(k.String())
	st.mu.Lock()
	cached := st.snap
	hasData := !cached.FetchedAt.IsZero()
	st.mu.Unlock()
	if hasData {
		return cached, nil
	}
	if err := s.Refresh(ctx, k); err != nil {
		return Snapshot{}, err
	}
	st.mu.Lock()
	out := st.snap
	st.mu.Unlock()
	return out, nil
}

func (s *Store) setConfig(k Key, cfg tickets.Config) {
	s.configMu.Lock()
	s.configs[k.String()] = cfg
	s.configMu.Unlock()
}

// SetConfig records the Jira credentials used to fetch this key. Callers
// that drive Refresh without going through Register (e.g. one-shot reads
// from a Wails endpoint) must call this first.
func (s *Store) SetConfig(k Key, cfg tickets.Config) {
	s.setConfig(k, cfg)
}

func (s *Store) getConfig(k Key) (tickets.Config, bool) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	c, ok := s.configs[k.String()]
	return c, ok
}

func (s *Store) Refresh(ctx context.Context, k Key) error {
	cfg, ok := s.getConfig(k)
	if !ok {
		return fmt.Errorf("jirastore: no config registered for %s", k)
	}
	st := s.getOrCreateState(k.String())

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

	sprint, err := tickets.FetchActiveSprint(cfg)
	after := Snapshot{Sprint: sprint, FetchedAt: time.Now(), Err: err}

	st.mu.Lock()
	st.snap = after
	st.mu.Unlock()

	if s.persist != nil {
		_ = s.persist.Save(k, after)
	}

	s.notify(Diff{
		Key:                k,
		Before:             before,
		After:              after,
		IssuesTransitioned: transitions(before, after),
		IssuesReassigned:   reassignments(before, after),
		IssuesAdded:        addedIssues(before, after),
	})
	return nil
}

func (s *Store) Register(ctx context.Context, k Key, cfg tickets.Config, refKey string, interval time.Duration) {
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	s.setConfig(k, cfg)
	st := s.getOrCreateState(k.String())

	st.mu.Lock()
	st.refs[refKey] = interval
	next := minInterval(st.refs)
	needRestart := st.cancel == nil || next != st.current
	st.mu.Unlock()

	if needRestart {
		s.restartTicker(ctx, k, st, next)
	}
}

func (s *Store) Unregister(k Key, refKey string) {
	s.mu.RLock()
	st, ok := s.states[k.String()]
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
		s.restartTicker(context.Background(), k, st, next)
	}
}

func (s *Store) restartTicker(ctx context.Context, k Key, st *state, interval time.Duration) {
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
				_ = s.Refresh(tickerCtx, k)
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
