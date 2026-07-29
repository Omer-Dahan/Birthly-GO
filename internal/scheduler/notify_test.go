package scheduler

import (
	"testing"
	"time"
)

// Tests the outbound send-rate limiter (sendRateLimiter/newSendRateLimiter),
// the port of app/utils/ratelimit.py's AsyncRateLimiter used for the global
// cap on bot.send_message calls/sec. Distinct from
// internal/bot/middleware.TokenBucket, which throttles inbound commands per
// user — a different concern already covered by
// internal/bot/middleware/ratelimit_test.go.
//
// sendLimiter()'s process-wide sync.Once singleton makes the *shared*
// limiter unsuitable for isolated per-test rate assertions (whichever test
// runs first in the binary "wins" the rate), so these tests construct a
// fresh limiter directly via newSendRateLimiter instead.

func TestSendRateLimiter_AcquireConsumesOneToken(t *testing.T) {
	l := newSendRateLimiter(10)
	before := l.tokens
	l.Acquire()
	if before-l.tokens < 0.9 || before-l.tokens > 1.1 {
		t.Errorf("Acquire consumed %.2f tokens, want ~1", before-l.tokens)
	}
}

func TestSendRateLimiter_BurstUpToCapacityDoesNotBlock(t *testing.T) {
	l := newSendRateLimiter(5)
	start := time.Now()
	for i := 0; i < 5; i++ {
		l.Acquire()
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Errorf("acquiring up to capacity (5) took %v, want near-instant (no blocking)", elapsed)
	}
}

func TestSendRateLimiter_BlocksWhenExhausted(t *testing.T) {
	l := newSendRateLimiter(20) // 1 token every 50ms
	for i := 0; i < 20; i++ {
		l.Acquire() // drain the initial burst capacity
	}

	start := time.Now()
	l.Acquire() // must now wait ~1/20s for a token to refill
	elapsed := time.Since(start)

	if elapsed < 20*time.Millisecond {
		t.Errorf("Acquire on an exhausted limiter returned in %v, want it to block for refill", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Acquire blocked for %v, want well under a second", elapsed)
	}
}
