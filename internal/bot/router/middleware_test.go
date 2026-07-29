package router

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/config"
)

func throttleTestConfig() *config.Config {
	return &config.Config{
		RateLimitMessages: 1000, RateLimitCallbacks: 1000,
		NewAccountRateLimitMessages: 1000, NewAccountRateLimitCallbacks: 1000,
		NewAccountGraceHours: 0,
		DefaultLanguage:      "he",
	}
}

// TestThrottleHandler_SilencesAfterLimitExceeded confirms the basic
// behavior this leak-prevention sweep must not disturb: a user who trips
// their rate limit gets silenced (EndGroups, no further processing) until
// silenceSeconds elapses.
func TestThrottleHandler_SilencesAfterLimitExceeded(t *testing.T) {
	cfg := throttleTestConfig()
	cfg.RateLimitMessages = 1 // one token, no time to refill within the test
	h := newThrottleHandler(cfg)

	client := &fakeBotClient{}
	bot := testBot(client)
	mkCtx := func() *ext.Context {
		return &ext.Context{
			Update:        &gotgbot.Update{},
			Data:          map[string]any{},
			EffectiveUser: &gotgbot.User{Id: 42},
			EffectiveChat: &gotgbot.Chat{Id: 42},
		}
	}

	if err := h.HandleUpdate(bot, mkCtx()); err != nil {
		t.Fatalf("first message (within limit): unexpected result %v", err)
	}
	if err := h.HandleUpdate(bot, mkCtx()); err != ext.EndGroups {
		t.Fatalf("second message (over limit) = %v, want ext.EndGroups (silenced)", err)
	}

	h.mu.Lock()
	_, silenced := h.silencedUntil[42]
	h.mu.Unlock()
	if !silenced {
		t.Error("expected user 42 to have a silencedUntil entry after tripping the limit")
	}
}

// TestThrottleHandler_SweepEvictsStaleSilencedEntries is the actual
// regression test for the leak: a user silenced once and never seen again
// must not stay in silencedUntil forever. Unlike the delete() in
// HandleUpdate (which only fires for a user who comes back), the periodic
// sweep must clean up entries for users who never return.
func TestThrottleHandler_SweepEvictsStaleSilencedEntries(t *testing.T) {
	h := newThrottleHandler(throttleTestConfig())

	// Seed 1000 already-expired silenced entries directly, simulating 1000
	// distinct users who each got rate-limited once and never came back.
	past := time.Now().Add(-time.Hour)
	h.mu.Lock()
	for i := int64(0); i < 1000; i++ {
		h.silencedUntil[i] = past
	}
	h.mu.Unlock()

	client := &fakeBotClient{}
	bot := testBot(client)

	// Drive throttleSweepEveryN calls from an unrelated user to cross the
	// sweep threshold — the sweep walks the whole map regardless of which
	// key triggered it.
	for i := 0; i < throttleSweepEveryN; i++ {
		ctx := &ext.Context{
			Update:        &gotgbot.Update{},
			Data:          map[string]any{},
			EffectiveUser: &gotgbot.User{Id: 999999},
			EffectiveChat: &gotgbot.Chat{Id: 999999},
		}
		if err := h.HandleUpdate(bot, ctx); err != nil {
			t.Fatalf("HandleUpdate call %d: unexpected result %v", i, err)
		}
	}

	h.mu.Lock()
	remaining := len(h.silencedUntil)
	h.mu.Unlock()
	if remaining > 1 { // the driving user itself must never get silenced here
		t.Errorf("silencedUntil has %d entries after the sweep threshold, want the 1000 stale entries evicted", remaining)
	}
}
