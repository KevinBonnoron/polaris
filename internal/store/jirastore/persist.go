package jirastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KevinBonnoron/polaris/internal/providers/jira"
)

const provider = "jira"

type SQLitePersistence struct {
	DB *sql.DB
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
		snap := Snapshot{}
		if raw != "" && raw != "null" {
			var sprint jira.Sprint
			if err := json.Unmarshal([]byte(raw), &sprint); err == nil {
				snap.Sprint = &sprint
			}
		}
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
	payload := "null"
	if snap.Sprint != nil {
		buf, err := json.Marshal(snap.Sprint)
		if err == nil {
			payload = string(buf)
		}
	}
	_, err := p.DB.Exec(
		`INSERT INTO provider_cache (provider, key, payload, fetchedAt) VALUES (?, ?, ?, ?)
		 ON CONFLICT(provider, key) DO UPDATE SET payload=excluded.payload, fetchedAt=excluded.fetchedAt`,
		provider, k.String(), payload, snap.FetchedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("jirastore: save %s: %w", k, err)
	}
	return nil
}
