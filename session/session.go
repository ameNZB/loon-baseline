// Package session issues and verifies stateless, HMAC-signed session cookies —
// the plumbing every loon host needs and that the framework deliberately leaves
// to the host (loon/core has no login/session seam by design). A token carries
// the user id, an issue time (for server-side expiry), and an "epoch" the host
// can bump to invalidate every outstanding session after a password change.
//
// Extracted from the loon demo site's inline cookie code so the demo and a real
// site share one implementation.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Manager signs and reads session cookies with a single HMAC key. The zero value
// is unusable — set Secret. Cookie/MaxAge/Path fall back to sane defaults.
type Manager struct {
	Secret []byte        // HMAC-SHA256 key (required; ≥32 bytes recommended)
	Cookie string        // cookie name (default "loon_session")
	MaxAge time.Duration // cookie + server-side lifetime (default 7 days)
	Secure bool          // set the Secure flag (HTTPS-only); false for plain-HTTP dev
	Path   string        // cookie path (default "/")
}

// Claims are the verified contents of a session token.
type Claims struct {
	UserID int64
	Issued time.Time
	Epoch  int64 // host-defined; mismatch vs the user's current epoch ⇒ session is stale
}

func (m Manager) cookie() string { if m.Cookie == "" { return "loon_session" }; return m.Cookie }
func (m Manager) path() string   { if m.Path == "" { return "/" }; return m.Path }
func (m Manager) maxAge() time.Duration {
	if m.MaxAge <= 0 {
		return 7 * 24 * time.Hour
	}
	return m.MaxAge
}

// Issue sets a signed session cookie for the user.
func (m Manager) Issue(c *gin.Context, userID, epoch int64) {
	c.SetCookie(m.cookie(), m.token(userID, epoch), int(m.maxAge().Seconds()), m.path(), "", m.Secure, true)
}

// Clear removes the session cookie (logout).
func (m Manager) Clear(c *gin.Context) {
	c.SetCookie(m.cookie(), "", -1, m.path(), "", m.Secure, true)
}

// Read verifies the request's session cookie and returns its claims. ok is false
// when the cookie is missing, tampered, or past MaxAge.
func (m Manager) Read(c *gin.Context) (Claims, bool) {
	raw, err := c.Cookie(m.cookie())
	if err != nil || raw == "" {
		return Claims{}, false
	}
	payloadB64, sig, found := strings.Cut(raw, ".")
	if !found {
		return Claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return Claims{}, false
	}
	if !hmac.Equal([]byte(sig), []byte(m.mac(payload))) {
		return Claims{}, false
	}
	uid, issued, epoch, ok := parsePayload(string(payload))
	if !ok {
		return Claims{}, false
	}
	c2 := Claims{UserID: uid, Issued: time.Unix(issued, 0), Epoch: epoch}
	if time.Since(c2.Issued) > m.maxAge() {
		return Claims{}, false
	}
	return c2, true
}

// token = base64url(payload) + "." + base64url(hmac(payload)), where payload is
// "uid|issued|epoch". Base64-encoding the payload keeps the '.' split unambiguous.
func (m Manager) token(userID, epoch int64) string {
	payload := strconv.FormatInt(userID, 10) + "|" +
		strconv.FormatInt(time.Now().Unix(), 10) + "|" +
		strconv.FormatInt(epoch, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + m.mac([]byte(payload))
}

func (m Manager) mac(payload []byte) string {
	h := hmac.New(sha256.New, m.Secret)
	h.Write(payload)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func parsePayload(s string) (uid, issued, epoch int64, ok bool) {
	parts := strings.Split(s, "|")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if uid, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if issued, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if epoch, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	return uid, issued, epoch, true
}
