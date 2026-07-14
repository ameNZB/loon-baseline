package heartbeat

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Exercises the real Postgres upsert + windowed Active + Prune. Skipped unless
// LOON_TEST_DSN is set.
func TestPGStoreAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("LOON_TEST_DSN")
	if dsn == "" {
		t.Skip("LOON_TEST_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS service_heartbeats`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	s := NewPGStore(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	start := time.Now().Add(-90 * time.Second)
	_ = s.Beat(ctx, Heartbeat{ServiceID: "worker@a-1", Kind: "worker", StartedAt: start})
	_ = s.Beat(ctx, Heartbeat{ServiceID: "worker@a-1", Kind: "worker", StartedAt: start}) // upsert
	_ = s.Beat(ctx, Heartbeat{ServiceID: "web@b-1", Kind: "web"})

	got, err := s.Active(ctx, time.Minute)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("active = %d, want 2 (upserted)", len(got))
	}
	// Upserted row keeps its started_at (uptime ~90s), last_seen is fresh.
	var worker Heartbeat
	for _, h := range got {
		if h.ServiceID == "worker@a-1" {
			worker = h
		}
	}
	if time.Since(worker.StartedAt) < 60*time.Second {
		t.Fatalf("started_at should be ~90s old, got %v", time.Since(worker.StartedAt))
	}
	if time.Since(worker.LastSeen) > 10*time.Second {
		t.Fatalf("last_seen should be fresh, got %v", time.Since(worker.LastSeen))
	}

	// Prune everything, then Active is empty.
	if err := s.Prune(ctx, 0); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if again, _ := s.Active(ctx, time.Hour); len(again) != 0 {
		t.Fatalf("after prune(0), active = %d, want 0", len(again))
	}
}
