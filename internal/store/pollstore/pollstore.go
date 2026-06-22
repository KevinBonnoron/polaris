// Package pollstore is the shared engine behind the provider caches
// (sentry/tickets/dokploy/repository). Each of those caches one snapshot per
// watched key, lets several polaris projects share a single poll loop via
// ref-counted registrations, coalesces concurrent refreshes, and fans diffs out
// to subscribers. Only the fetch + diff logic differs per provider; that lives
// in a Driver. The locking discipline (a store-level RWMutex guarding the state
// registry, a per-state mutex guarding that state's fields, and a singleflight
// channel) is defined here once.
package pollstore

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Driver supplies the provider-specific parts. A Driver may keep cross-tick
// state of its own (e.g. a seen-set); since Refresh runs outside the store's
// locks it must guard that itself.
type Driver[K comparable, C any, S any, D any] interface {
	// KeyString is the stable string form of a key, used as the registry map
	// key, in error messages, and for persistence.
	KeyString(K) string
	// Refresh fetches fresh data for k (given cfg and the previous snapshot) and
	// returns the new snapshot plus the diff to deliver. A non-nil error means
	// the fetch failed outright: the snapshot is left untouched and the error is
	// returned to the caller (no diff is emitted). Recoverable/partial failures
	// should instead be carried inside the snapshot, not returned here.
	Refresh(k K, cfg C, before S) (after S, diff D, err error)
	// HasData reports whether a snapshot is warm, so a cold key triggers an
	// inline fetch in Get.
	HasData(S) bool
	// Persist saves a snapshot across restarts. Drivers without persistence
	// return nil.
	Persist(k K, snap S) error
}

type Store[K comparable, C any, S any, D any] struct {
	name   string
	driver Driver[K, C, S, D]

	mu     sync.RWMutex
	states map[string]*state[S]

	subMu sync.RWMutex
	subs  map[int]func(D)
	subID int

	configMu sync.RWMutex
	configs  map[string]C
}

type state[S any] struct {
	mu      sync.Mutex
	snap    S
	refs    map[string]time.Duration
	cancel  context.CancelFunc
	current time.Duration
	flight  chan struct{}
}

// New builds a store for the given driver. name prefixes error messages.
// preloaded seeds warm snapshots (keyed by KeyString) for stores restoring from
// persistence; pass nil otherwise.
func New[K comparable, C any, S any, D any](name string, driver Driver[K, C, S, D], preloaded map[string]S) *Store[K, C, S, D] {
	s := &Store[K, C, S, D]{
		name:    name,
		driver:  driver,
		states:  map[string]*state[S]{},
		subs:    map[int]func(D){},
		configs: map[string]C{},
	}
	for k, snap := range preloaded {
		s.states[k] = &state[S]{snap: snap, refs: map[string]time.Duration{}}
	}
	return s
}

func (s *Store[K, C, S, D]) getOrCreateState(k string) *state[S] {
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
	st = &state[S]{refs: map[string]time.Duration{}}
	s.states[k] = st
	return st
}

// SetConfig records the credentials used to fetch this key. Callers that drive
// Refresh without going through Register (e.g. one-shot reads from a Wails
// endpoint) must call this first.
func (s *Store[K, C, S, D]) SetConfig(k K, cfg C) {
	s.configMu.Lock()
	s.configs[s.driver.KeyString(k)] = cfg
	s.configMu.Unlock()
}

func (s *Store[K, C, S, D]) getConfig(k K) (C, bool) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	c, ok := s.configs[s.driver.KeyString(k)]
	return c, ok
}

// Get returns the cached snapshot, fetching inline if the cache is cold.
func (s *Store[K, C, S, D]) Get(ctx context.Context, k K, cfg C) (S, error) {
	s.SetConfig(k, cfg)
	st := s.getOrCreateState(s.driver.KeyString(k))
	st.mu.Lock()
	cached := st.snap
	warm := s.driver.HasData(cached)
	st.mu.Unlock()
	if warm {
		return cached, nil
	}
	if err := s.Refresh(ctx, k); err != nil {
		var zero S
		return zero, err
	}
	st.mu.Lock()
	out := st.snap
	st.mu.Unlock()
	return out, nil
}

// Cached returns the cached snapshot without fetching, and whether the key has
// any state yet.
func (s *Store[K, C, S, D]) Cached(k K) (S, bool) {
	s.mu.RLock()
	st, ok := s.states[s.driver.KeyString(k)]
	s.mu.RUnlock()
	if !ok {
		var zero S
		return zero, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.snap, true
}

// Refresh forces a fetch and emits a diff to subscribers. Concurrent calls for
// the same key coalesce: a second caller waits for the in-flight fetch instead
// of issuing a duplicate request.
func (s *Store[K, C, S, D]) Refresh(ctx context.Context, k K) error {
	cfg, ok := s.getConfig(k)
	if !ok {
		return fmt.Errorf("%s: no config registered for %s", s.name, s.driver.KeyString(k))
	}
	st := s.getOrCreateState(s.driver.KeyString(k))

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

	after, diff, err := s.driver.Refresh(k, cfg, before)
	if err != nil {
		return err
	}

	st.mu.Lock()
	st.snap = after
	st.mu.Unlock()

	_ = s.driver.Persist(k, after)
	s.notify(diff)
	return nil
}

// Register signals that refKey wants this key polled at interval. The effective
// cadence is the minimum across all live registrations. Calling Register again
// with the same refKey updates that registration's interval.
func (s *Store[K, C, S, D]) Register(ctx context.Context, k K, cfg C, refKey string, interval time.Duration) {
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	s.SetConfig(k, cfg)
	st := s.getOrCreateState(s.driver.KeyString(k))

	st.mu.Lock()
	st.refs[refKey] = interval
	next := minInterval(st.refs)
	needRestart := st.cancel == nil || next != st.current
	st.mu.Unlock()

	if needRestart {
		s.restartTicker(ctx, k, st, next)
	}
}

func (s *Store[K, C, S, D]) Unregister(k K, refKey string) {
	s.mu.RLock()
	st, ok := s.states[s.driver.KeyString(k)]
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

func (s *Store[K, C, S, D]) restartTicker(ctx context.Context, k K, st *state[S], interval time.Duration) {
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

// Subscribe registers a handler called after every successful refresh and
// returns an unsubscribe function. Handlers must not block — they run on the
// refresh goroutine.
func (s *Store[K, C, S, D]) Subscribe(sub func(D)) func() {
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

func (s *Store[K, C, S, D]) notify(d D) {
	s.subMu.RLock()
	subs := make([]func(D), 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subMu.RUnlock()
	for _, sub := range subs {
		sub(d)
	}
}

// Stop cancels every active ticker. Safe to call multiple times.
func (s *Store[K, C, S, D]) Stop() {
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
