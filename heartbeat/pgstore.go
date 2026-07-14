package heartbeat

import (
	"context"
	"database/sql"
	"time"
)

// PGStore is the Postgres reference store. One row per service_id; last_seen is
// stamped server-side (now()) so all instances share the DB clock.
type PGStore struct{ db *sql.DB }

func NewPGStore(db *sql.DB) *PGStore { return &PGStore{db: db} }

var _ Store = (*PGStore)(nil)

// Migrate creates the table + a last_seen index (for the windowed Active/Prune).
func (s *PGStore) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS service_heartbeats (
	    service_id TEXT        PRIMARY KEY,
	    kind       TEXT        NOT NULL DEFAULT '',
	    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
	    meta       TEXT        NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS service_heartbeats_seen ON service_heartbeats (last_seen)`)
	return err
}

func (s *PGStore) Beat(ctx context.Context, hb Heartbeat) error {
	var started sql.NullTime
	if !hb.StartedAt.IsZero() {
		started = sql.NullTime{Time: hb.StartedAt.UTC(), Valid: true}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO service_heartbeats (service_id, kind, started_at, last_seen, meta)
		 VALUES ($1,$2,COALESCE($3, now()), now(), $4)
		 ON CONFLICT (service_id) DO UPDATE
		   SET kind=EXCLUDED.kind, started_at=EXCLUDED.started_at, last_seen=now(), meta=EXCLUDED.meta`,
		hb.ServiceID, hb.Kind, started, hb.Meta)
	return err
}

func (s *PGStore) Active(ctx context.Context, within time.Duration) ([]Heartbeat, error) {
	cutoff := time.Now().Add(-within).UTC()
	rows, err := s.db.QueryContext(ctx,
		`SELECT service_id, kind, started_at, last_seen, meta FROM service_heartbeats
		 WHERE last_seen > $1 ORDER BY kind, service_id`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Heartbeat
	for rows.Next() {
		var h Heartbeat
		if err := rows.Scan(&h.ServiceID, &h.Kind, &h.StartedAt, &h.LastSeen, &h.Meta); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *PGStore) Prune(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan).UTC()
	_, err := s.db.ExecContext(ctx, `DELETE FROM service_heartbeats WHERE last_seen < $1`, cutoff)
	return err
}
