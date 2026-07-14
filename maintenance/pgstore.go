package maintenance

import (
	"context"
	"database/sql"
)

// PGStore persists the single maintenance row (id = 1). A host with its own
// settings store implements Store directly and ignores this.
type PGStore struct{ db *sql.DB }

func NewPGStore(db *sql.DB) *PGStore { return &PGStore{db: db} }

var _ Store = (*PGStore)(nil)

// Migrate creates the single-row maintenance_state table (idempotent).
func (s *PGStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS maintenance_state (
	    id         INT         PRIMARY KEY DEFAULT 1 CHECK (id = 1),
	    active     BOOLEAN     NOT NULL DEFAULT false,
	    reason     TEXT        NOT NULL DEFAULT '',
	    started_at TIMESTAMPTZ,
	    eta_secs   INT         NOT NULL DEFAULT 0
	)`)
	return err
}

func (s *PGStore) Get(ctx context.Context) (State, error) {
	var st State
	var started sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT active, reason, started_at, eta_secs FROM maintenance_state WHERE id = 1`).
		Scan(&st.Active, &st.Reason, &started, &st.ETASecs)
	if err == sql.ErrNoRows {
		return State{}, nil // no row yet = maintenance off
	}
	if err != nil {
		return State{}, err
	}
	if started.Valid {
		st.StartedAt = started.Time
	}
	return st, nil
}

func (s *PGStore) Set(ctx context.Context, st State) error {
	var started sql.NullTime
	if !st.StartedAt.IsZero() {
		started = sql.NullTime{Time: st.StartedAt.UTC(), Valid: true}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO maintenance_state (id, active, reason, started_at, eta_secs)
		 VALUES (1,$1,$2,$3,$4)
		 ON CONFLICT (id) DO UPDATE SET active=EXCLUDED.active, reason=EXCLUDED.reason,
		   started_at=EXCLUDED.started_at, eta_secs=EXCLUDED.eta_secs`,
		st.Active, st.Reason, started, st.ETASecs)
	return err
}
