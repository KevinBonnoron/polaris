package dokploystore

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE provider_cache (provider TEXT NOT NULL, key TEXT NOT NULL, payload TEXT NOT NULL DEFAULT '{}', fetchedAt INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (provider, key))`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestPersistenceRoundTrip(t *testing.T) {
	p := &SQLitePersistence{DB: newTestDB(t)}
	k := Key{BaseURL: "https://dokploy.example.com", Projects: "api"}
	snap := Snapshot{
		Services:    []dokploy.Service{{ID: "s1", Name: "api", Project: "Backend"}},
		Deployments: []dokploy.Deployment{{ID: "d1", Status: "done", Service: "api"}},
		FetchedAt:   time.Unix(1_700_000_000, 0),
	}
	if err := p.Save(k, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := p.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := loaded[k.String()]
	if !ok {
		t.Fatalf("key %q not loaded; got %v", k.String(), loaded)
	}
	if len(got.Services) != 1 || got.Services[0].ID != "s1" {
		t.Fatalf("services round-trip mismatch: %+v", got.Services)
	}
	if len(got.Deployments) != 1 || got.Deployments[0].Status != "done" {
		t.Fatalf("deployments round-trip mismatch: %+v", got.Deployments)
	}
	if !got.FetchedAt.Equal(snap.FetchedAt) {
		t.Fatalf("fetchedAt = %v, want %v", got.FetchedAt, snap.FetchedAt)
	}
}

func TestPersistenceUpsertReplaces(t *testing.T) {
	p := &SQLitePersistence{DB: newTestDB(t)}
	k := Key{BaseURL: "https://x", Projects: ""}
	_ = p.Save(k, Snapshot{Deployments: []dokploy.Deployment{{ID: "old"}}, FetchedAt: time.Unix(1, 0)})
	_ = p.Save(k, Snapshot{Deployments: []dokploy.Deployment{{ID: "new"}}, FetchedAt: time.Unix(2, 0)})

	loaded, err := p.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := loaded[k.String()]
	if len(got.Deployments) != 1 || got.Deployments[0].ID != "new" {
		t.Fatalf("upsert did not replace: %+v", got.Deployments)
	}
}

func TestNilPersistenceIsSafe(t *testing.T) {
	var p *SQLitePersistence
	if _, err := p.Load(); err != nil {
		t.Fatalf("nil Load: %v", err)
	}
	if err := p.Save(Key{}, Snapshot{}); err != nil {
		t.Fatalf("nil Save: %v", err)
	}
}
