package dokploystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/dokploy"
)

const provider = "dokploy"

type SQLitePersistence struct {
	DB *sql.DB
}

type payload struct {
	Services    []dokploy.Service    `json:"services"`
	Deployments []dokploy.Deployment `json:"deployments"`
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
			key       string
			raw       string
			fetchedAt int64
		)
		if err := rows.Scan(&key, &raw, &fetchedAt); err != nil {
			return nil, err
		}
		var pl payload
		_ = json.Unmarshal([]byte(raw), &pl)
		snap := Snapshot{Services: pl.Services, Deployments: pl.Deployments}
		if fetchedAt > 0 {
			snap.FetchedAt = time.Unix(fetchedAt, 0)
		}
		out[key] = snap
	}
	return out, rows.Err()
}

func (p *SQLitePersistence) Save(k Key, snap Snapshot) error {
	if p == nil || p.DB == nil {
		return nil
	}
	buf, _ := json.Marshal(payload{Services: snap.Services, Deployments: snap.Deployments})
	_, err := p.DB.Exec(
		`INSERT INTO provider_cache (provider, key, payload, fetchedAt) VALUES (?, ?, ?, ?)
		 ON CONFLICT(provider, key) DO UPDATE SET payload=excluded.payload, fetchedAt=excluded.fetchedAt`,
		provider, k.String(), string(buf), snap.FetchedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("dokploystore: save %s: %w", k, err)
	}
	return nil
}
