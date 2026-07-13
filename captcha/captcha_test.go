package captcha

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisabled(t *testing.T) {
	d := New(Config{}) // no secret
	if d.Enabled() {
		t.Fatal("no secret => disabled")
	}
	// disabled passes every check, even an empty token
	if err := d.Verify(context.Background(), "", ""); err != nil {
		t.Fatalf("disabled Verify should pass: %v", err)
	}
	if w := d.Widget(); w != "" {
		t.Fatalf("disabled Widget should be empty, got %q", w)
	}
	// a nil verifier is safe and reads as disabled
	var n *Verifier
	if n.Enabled() {
		t.Fatal("nil verifier should be disabled")
	}
	if err := n.Verify(context.Background(), "", ""); err != nil {
		t.Fatalf("nil Verify should pass: %v", err)
	}
}

func TestVerifyEnabled(t *testing.T) {
	var gotSecret, gotResp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotSecret, gotResp = r.FormValue("secret"), r.FormValue("response")
		if gotResp == "good" {
			io.WriteString(w, `{"success":true}`)
			return
		}
		io.WriteString(w, `{"success":false,"error-codes":["invalid-input-response"]}`)
	}))
	defer srv.Close()

	v := New(Config{SiteKey: "site-abc", Secret: "secret-xyz"})
	v.verifyURL = srv.URL
	if !v.Enabled() {
		t.Fatal("secret set => enabled")
	}

	// empty token is rejected WITHOUT calling Cloudflare
	if err := v.Verify(context.Background(), "  ", ""); !errors.Is(err, ErrFailed) {
		t.Fatalf("empty token err = %v, want ErrFailed", err)
	}

	// valid token passes and forwards secret + response to siteverify
	if err := v.Verify(context.Background(), "good", "1.2.3.4"); err != nil {
		t.Fatalf("good token: %v", err)
	}
	if gotSecret != "secret-xyz" || gotResp != "good" {
		t.Fatalf("siteverify received secret=%q response=%q", gotSecret, gotResp)
	}

	// invalid token is rejected
	if err := v.Verify(context.Background(), "bad", ""); !errors.Is(err, ErrFailed) {
		t.Fatalf("bad token err = %v, want ErrFailed", err)
	}

	// widget renders and carries the (public) site key + the token field name
	w := string(v.Widget())
	if !strings.Contains(w, "site-abc") || !strings.Contains(w, "cf-turnstile") {
		t.Fatalf("widget missing sitekey/class: %q", w)
	}
}
