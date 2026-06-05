// Package dokploystore caches recent deployments for a Dokploy (instance,
// project-filter) tuple and emits diffs when deployments reach a terminal
// state. Multiple polaris projects watching the same instance + filter share a
// single poll loop, mirroring ghstore / ticketsstore / sentrystore.
package dokploystore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
)

// Key identifies one watched (instance, project-filter) tuple. Projects is the
// sorted, comma-joined name filter so the key stays comparable; empty means
// "all projects on the instance".
type Key struct {
	BaseURL  string
	Projects string
}

func (k Key) String() string {
	return fmt.Sprintf("%s|%s", k.BaseURL, k.Projects)
}

// Config carries the credentials and the optional project name filter. An
// empty Projects list watches every project on the instance.
type Config struct {
	BaseURL  string
	APIKey   string
	Projects []string
}

func KeyFor(cfg Config) Key {
	names := append([]string(nil), cfg.Projects...)
	sort.Strings(names)
	return Key{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), Projects: strings.Join(names, ",")}
}

// Snapshot is the cached union of services and their recent deployments across
// the key's projects. Services includes databases (which carry only a status,
// no deployments) so the dashboard can list every service.
type Snapshot struct {
	Services    []dokploy.Service
	Deployments []dokploy.Deployment
	FetchedAt   time.Time
	Err         error
}

