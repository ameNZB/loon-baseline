// Package webauth turns a session.Manager + a host user-resolver into the
// current-user middleware and role gates a loon host needs, and wires them into
// loon's core.AuthService seam. It stores the resolved *core.User in the gin
// context; the host reads it for rendering and authorization.
//
// This is the reusable core of the prod site's SoftAuth/AuthRequired, minus the
// product-specific enrichment (inbox badges, admin pins, IP-pinning) that stays
// in the host. Extracted so the demo and prod share one implementation.
package webauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ameNZB/loon/core"

	"github.com/ameNZB/loon-baseline/session"
)

// Resolver loads the host's user for a verified session id. epoch is the user's
// current password epoch — return the value stamped into the session at login so
// a mismatch invalidates the session (return 0 to disable that check). ok is
// false when the user no longer exists.
type Resolver func(ctx context.Context, id int64) (u *core.User, epoch int64, ok bool)

// Auth is the middleware bundle. Set Session + Resolve; LoginPath defaults to
// "/login".
type Auth struct {
	Session   session.Manager
	Resolve   Resolver
	LoginPath string
}

const ctxUser = "loon.user"

func (a Auth) login() string {
	if a.LoginPath == "" {
		return "/login"
	}
	return a.LoginPath
}

// resolve verifies the session cookie and loads the user, enforcing the epoch
// (password-change) invalidation contract.
func (a Auth) resolve(c *gin.Context) (*core.User, bool) {
	claims, ok := a.Session.Read(c)
	if !ok {
		return nil, false
	}
	u, epoch, ok := a.Resolve(c.Request.Context(), claims.UserID)
	if !ok || u == nil {
		return nil, false
	}
	if epoch != 0 && epoch != claims.Epoch {
		return nil, false // password changed since this session was issued
	}
	return u, true
}

// Soft loads the session user into the context when present, but never blocks —
// for public pages that render differently when logged in.
func (a Auth) Soft() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u, ok := a.resolve(c); ok {
			c.Set(ctxUser, u)
		}
		c.Next()
	}
}

// Require aborts the request when there is no user OR the user is below minRole.
// Browser requests are redirected to LoginPath; API/curl requests get JSON.
func (a Auth) Require(minRole core.Role) gin.HandlersChain {
	return gin.HandlersChain{func(c *gin.Context) {
		u, ok := a.resolve(c)
		if !ok {
			if wantsHTML(c) {
				c.Redirect(http.StatusSeeOther, a.login())
				c.Abort()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "log in first"})
			return
		}
		if u.Role < minRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "insufficient role"})
			return
		}
		c.Set(ctxUser, u)
		c.Next()
	}}
}

// RequireExact aborts unless the user has exactly the given role (the
// core.AuthAdapter.RequireRoleFn shape). Prefer Require for AtLeast semantics.
func (a Auth) RequireExact(role core.Role) gin.HandlersChain {
	return gin.HandlersChain{func(c *gin.Context) {
		u, ok := a.resolve(c)
		if !ok || u.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "wrong role"})
			return
		}
		c.Set(ctxUser, u)
		c.Next()
	}}
}

// Current returns the request's user: the one a middleware already resolved into
// the context, or a fresh resolve from the cookie (so it works on routes with no
// auth middleware, e.g. public pages that render the navbar).
func (a Auth) Current(c *gin.Context) (*core.User, bool) {
	if v, ok := c.Get(ctxUser); ok {
		if u, ok := v.(*core.User); ok {
			return u, true
		}
	}
	return a.resolve(c)
}

func wantsHTML(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "text/html")
}
