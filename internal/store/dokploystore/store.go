// Package dokploystore caches recent deployments for a Dokploy (instance,
// project-filter) tuple and emits diffs when deployments reach a terminal
// state. Multiple polaris projects watching the same instance + filter share a
// single poll loop (see internal/store/pollstore).
package dokploystore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
	"github.com/KevinBonnoron/polaris/internal/store/pollstore"
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

func (driver) Refresh(k Key, cfg Config, before Snapshot) (Snapshot, Diff, error) {
	after := fetchAll(cfg)
	return after, Diff{
		Key:                 k,
		Before:              before,
		After:               after,
		DeploymentsFinished: finishedDeployments(before, after),
	}, nil
}

type Store struct {
	*pollstore.Store[Key, Config, Snapshot, Diff]
}

func New(persist Persistence) *Store {
	var preloaded map[string]Snapshot
	if persist != nil {
		if loaded, err := persist.Load(); err == nil {
			preloaded = loaded
		}
	}
	return &Store{pollstore.New[Key, Config, Snapshot, Diff]("dokploystore", driver{persist: persist}, preloaded)}
}

// GetSnapshot returns the cached snapshot, fetching inline if cold.
func (s *Store) GetSnapshot(ctx context.Context, key Key, cfg Config) (Snapshot, error) {
	return s.Get(ctx, key, cfg)
}

// Reload registers the config and forces a fresh poll, returning the resulting
// snapshot. It is the UI read path: it shares the store cache (and in-flight
// requests) with automation polling, so the dashboard never double-calls the
// Dokploy API.
func (s *Store) Reload(ctx context.Context, cfg Config) (Snapshot, error) {
	k := KeyFor(cfg)
	s.SetConfig(k, cfg)
	if err := s.Refresh(ctx, k); err != nil {
		return Snapshot{}, err
	}
	snap, _ := s.Cached(k)
	return snap, nil
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