// Diff is delivered to subscribers after a refresh. DeploymentsFinished are
// deployments whose status just transitioned to a terminal state (done, error
// or cancelled) since the previous snapshot.
type Diff struct {
	Key                 Key
	Before              Snapshot
	After               Snapshot
	DeploymentsFinished []dokploy.Deployment
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

func (store *Store) getOrCreateState(k string) *state {
	store.mu.RLock()
	st, ok := store.states[k]
	store.mu.RUnlock()
	if ok {
		return st
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if st, ok := store.states[k]; ok {
		return st
	}
	st = &state{refs: map[string]time.Duration{}}
	store.states[k] = st
	return st
}

func (store *Store) setConfig(k Key, cfg Config) {
	store.configMu.Lock()
	store.configs[k.String()] = cfg
	store.configMu.Unlock()
}

func (store *Store) getConfig(k Key) (Config, bool) {
	store.configMu.RLock()
	defer store.configMu.RUnlock()
	c, ok := store.configs[k.String()]
	return c, ok
}

// Reload registers the config and forces a fresh poll, returning the resulting
// snapshot. It is the UI read path: it shares the store cache (and in-flight
// requests) with automation polling, so the dashboard never double-calls the
// Dokploy API. A concurrent automation tick is deduplicated by Refresh.
func (store *Store) Reload(ctx context.Context, cfg Config) (Snapshot, error) {
	k := KeyFor(cfg)
	store.setConfig(k, cfg)
	if err := store.Refresh(ctx, k); err != nil {
		return Snapshot{}, err
	}
	st := store.getOrCreateState(k.String())
	st.mu.Lock()
	out := st.snap
	st.mu.Unlock()
	return out, nil
}

// GetSnapshot returns the cached snapshot, fetching inline if cold.
func (store *Store) GetSnapshot(ctx context.Context, key Key, cfg Config) (Snapshot, error) {
	store.setConfig(key, cfg)
	state := store.getOrCreateState(key.String())
	state.mu.Lock()
	cached := state.snap
	hasData := !cached.FetchedAt.IsZero()
	state.mu.Unlock()
	if hasData {
		return cached, nil
	}

	if err := store.Refresh(ctx, key); err != nil {
		return Snapshot{}, err
	}

	state.mu.Lock()
	out := state.snap
	state.mu.Unlock()
	return out, nil
}

func (store *Store) Refresh(ctx context.Context, k Key) error {
	cfg, ok := store.getConfig(k)
	if !ok {
		return fmt.Errorf("dokploystore: no config registered for %s", k)
	}

	state := store.getOrCreateState(k.String())
	state.mu.Lock()
	if state.flight != nil {
		ch := state.flight
		state.mu.Unlock()
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	state.flight = make(chan struct{})
	before := state.snap
	state.mu.Unlock()

	defer func() {
		state.mu.Lock()
		close(state.flight)
		state.flight = nil
		state.mu.Unlock()
	}()

	after := fetchAll(cfg)

	state.mu.Lock()
	state.snap = after
	state.mu.Unlock()

	store.notify(Diff{
		Key:                 k,
		Before:              before,
		After:               after,
		DeploymentsFinished: finishedDeployments(before, after),
	})
	return nil
}

func (store *Store) Register(ctx context.Context, k Key, cfg Config, refKey string, interval time.Duration) {
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	store.setConfig(k, cfg)
	st := store.getOrCreateState(k.String())

	st.mu.Lock()
	st.refs[refKey] = interval
	next := minInterval(st.refs)
	needRestart := st.cancel == nil || next != st.current
	st.mu.Unlock()

	if needRestart {
		store.restartTicker(ctx, k, st, next)
	}
}

func (store *Store) Unregister(k Key, refKey string) {
	store.mu.RLock()
	st, ok := store.states[k.String()]
	store.mu.RUnlock()
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
		store.restartTicker(context.Background(), k, st, next)
	}
}

func (store *Store) restartTicker(ctx context.Context, k Key, st *state, interval time.Duration) {
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
				_ = store.Refresh(tickerCtx, k)
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

func (store *Store) Subscribe(sub Subscriber) func() {
	store.subMu.Lock()
	store.subID++
	id := store.subID
	store.subs[id] = sub
	store.subMu.Unlock()
	return func() {
		store.subMu.Lock()
		delete(store.subs, id)
		store.subMu.Unlock()
	}
}

func (store *Store) notify(d Diff) {
	store.subMu.RLock()
	subs := make([]Subscriber, 0, len(store.subs))
	for _, sub := range store.subs {
		subs = append(subs, sub)
	}
	store.subMu.RUnlock()
	for _, sub := range subs {
		sub(d)
	}
}

func (store *Store) Stop() {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, st := range store.states {
		st.mu.Lock()
		if st.cancel != nil {
			st.cancel()
			st.cancel = nil
		}
		st.mu.Unlock()
	}
}

// fetchAll resolves the project tree, keeps the projects matching the filter
// (empty filter = all), then queries each service's recent deployments and
// unions them. A per-service failure is recorded in Err but never drops the
// other services' results.
func fetchAll(cfg Config) Snapshot {
	dcfg := dokploy.Config{BaseURL: cfg.BaseURL, APIKey: cfg.APIKey}
	projects, err := dokploy.FetchProjects(dcfg)
	if err != nil {
		return Snapshot{FetchedAt: time.Now(), Err: fmt.Errorf("dokploystore: %w", err)}
	}

	services := selectServices(projects, cfg.Projects)

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		all  []dokploy.Deployment
		errs []error
	)
	for _, svc := range services {
		if !svc.Type.Deployable() {
			continue
		}
		wg.Add(1)
		go func(svc dokploy.Service) {
			defer wg.Done()
			deployments, err := dokploy.FetchDeployments(dcfg, svc)
			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", svc.Project, svc.Name, err))
			} else {
				for i := range deployments {
					deployments[i].Project = svc.Project
					deployments[i].Service = svc.Name
				}
				all = append(all, deployments...)
			}
			mu.Unlock()
		}(svc)
	}
	wg.Wait()

	snap := Snapshot{Services: services, Deployments: all, FetchedAt: time.Now()}
	if len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		snap.Err = fmt.Errorf("dokploystore: %s", strings.Join(msgs, "; "))
	}
	return snap
}

// selectServices flattens the services of every project, keeping only the
// projects whose name matches the filter (case-insensitive, trimmed). An empty
// filter keeps every project.
func selectServices(projects []dokploy.Project, filter []string) []dokploy.Service {
	wanted := map[string]struct{}{}
	for _, name := range filter {
		if n := strings.ToLower(strings.TrimSpace(name)); n != "" {
			wanted[n] = struct{}{}
		}
	}

	var services []dokploy.Service
	for _, p := range projects {
		if len(wanted) > 0 {
			if _, ok := wanted[strings.ToLower(strings.TrimSpace(p.Name))]; !ok {
				continue
			}
		}
		services = append(services, p.Services...)
	}
	return services
}

// finishedDeployments returns deployments that are terminal in After but were
// either absent or still running in Before — i.e. they just finished.
func finishedDeployments(before, after Snapshot) []dokploy.Deployment {
	prev := map[string]dokploy.Deployment{}
	for _, d := range before.Deployments {
		prev[d.ID] = d
	}
	var out []dokploy.Deployment
	for _, d := range after.Deployments {
		if !d.IsTerminal() {
			continue
		}
		old, ok := prev[d.ID]
		if !ok || !old.IsTerminal() {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
