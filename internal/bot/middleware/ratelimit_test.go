package middleware

import (
	"testing"
	"time"
)

func TestTokenBucket_AllowsUpToCapacityThenBlocks(t *testing.T) {
	b := NewTokenBucket(3) // 3/minute
	for i := 0; i < 3; i++ {
		if !b.Allow(1) {
			t.Fatalf("call %d should be allowed (within capacity)", i)
		}
	}
	if b.Allow(1) {
		t.Error("4th immediate call should be blocked")
	}
}

func TestTokenBucket_PerKeyIsolation(t *testing.T) {
	b := NewTokenBucket(1)
	if !b.Allow(1) {
		t.Fatal("first call for key 1 should be allowed")
	}
	if !b.Allow(2) {
		t.Error("key 2 should have its own bucket, unaffected by key 1")
	}
}

func TestDebouncer_BlocksWithinInterval(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)
	if !d.Allow(1) {
		t.Fatal("first call should be allowed")
	}
	if d.Allow(1) {
		t.Error("immediate second call should be debounced")
	}
	time.Sleep(60 * time.Millisecond)
	if !d.Allow(1) {
		t.Error("call after interval elapsed should be allowed")
	}
}
