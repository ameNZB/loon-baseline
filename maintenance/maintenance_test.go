package maintenance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestControllerToggle(t *testing.T) {
	ctx := context.Background()
	c := NewController(NewMock())
	if c.Active() {
		t.Fatal("should start off")
	}
	if err := c.Begin(ctx, "upgrade", 300); err != nil {
		t.Fatal(err)
	}
	if !c.Active() {
		t.Fatal("should be on after Begin")
	}
	if s := c.Snapshot(); s.Reason != "upgrade" || s.ETASecs != 300 || s.StartedAt.IsZero() {
		t.Fatalf("state = %+v", s)
	}
	if err := c.End(ctx); err != nil {
		t.Fatal(err)
	}
	if c.Active() {
		t.Fatal("should be off after End")
	}
}

func TestRestoreLoadsPersistedState(t *testing.T) {
	ctx := context.Background()
	store := NewMock()
	_ = store.Set(ctx, State{Active: true, Reason: "seeded"})

	c := NewController(store)
	if c.Active() {
		t.Fatal("not yet restored — should read off until Restore")
	}
	if err := c.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.Active() || c.Snapshot().Reason != "seeded" {
		t.Fatalf("Restore didn't load state: %+v", c.Snapshot())
	}
}

func TestMiddlewareGatesExceptAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	c := NewController(NewMock())
	r := gin.New()
	// Bypass policy: /admin is always reachable so the operator can toggle off.
	r.Use(c.Middleware(func(g *gin.Context) bool { return strings.HasPrefix(g.Request.URL.Path, "/admin") }))
	r.GET("/", func(g *gin.Context) { g.String(200, "home") })
	r.GET("/admin/x", func(g *gin.Context) { g.String(200, "admin") })

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	// Off: everything serves.
	if w := get("/"); w.Code != 200 {
		t.Fatalf("off: / = %d, want 200", w.Code)
	}

	// On: normal paths get the 503 page; /admin bypasses.
	_ = c.Begin(ctx, "upgrade", 120)
	w := get("/")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "maintenance") {
		t.Fatalf("on: / = %d (has page=%v)", w.Code, strings.Contains(w.Body.String(), "maintenance"))
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("on: expected Retry-After header")
	}
	if w := get("/admin/x"); w.Code != 200 {
		t.Fatalf("on: /admin/x = %d, want 200 (bypass)", w.Code)
	}

	// Off again: normal path serves.
	_ = c.End(ctx)
	if w := get("/"); w.Code != 200 {
		t.Fatalf("after end: / = %d, want 200", w.Code)
	}
}
