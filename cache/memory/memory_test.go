package memory

import (
	"context"
	"testing"
	"time"
)

func TestGetSetDelete(t *testing.T) {
	ctx := context.Background()
	c := New()

	// miss
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("expected miss")
	}
	// set + hit
	_ = c.Set(ctx, "k", []byte("v"), time.Minute)
	if b, ok, _ := c.Get(ctx, "k"); !ok || string(b) != "v" {
		t.Fatalf("get = %q ok=%v", b, ok)
	}
	// delete
	_ = c.Delete(ctx, "k")
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("expected miss after delete")
	}
}

func TestExpiry(t *testing.T) {
	ctx := context.Background()
	c := New()
	_ = c.Set(ctx, "k", []byte("v"), 20*time.Millisecond)
	if _, ok, _ := c.Get(ctx, "k"); !ok {
		t.Fatal("should be present before expiry")
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("should be expired")
	}
}

func TestReturnedBytesAreACopy(t *testing.T) {
	ctx := context.Background()
	c := New()
	_ = c.Set(ctx, "k", []byte("orig"), time.Minute)
	b, _, _ := c.Get(ctx, "k")
	b[0] = 'X' // mutate the returned slice
	if b2, _, _ := c.Get(ctx, "k"); string(b2) != "orig" {
		t.Fatalf("cached value was mutated: %q", b2)
	}
}
