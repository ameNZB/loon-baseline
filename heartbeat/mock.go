package heartbeat

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Mock is an in-memory Store for tests and no-database hosts.
type Mock struct {
	mu sync.Mutex
	m  map[string]Heartbeat
}

func NewMock() *Mock { return &Mock{m: map[string]Heartbeat{}} }

var _ Store = (*Mock)(nil)

func (mk *Mock) Beat(_ context.Context, hb Heartbeat) error {
	mk.mu.Lock()
	defer mk.mu.Unlock()
	hb.LastSeen = time.Now()
	if hb.StartedAt.IsZero() {
		hb.StartedAt = hb.LastSeen
	}
	mk.m[hb.ServiceID] = hb
	return nil
}

func (mk *Mock) Active(_ context.Context, within time.Duration) ([]Heartbeat, error) {
	mk.mu.Lock()
	defer mk.mu.Unlock()
	cutoff := time.Now().Add(-within)
	var out []Heartbeat
	for _, h := range mk.m {
		if h.LastSeen.After(cutoff) {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ServiceID < out[j].ServiceID
	})
	return out, nil
}

func (mk *Mock) Prune(_ context.Context, olderThan time.Duration) error {
	mk.mu.Lock()
	defer mk.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	for id, h := range mk.m {
		if h.LastSeen.Before(cutoff) {
			delete(mk.m, id)
		}
	}
	return nil
}
