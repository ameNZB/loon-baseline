// Package password hashes and verifies passwords with bcrypt over an optional
// HMAC "pepper", and supports transparent pepper rotation. This is the exact
// scheme the loon prod site uses (pkg/models/user.go), lifted so any loon host
// gets the same battle-tested primitive instead of re-deriving it.
//
// The pepper is HMAC-SHA256(pepper, password) → base64 BEFORE bcrypt. That keeps
// the bcrypt input within its 72-byte limit regardless of password length and
// means a stolen database alone (without the pepper) can't be brute-forced.
package password

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// Hasher hashes/verifies under a current pepper, with an optional previous pepper
// for rotation. The zero value works (no pepper, default cost) — that is bare
// bcrypt, which is fine for a dev/demo host.
type Hasher struct {
	Pepper     []byte // current HMAC pepper; empty ⇒ bare bcrypt
	PrevPepper []byte // previous pepper, accepted on verify then rehashed
	Cost       int    // bcrypt cost (default bcrypt.DefaultCost)
}

func (h Hasher) cost() int {
	if h.Cost == 0 {
		return bcrypt.DefaultCost
	}
	return h.Cost
}

// peppered applies the HMAC pepper (passthrough when key is empty).
func peppered(pw string, key []byte) []byte {
	if len(key) == 0 {
		return []byte(pw)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(pw))
	sum := mac.Sum(nil)
	out := make([]byte, base64.StdEncoding.EncodedLen(len(sum)))
	base64.StdEncoding.Encode(out, sum)
	return out
}

// Hash returns a bcrypt hash of the peppered password under the current pepper.
func (h Hasher) Hash(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword(peppered(pw, h.Pepper), h.cost())
	return string(b), err
}

// Check reports whether pw matches hash under the current pepper.
func (h Hasher) Check(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), peppered(pw, h.Pepper)) == nil
}

// Verify checks pw under the current pepper, then the previous one. needsRehash
// is true when it matched only under the previous pepper — the caller should
// rewrite the stored hash with Hash() to migrate it forward.
func (h Hasher) Verify(hash, pw string) (ok, needsRehash bool) {
	if h.Check(hash, pw) {
		return true, false
	}
	if len(h.PrevPepper) > 0 &&
		bcrypt.CompareHashAndPassword([]byte(hash), peppered(pw, h.PrevPepper)) == nil {
		return true, true
	}
	return false, false
}
