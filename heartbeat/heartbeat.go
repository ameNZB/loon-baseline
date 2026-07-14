// Package heartbeat is loon-baseline's process-presence layer: each process
// (web / api / worker) upserts a heartbeat row on an interval, and an admin
// "Services online" view lists who's checked in recently. It answers "how many
// workers are online?" and gives a distributed deployment operational
// visibility — coordinating through shared state (a table), no message bus
// (LOON-DISTRIBUTED).
//
// It complements /healthz: the health endpoint tells a load balancer "this
// process answers HTTP right now"; the heartbeat tells an operator "this
// instance (kind, uptime, last seen) is part of the fleet". The read tier, which
// may run against a read replica, is best watched by the LB health check rather
// than a DB heartbeat; the worker and web tiers (which write anyway) report here.
package heartbeat

import (
	"context"
	"os"
	"strconv"
	"time"
)

// Heartbeat is one process instance's presence record.
type Heartbeat struct {
	ServiceID string    // unique per process instance
	Kind      string    // "web" | "api" | "worker" | "all"
	StartedAt time.Time // process start (for uptime)
	LastSeen  time.Time // set by the store on each Beat
	Meta      string    // optional freeform (version, host)
}

// Store persists heartbeats.
type Store interface {
	// Beat upserts hb by ServiceID, stamping last_seen = now.
	Beat(ctx context.Context, hb Heartbeat) error
	// Active returns instances last seen within the window, ordered by kind then id.
	Active(ctx context.Context, within time.Duration) ([]Heartbeat, error)
	// Prune deletes instances not seen for olderThan (dead processes).
	Prune(ctx context.Context, olderThan time.Duration) error
}

// HostID builds a per-instance id "<kind>@<hostname>-<pid>". In a container the
// hostname is the container id, so instances are distinct.
func HostID(kind string) string {
	h, _ := os.Hostname()
	if h == "" {
		h = "host"
	}
	return kind + "@" + h + "-" + strconv.Itoa(os.Getpid())
}

// StartReporter beats immediately, then every interval until ctx is done. Run
// one per process in a goroutine. Best-effort: a failed beat is retried next
// tick (a process that can't reach the DB simply ages out of Active).
func StartReporter(ctx context.Context, store Store, serviceID, kind, meta string, interval time.Duration) {
	started := time.Now()
	beat := func() {
		_ = store.Beat(ctx, Heartbeat{ServiceID: serviceID, Kind: kind, StartedAt: started, Meta: meta})
	}
	beat()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}
