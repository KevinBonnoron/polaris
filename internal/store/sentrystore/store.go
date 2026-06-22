// Package sentrystore caches the unresolved-issue list for a Sentry
// (org, projects) tuple and emits diffs when issues enter or leave that set.
// Multiple polaris projects watching the same org share a single poll loop
// (see internal/store/pollstore).
package sentrystore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/sentry"
	"github.com/KevinBonnoron/polaris/internal/store/pollstore"
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
// present in After but not seen before — i.e. brand-new issues and regressions
// (resolved → unresolved re-entries) alike.
type Diff struct {
	Key         Key
	Before      Snapshot
	After       Snapshot
	IssuesAdded []sentry.Issue
}

type Subscriber func(Diff)

type Persistence interface {
	Load() (map[string]Snapshot, error)
	Save(k Key, snap Snapshot) error
}

// driver tracks, per key, the set of issue IDs seen so far so a refresh can tell
// which issues are new. The seen-set is guarded by its own mutex: the singleflight
// in pollstore already serialises refreshes per key, this just makes concurrent
// keys safe.
type driver struct {
	persist Persistence

	mu   sync.Mutex
	seen map[string]map[string]struct{}
}

func newDriver(persist Persistence) *driver {
	return &driver{persist: persist, seen: map[string]map[string]struct{}{}}
}

func (*driver) KeyString(k Key) string { return k.String() }

// HasData treats a failed fetch as cold so Get re-fetches instead of serving an
// error snapshot as cache.
func (*driver) HasData(s Snapshot) bool { return !s.FetchedAt.IsZero() && s.Err == nil }

func (d *driver) Persist(k Key, snap Snapshot) error {
	// Never overwrite the last good cache with a failed fetch.
	if snap.Err != nil || d.persist == nil {
		return nil
	}
	return d.persist.Save(k, snap)
}

func (d *driver) Refresh(k Key, cfg Config, before Snapshot) (Snapshot, Diff, error) {
	after := fetchAll(cfg)

	d.mu.Lock()
	kstr := k.String()
	seen := d.seen[kstr]
	if seen == nil {
		seen = map[string]struct{}{}
		d.seen[kstr] = seen
	}
	added := addedFromSeen(seen, after)
	for _, i := range after.Issues {
		seen[i.ID] = struct{}{}
	}
	if after.Err == nil {
		current := map[string]struct{}{}
		for _, i := range after.Issues {
			current[i.ID] = struct{}{}
		}
		for id := range seen {
			if _, ok := current[id]; !ok {
				delete(seen, id)
			}
		}
	}
	d.mu.Unlock()

	return after, Diff{Key: k, Before: before, After: after, IssuesAdded: added}, nil
}

type Store struct {
	*pollstore.Store[Key, Config, Snapshot, Diff]
}

func New(persist Persistence) *Store {
	d := newDriver(persist)
	var preloaded map[string]Snapshot
	if persist != nil {
		if loaded, err := persist.Load(); err == nil {
			preloaded = loaded
			// Seed the seen-set from the restored snapshots so the first refresh
			// after a restart doesn't report every already-known issue as new.
			for kstr, snap := range loaded {
				seen := make(map[string]struct{}, len(snap.Issues))
				for _, i := range snap.Issues {
					seen[i.ID] = struct{}{}
				}
				d.seen[kstr] = seen
			}
		}
	}
	return &Store{pollstore.New[Key, Config, Snapshot, Diff]("sentrystore", d, preloaded)}
}

// GetSnapshot returns the cached snapshot, fetching inline if cold.
func (s *Store) GetSnapshot(ctx context.Context, k Key, cfg Config) (Snapshot, error) {
	return s.Get(ctx, k, cfg)
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
