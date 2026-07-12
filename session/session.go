// Package session is the loon host session layer, extracted VERBATIM from the
// production site's wiring (cmd/main.go + web/handlers/handlers.go) so a host
// that adopts it is cookie-compatible with a site already running the prod
// scheme — no logout wave, no CSRF break.
//
// Mechanism: gin-contrib/sessions with a cookie store — a server-signed session
// MAP (not a bare token), which is also where a double-submit CSRF token can
// live. A login stamps four keys:
//
//	user_id             int64  — who
//	login_at            int64  — unix; server-side expiry (MaxAge)
//	login_ip            string — hashed client IP at login (admin IP pinning)
//	password_changed_at int64  — unix; sessions older than the DB stamp are dead
//
// The key names and types are the production contract. Do not rename them.
package session

import (
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// Config builds the session middleware. Secret is required (≥32 bytes — the
// prod site log.Fatals below that; hosts should enforce the same).
type Config struct {
	Secret []byte
	// Name is the cookie name. Default "mysession" — the prod site's name, kept
	// so adopting this package preserves every live session.
	Name string
	// MaxAge is both the cookie lifetime and the server-side session lifetime
	// (enforced against login_at). Default 7 days.
	MaxAge time.Duration
	// Secure sets the Secure cookie flag (HTTPS-only). Off for plain-HTTP dev.
	Secure bool
}

func (cfg Config) name() string {
	if cfg.Name == "" {
		return "mysession"
	}
	return cfg.Name
}

// MaxAgeD returns the effective session lifetime.
func (cfg Config) MaxAgeD() time.Duration {
	if cfg.MaxAge <= 0 {
		return 7 * 24 * time.Hour
	}
	return cfg.MaxAge
}

// Middleware returns the sessions middleware; install it on the engine before
// any route that logs in or reads the user. Options mirror prod: SameSite Lax
// (cookie rides top-level GETs from external links so users stay logged in;
// cross-origin POSTs don't carry it, and a double-submit CSRF token covers the
// rest — see the prod comment at cmd/main.go:1139).
func (cfg Config) Middleware() gin.HandlerFunc {
	store := cookie.NewStore(cfg.Secret)
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   int(cfg.MaxAgeD().Seconds()),
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	return sessions.Sessions(cfg.name(), store)
}

// Issue stamps a logged-in session. It clears any pre-login content first
// (CSRF token, OAuth state) — starting from a known-clean state is the
// session-fixation defence-in-depth contract prod established.
//
// ipHash is the HASHED client IP ("" to skip IP pinning); pwChangedAt is the
// user's password_changed_at unix stamp (0 when the host has no such column).
func Issue(c *gin.Context, userID int64, ipHash string, pwChangedAt int64) error {
	s := sessions.Default(c)
	s.Clear()
	s.Set("user_id", userID)
	s.Set("login_at", time.Now().Unix())
	if ipHash != "" {
		s.Set("login_ip", ipHash)
	}
	s.Set("password_changed_at", pwChangedAt)
	return s.Save()
}

// Clear wipes the session (logout, or invalidation on a failed auth check —
// prod clears stale cookies rather than leaving them to re-fail every request).
func Clear(c *gin.Context) error {
	s := sessions.Default(c)
	s.Clear()
	return s.Save()
}

// Claims are the stamped values read back from the session.
type Claims struct {
	UserID            int64
	LoginAt           int64 // unix
	LoginIP           string
	PasswordChangedAt int64 // unix
}

// Read returns the session claims; ok is false when no user_id is stamped.
// Expiry and invalidation checks belong to the caller (webauth) — Read only
// decodes.
func Read(c *gin.Context) (Claims, bool) {
	s := sessions.Default(c)
	uid := idFromAny(s.Get("user_id"))
	if uid == 0 {
		return Claims{}, false
	}
	cl := Claims{UserID: uid}
	cl.LoginAt, _ = s.Get("login_at").(int64)
	cl.LoginIP, _ = s.Get("login_ip").(string)
	cl.PasswordChangedAt = idFromAny(s.Get("password_changed_at"))
	return cl, true
}

// idFromAny coerces the gob round-trip: values stored as int come back int,
// stored as int64 come back int64 (prod's sessionIDFromAny, generalized).
func idFromAny(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
