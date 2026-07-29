// Package middleware holds the dispatcher middlewares: logging, db, user,
// throttle, i18n, and callback debounce, applied in this fixed order (see
// app/main.py — the order is load-bearing: db before user before throttle,
// so throttle can read the user row user middleware just attached).
package middleware

import (
	"sync"
	"time"
)

// TokenBucket is a per-key token bucket for chat-level rate limiting (e.g.
// per Telegram user_id) — port of app/utils/ratelimit.py's PerUserTokenBucket.
//
// Buckets are created lazily; idle buckets (untouched for over an hour) are
// swept out periodically so a burst of distinct user_ids can't grow this map
// without bound.
type TokenBucket struct {
	mu            sync.Mutex
	ratePerMinute float64
	buckets       map[int64]bucketState
	callsSweep    int
}

type bucketState struct {
	tokens     float64
	lastRefill time.Time
}

const (
	idleEvictInterval = time.Hour
	sweepEveryNCalls  = 500
)

func NewTokenBucket(ratePerMinute int) *TokenBucket {
	return &TokenBucket{
		ratePerMinute: float64(ratePerMinute),
		buckets:       make(map[int64]bucketState),
	}
}

// Allow reports whether the action for key is allowed right now, consuming
// a token if so.
func (b *TokenBucket) Allow(key int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.callsSweep++
	if b.callsSweep >= sweepEveryNCalls {
		b.callsSweep = 0
		b.evictIdleLocked(now)
	}

	state, ok := b.buckets[key]
	if !ok {
		state = bucketState{tokens: b.ratePerMinute, lastRefill: now}
	}

	elapsed := now.Sub(state.lastRefill).Seconds()
	refillRate := b.ratePerMinute / 60.0
	tokens := state.tokens + elapsed*refillRate
	if tokens > b.ratePerMinute {
		tokens = b.ratePerMinute
	}

	if tokens >= 1 {
		tokens--
		b.buckets[key] = bucketState{tokens: tokens, lastRefill: now}
		return true
	}
	b.buckets[key] = bucketState{tokens: tokens, lastRefill: now}
	return false
}

func (b *TokenBucket) evictIdleLocked(now time.Time) {
	cutoff := now.Add(-idleEvictInterval)
	for key, state := range b.buckets {
		if state.lastRefill.Before(cutoff) {
			delete(b.buckets, key)
		}
	}
}

// Debouncer is a per-key minimum-interval gate — blocks a second call within
// interval, independent of the broader per-minute rate limits. Port of
// app/utils/ratelimit.py's Debouncer.
type Debouncer struct {
	mu         sync.Mutex
	interval   time.Duration
	lastCall   map[int64]time.Time
	callsSweep int
}

func NewDebouncer(interval time.Duration) *Debouncer {
	return &Debouncer{interval: interval, lastCall: make(map[int64]time.Time)}
}

// Allow reports whether the action for key is allowed right now, recording
// it if so.
func (d *Debouncer) Allow(key int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.callsSweep++
	if d.callsSweep >= sweepEveryNCalls {
		d.callsSweep = 0
		cutoff := now.Add(-idleEvictInterval)
		for k, last := range d.lastCall {
			if last.Before(cutoff) {
				delete(d.lastCall, k)
			}
		}
	}

	if last, ok := d.lastCall[key]; ok && now.Sub(last) < d.interval {
		return false
	}
	d.lastCall[key] = now
	return true
}
