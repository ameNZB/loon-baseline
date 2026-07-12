package password

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestProdSchemeInterop pins the wire-compatibility contract with the loon prod
// site's pkg/models/user.go (HMAC-SHA256 → StdEncoding → bcrypt cost 14): a hash
// made by either scheme must verify under the other, so prod can adopt this
// package without invalidating a single stored hash.
func TestProdSchemeInterop(t *testing.T) {
	pepper := []byte("prod-pepper-value")
	const pw = "correct horse battery staple"

	// Reproduce prod's exact peppered input + a cost-14 hash.
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(pw))
	peppered := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	prodHash, err := bcrypt.GenerateFromPassword([]byte(peppered), 14)
	if err != nil {
		t.Fatal(err)
	}

	h := Hasher{Pepper: pepper, Cost: 14}
	if !h.Check(string(prodHash), pw) {
		t.Fatal("baseline could not verify a prod-scheme hash — schemes diverge")
	}
	blHash, _ := h.Hash(pw)
	if bcrypt.CompareHashAndPassword([]byte(blHash), []byte(peppered)) != nil {
		t.Fatal("prod scheme could not verify a baseline hash")
	}
}

func TestHashCheck(t *testing.T) {
	h := Hasher{Pepper: []byte("pepper-key")}
	hash, err := h.Hash("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !h.Check(hash, "hunter2") {
		t.Fatal("correct password rejected")
	}
	if h.Check(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
}

func TestPepperRotation(t *testing.T) {
	old := Hasher{Pepper: []byte("old-pepper")}
	hash, _ := old.Hash("s3cret")

	// Rotated: current pepper differs; the old one is accepted as previous.
	rot := Hasher{Pepper: []byte("new-pepper"), PrevPepper: []byte("old-pepper")}
	ok, needsRehash := rot.Verify(hash, "s3cret")
	if !ok || !needsRehash {
		t.Fatalf("rotation verify = (%v,%v), want (true,true)", ok, needsRehash)
	}

	// Rehashing under the new pepper clears the needs-rehash signal.
	newHash, _ := rot.Hash("s3cret")
	if ok, needsRehash = rot.Verify(newHash, "s3cret"); !ok || needsRehash {
		t.Fatalf("post-rehash verify = (%v,%v), want (true,false)", ok, needsRehash)
	}

	// Wrong password fails under both peppers.
	if ok, _ := rot.Verify(hash, "nope"); ok {
		t.Fatal("wrong password accepted")
	}
}

func TestNoPepperIsBareBcrypt(t *testing.T) {
	h := Hasher{}
	hash, _ := h.Hash("plain")
	if !h.Check(hash, "plain") {
		t.Fatal("bare bcrypt check failed")
	}
}
