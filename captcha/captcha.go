// Package captcha is a reusable Cloudflare Turnstile hook for loon-baseline
// hosts: verify tokens server-side, render the widget into any form, and gate
// any POST route with middleware. Attach it to login, register, or an action
// button (e.g. a daily-reward claim).
//
// DISABLED WHEN UNCONFIGURED: with an empty Secret the verifier passes every
// check and Widget renders nothing — so a dev/demo without Cloudflare keys
// still works. Set SiteKey + Secret (prod) to enforce. When enabled it is
// fail-CLOSED: a missing/invalid token, or a transport failure reaching
// Cloudflare, is rejected.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// FormField is the request field Turnstile populates with the solved token.
const FormField = "cf-turnstile-response"

const siteVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// ErrFailed is returned by Verify when an enabled captcha rejects the token.
var ErrFailed = errors.New("captcha: verification failed")

// Config holds the Turnstile key pair. Empty Secret = disabled (see package doc).
type Config struct {
	SiteKey string // public; embedded in the page widget
	Secret  string // server-side siteverify key
}

// Verifier verifies Turnstile tokens and renders the widget. Construct once at
// boot and share it; it is safe for concurrent use.
type Verifier struct {
	cfg       Config
	client    *http.Client
	verifyURL string // overridable in tests
}

// New builds a Verifier. A nil *Verifier is safe to use (it reads as disabled).
func New(cfg Config) *Verifier {
	return &Verifier{
		cfg:       cfg,
		client:    &http.Client{Timeout: 10 * time.Second},
		verifyURL: siteVerifyURL,
	}
}

// Enabled reports whether a secret is configured (verification enforced).
func (v *Verifier) Enabled() bool { return v != nil && v.cfg.Secret != "" }

// Verify checks a Turnstile token server-side. It returns nil when the captcha
// is disabled or the token is valid, ErrFailed when an enabled captcha rejects
// a missing/invalid token, and a wrapped error on a transport/parse failure
// reaching Cloudflare (fail-closed while enabled).
func (v *Verifier) Verify(ctx context.Context, token, remoteIP string) error {
	if !v.Enabled() {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return ErrFailed
	}
	form := url.Values{"secret": {v.cfg.Secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("captcha: siteverify: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("captcha: decode siteverify: %w", err)
	}
	if !out.Success {
		return ErrFailed
	}
	return nil
}

// Middleware gates a POST route: it reads the token from the standard form
// field (or the Cf-Turnstile-Response header for XHR) and aborts 403 on
// failure. No-op when disabled. Use it for action buttons / API-style
// endpoints; for login/register prefer calling Verify inline so you can
// re-render the form with a friendly message.
func (v *Verifier) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !v.Enabled() {
			c.Next()
			return
		}
		token := c.PostForm(FormField)
		if token == "" {
			token = c.GetHeader("Cf-Turnstile-Response")
		}
		if err := v.Verify(c.Request.Context(), token, c.ClientIP()); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "error": "captcha verification failed"})
			return
		}
		c.Next()
	}
}

type options struct{ theme, size, appearance string }

// Option tunes the rendered widget.
type Option func(*options)

// Theme sets data-theme (auto | light | dark). Size sets data-size
// (normal | flexible | compact). Appearance sets data-appearance
// (always | execute | interaction-only — the last is the "invisible until
// challenged" mode).
func Theme(t string) Option      { return func(o *options) { o.theme = t } }
func Size(s string) Option       { return func(o *options) { o.size = s } }
func Appearance(a string) Option { return func(o *options) { o.appearance = a } }

// Widget returns the Turnstile widget HTML (a div plus the API script) to embed
// inside a form, or "" when disabled. On solve, Turnstile injects the
// cf-turnstile-response token into the enclosing form. The site key is a
// trusted config value, so the markup is returned as safe template.HTML.
func (v *Verifier) Widget(opts ...Option) template.HTML {
	if !v.Enabled() {
		return ""
	}
	o := options{theme: "auto", size: "flexible", appearance: "always"}
	for _, fn := range opts {
		fn(&o)
	}
	div := fmt.Sprintf(
		`<div class="cf-turnstile" data-sitekey=%q data-theme=%q data-size=%q data-appearance=%q></div>`,
		v.cfg.SiteKey, o.theme, o.size, o.appearance)
	const script = `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`
	return template.HTML(div + script)
}
