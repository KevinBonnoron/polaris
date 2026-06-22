package pollstore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type diff struct {
	key string
	val int
}

// fakeDriver counts fetches and can block them on a release channel so a test
// can hold several callers in flight at once.
type fakeDriver struct {
	count   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (*fakeDriver) KeyString(k string) string { return k }
func (*fakeDriver) HasData(s int) bool        { return s > 0 }
func (*fakeDriver) Persist(string, int) error { return nil }

func (d *fakeDriver) Refresh(k string, cfg int, before int) (int, diff, error) {
	d.count.Add(1)
	if d.started != nil {
		d.started <- struct{}{}
	}
	if d.release != nil {
		<-d.release
	}
	return before + 1, diff{key: k, val: before + 1}, nil
}

func TestRefreshCoalesces(t *testing.T) {
	d := &fakeDriver{started: make(chan struct{}, 16), release: make(chan struct{})}
	s := New[string, int, int, diff]("test", d, nil)
	s.SetConfig("k", 0)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.Refresh(context.Background(), "k") }()
	}
	<-d.started // one fetch has entered
	// Let the other five reach the in-flight check and coalesce.
	time.Sleep(50 * time.Millisecond)
	close(d.release)
	wg.Wait()

	if got := d.count.Load(); got != 1 {
		t.Fatalf("concurrent refreshes triggered %d fetches, want 1 (coalesced)", got)
	}
}

func TestGetWarmsAndCaches(t *testing.T) {
	d := &fakeDriver{}
	s := New[string, int, int, diff]("test", d, nil)

	snap, err := s.Get(context.Background(), "k", 0)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap != 1 {
		t.Fatalf("first Get snapshot = %d, want 1", snap)
	}
	// Warm now: a second Get must not fetch again.
	if _, err := s.Get(context.Background(), "k", 0); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := d.count.Load(); got != 1 {
		t.Fatalf("warm Get re-fetched: %d fetches, want 1", got)
	}
}

func TestRefreshNeedsConfig(t *testing.T) {
	s := New[string, int, int, diff]("test", &fakeDriver{}, nil)
	if err := s.Refresh(context.Background(), "k"); err == nil {
		t.Fatal("expected error refreshing a key with no config")
	}
}

func TestSubscribeReceivesDiffAndUnsubscribe(t *testing.T) {
	d := &fakeDriver{}
	s := New[string, int, int, diff]("test", d, nil)
	s.SetConfig("k", 0)

	var mu sync.Mutex
	var got []diff
	unsub := s.Subscribe(func(dd diff) {
		mu.Lock()
		got = append(got, dd)
		mu.Unlock()
	})

	if err := s.Refresh(context.Background(), "k"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("subscriber got %d diffs, want 1", n)
	}

	unsub()
	if err := s.Refresh(context.Background(), "k"); err != nil {
		t.Fatalf("Refresh after unsub: %v", err)
	}
	mu.Lock()
	n = len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("unsubscribed handler still fired: %d diffs", n)
	}
}

func TestRegisterPollsThenStop(t *testing.T) {
	d := &fakeDriver{}
	s := New[string, int, int, diff]("test", d, nil)

	ticked := make(chan struct{}, 8)
	s.Subscribe(func(diff) {
		select {
		case ticked <- struct{}{}:
		default:
		}
	})
	// 10s is the floor; the registration clamps up to it, so drive Refresh
	// directly to prove Register wired the config, then assert Stop is clean.
	s.Register(context.Background(), "k", 0, "ref", time.Second)
	if err := s.Refresh(context.Background(), "k"); err != nil {
		t.Fatalf("Refresh after Register: %v", err)
	}
	select {
	case <-ticked:
	case <-time.After(time.Second):
		t.Fatal("no diff delivered after Register + Refresh")
	}
	s.Unregister("k", "ref")
	s.Stop()
}
