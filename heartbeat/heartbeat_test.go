package heartbeat

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBeatUpsertsAndActiveWindows(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	// Two beats for the same instance => one record, last_seen advances.
	_ = m.Beat(ctx, Heartbeat{ServiceID: "worker@a-1", Kind: "worker"})
	_ = m.Beat(ctx, Heartbeat{ServiceID: "worker@a-1", Kind: "worker"})
	_ = m.Beat(ctx, Heartbeat{ServiceID: "web@b-1", Kind: "web"})

	got, _ := m.Active(ctx, time.Minute)
	if len(got) != 2 {
		t.Fatalf("active = %d instances, want 2 (upserted)", len(got))
	}
	// Ordered by kind: web before worker.
	if got[0].Kind != "web" || got[1].Kind != "worker" {
		t.Fatalf("order = %s,%s want web,worker", got[0].Kind, got[1].Kind)
	}

	// A window shorter than the elapsed age excludes everything.
	time.Sleep(15 * time.Millisecond)
	if stale, _ := m.Active(ctx, 5*time.Millisecond); len(stale) != 0 {
		t.Fatalf("5ms window over 15ms-old beats should be empty, got %d", len(stale))
	}
}

func TestPruneRemovesStale(t *testing.T) {
	ctx := context.Background()
	m := NewMock()
	_ = m.Beat(ctx, Heartbeat{ServiceID: "x", Kind: "worker"})
	time.Sleep(15 * time.Millisecond)
	// Prune anything not seen in the last 5ms — the 15ms-old beat qualifies.
	_ = m.Prune(ctx, 5*time.Millisecond)
	if got, _ := m.Active(ctx, time.Hour); len(got) != 0 {
		t.Fatalf("stale beat should be pruned, got %d", len(got))
	}
}

func TestServicesViewRenders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	m := NewMock()
	_ = m.Beat(ctx, Heartbeat{ServiceID: "all@demo-1", Kind: "all", Meta: "loon-demo"})

	views, err := Views(m)
	if err != nil || len(views) != 1 {
		t.Fatalf("Views: err=%v n=%d", err, len(views))
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/admin/p/services", nil)
	html, err := views[0].Render(c)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	body := string(html)
	for _, want := range []string{"all@demo-1", "loon-demo", "1 online"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q:\n%s", want, body)
		}
	}
}

func TestHostID(t *testing.T) {
	id := HostID("worker")
	if !strings.HasPrefix(id, "worker@") {
		t.Fatalf("HostID = %q, want worker@... ", id)
	}
}
