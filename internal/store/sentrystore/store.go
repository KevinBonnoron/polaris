// Package sentrystore caches the unresolved-issue list for a Sentry
// (org, projects) tuple and emits diffs when issues enter or leave that set.
// Multiple polaris projects watching the same org share a single poll loop,
// mirroring ghstore / jirastore.
package sentrystore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
)

// Key identifies one watched (instance, org, projects) tuple. Projects is the
// sorted, comma-joined slug list so the key stays comparable.
type Key struct {
	BaseURL  string
	Org      string
	Projects string
}

func (k Key) String() string {
	return fmt.Sprintf("%s|%s|%s", k.BaseURL, k.Org, k.Projects)
}

// Config carries the credentials and the full project list the store fetches.
type Config struct {
	Token    string
	Org      string
	URL      string
	Projects []string
}

func KeyFor(cfg Config) Key {
	slugs := append([]string(nil), cfg.Projects...)
	sort.Strings(slugs)
	return Key{BaseURL: strings.TrimRight(cfg.URL, "/"), Org: cfg.Org, Projects: strings.Join(slugs, ",")}
}

// Snapshot is the cached union of unresolved issues across the key's projects.
type Snapshot struct {
	Issues    []sentry.Issue
	FetchedAt time.Time
	Err       error
}

// Diff is delivered to subscribers after a refresh. IssuesAdded are issues
// present in After but not Before — i.e. brand-new issues and regressions
// (resolved → unresolved re-entries) alike.
type Diff struct {
	Key         Key
	Before      Snapshot
	After       Snapshot
	IssuesAdded []sentry.Issue
}

type Subscriber func(Diff)

type Store struct {
	mu     sync.RWMutex
	states map[string]*state

	subMu sync.RWMutex
	subs  map[int]Subscriber
	subID int

	configMu sync.RWMutex
	configs  map[string]Config
}

type state struct {
	mu      sync.Mutex
	snap    Snapshot
	seen    map[string]struct{}
	refs    map[string]time.Duration
	cancel  context.CancelFunc
	current time.Duration
	flight  chan struct{}
}

func New() *Store {
	return &Store{
		states:  map[string]*state{},
		subs:    map[int]Subscriber{},
		configs: map[string]Config{},
	}
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
	st = &state{refs: map[string]time.Duration{}, seen: map[string]struct{}{}}
	s.states[k] = st
	return st
}

func (s *Store) setConfig(k Key, cfg Config) {
	s.configMu.Lock()
	s.configs[k.String()] = cfg
	s.configMu.Unlock()
}

func (s *Store) getConfig(k Key) (Config, bool) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	c, ok := s.configs[k.String()]
	return c, ok
}

// GetSnapshot returns the cached snapshot, fetching inline if cold.
func (s *Store) GetSnapshot(ctx context.Context, k Key, cfg Config) (Snapshot, error) {
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

func (s *Store) Refresh(ctx context.Context, k Key) error {
	cfg, ok := s.getConfig(k)
	if !ok {
		return fmt.Errorf("sentrystore: no config registered for %s", k)
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

	after := fetchAll(cfg)

	st.mu.Lock()
	added := addedFromSeen(st.seen, after)
	for _, i := range after.Issues {
		st.seen[i.ID] = struct{}{}
	}
	if after.Err == nil {
		current := map[string]struct{}{}
		for _, i := range after.Issues {
			current[i.ID] = struct{}{}
		}
		for id := range st.seen {
			if _, ok := current[id]; !ok {
				delete(st.seen, id)
			}
		}
	}
	st.snap = after
	st.mu.Unlock()

	s.notify(Diff{
		Key:         k,
		Before:      before,
		After:       after,
		IssuesAdded: added,
	})
	return nil
}

func (s *Store) Register(ctx context.Context, k Key, cfg Config, refKey string, interval time.Duration) {
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

// fetchAll queries every project's unresolved issues and unions them. A
// per-project failure is recorded in Err but never drops the other projects'
// results.
func fetchAll(cfg Config) Snapshot {
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		all  []sentry.Issue
		errs []error
	)
	for _, project := range cfg.Projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			issues, err := sentry.FetchIssues(sentry.Config{Token: cfg.Token, Org: cfg.Org, Project: project, URL: cfg.URL})
			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", project, err))
			} else {
				all = append(all, issues...)
			}
			mu.Unlock()
		}(project)
	}
	wg.Wait()

	snap := Snapshot{Issues: all, FetchedAt: time.Now()}
	if len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		snap.Err = fmt.Errorf("sentrystore: %s", strings.Join(msgs, "; "))
	}
	return snap
}

func addedFromSeen(seen map[string]struct{}, after Snapshot) []sentry.Issue {
	var out []sentry.Issue
	for _, i := range after.Issues {
		if _, ok := seen[i.ID]; !ok {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
