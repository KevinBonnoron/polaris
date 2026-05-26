package ghstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/repository"
)

const provider = "github"

type SQLitePersistence struct {
	DB *sql.DB
}

type payload struct {
	PRs    []repository.PullRequest `json:"prs"`
	Issues []repository.Issue       `json:"issues"`
	Runs   []repository.WorkflowRun `json:"runs"`
}

func (p *SQLitePersistence) Load() (map[string]Snapshot, error) {
	if p == nil || p.DB == nil {
		return nil, nil
	}
	rows, err := p.DB.QueryContext(context.Background(), `SELECT key, payload, fetchedAt FROM provider_cache WHERE provider = ?`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Snapshot{}
	for rows.Next() {
		var (
			key        string
			raw        string
			fetchedAt  int64
		)
		if err := rows.Scan(&key, &raw, &fetchedAt); err != nil {
			return nil, err
		}
		var pl payload
		_ = json.Unmarshal([]byte(raw), &pl)
		snap := Snapshot{PRs: pl.PRs, Issues: pl.Issues, Runs: pl.Runs}
		if fetchedAt > 0 {
			snap.FetchedAt = time.Unix(fetchedAt, 0)
		}
		out[key] = snap
	}
	return out, rows.Err()
}

func (p *SQLitePersistence) Save(owner, repo string, snap Snapshot) error {
	if p == nil || p.DB == nil {
		return nil
	}
	pl := payload{
		PRs:    nilSlice(snap.PRs),
		Issues: nilSlice(snap.Issues),
		Runs:   nilSlice(snap.Runs),
	}
	buf, _ := json.Marshal(pl)
	_, err := p.DB.Exec(
		`INSERT INTO provider_cache (provider, key, payload, fetchedAt) VALUES (?, ?, ?, ?)
		 ON CONFLICT(provider, key) DO UPDATE SET payload=excluded.payload, fetchedAt=excluded.fetchedAt`,
		provider, owner+"/"+repo, string(buf), snap.FetchedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("ghstore: save %s/%s: %w", owner, repo, err)
	}
	return nil
}

func nilSlice[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
