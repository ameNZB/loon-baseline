package password

import "testing"

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
