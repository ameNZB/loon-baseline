// Package maintenance is loon-baseline's planned-maintenance mode: a persisted
// on/off flag with a reason + ETA, a middleware that serves a self-contained 503
// page while it's on, and an admin toggle view. It's for PLANNED downtime (you
// flip a switch to upgrade) — the app is up and chooses to show the page.
//
// UNPLANNED outages (a crash, a deploy in flight) are a different problem: a dead
// process can't serve its own page, so that fallback belongs in front, in the
// reverse proxy (see the Caddy sample in loon-demo-site/deploy). This package
// handles the planned case.
//
// The state is persisted (survives restarts, shared across web replicas) and
// refreshed on an interval, so a toggle on one replica reaches the others. The
// middleware is policy-free: the host passes an Allow predicate deciding who
// bypasses the page (admins, the toggle route, static assets, health checks), so
// the framework imposes no policy. The API tier simply doesn't install the
// middleware — it stays up while the web UI shows maintenance.
package maintenance

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// State is the persisted maintenance flag.
type State struct {
	Active    bool
	Reason    string
	StartedAt time.Time
	ETASecs   int // 0 = unknown
}

// Store persists the single maintenance State.
type Store interface {
	Get(ctx context.Context) (State, error)
	Set(ctx context.Context, s State) error
}

// Controller holds the current State in memory (read on every request via the
// middleware) backed by a Store. Restore loads it at boot; StartRefresh re-reads
// it periodically so a toggle on another replica propagates.
type Controller struct {
	store Store
	mu    sync.RWMutex
	state State
}

func NewController(store Store) *Controller { return &Controller{store: store} }

// Restore loads persisted state into memory. Call once at boot before serving.
func (c *Controller) Restore(ctx context.Context) error {
	s, err := c.store.Get(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	return nil
}

// StartRefresh re-reads the store every interval until ctx is done, so a toggle
// on one process reaches the others within the interval. Run in a goroutine.
func (c *Controller) StartRefresh(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s, err := c.store.Get(ctx); err == nil {
				c.mu.Lock()
				c.state = s
				c.mu.Unlock()
			}
		}
	}
}

// Snapshot returns the current state.
func (c *Controller) Snapshot() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Active reports whether maintenance is on.
func (c *Controller) Active() bool { return c.Snapshot().Active }

// Begin turns maintenance on (persisted + cached).
func (c *Controller) Begin(ctx context.Context, reason string, etaSecs int) error {
	s := State{Active: true, Reason: reason, StartedAt: time.Now(), ETASecs: etaSecs}
	if err := c.store.Set(ctx, s); err != nil {
		return err
	}
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	return nil
}

// End turns maintenance off.
func (c *Controller) End(ctx context.Context) error {
	s := State{}
	if err := c.store.Set(ctx, s); err != nil {
		return err
	}
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	return nil
}

// Middleware serves the 503 maintenance page while maintenance is on, except for
// requests allow returns true for — the host's bypass policy (admins, the toggle
// route, static assets, /healthz). allow may be nil (nobody bypasses). Install
// it in the web engine's chain; don't install it in the API tier, which should
// stay up.
func (c *Controller) Middleware(allow func(*gin.Context) bool) gin.HandlerFunc {
	return func(g *gin.Context) {
		s := c.Snapshot()
		if !s.Active || (allow != nil && allow(g)) {
			g.Next()
			return
		}
		if s.ETASecs > 0 {
			g.Header("Retry-After", strconv.Itoa(s.ETASecs))
		}
		g.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", renderPage(s))
		g.Abort()
	}
}
