package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"birthly/internal/store/models"
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

// TestSendReminder_UsesSendPhotoWhenEventHasPhoto covers the new behavior:
// an event with a photo_file_id gets its reminder delivered via SendPhoto
// (file_id + reminder text as caption) instead of a plain SendMessage, so
// the photo the user attached during event editing actually shows up in the
// day-of push. This is a deliberate divergence from app/scheduler/notify.py,
// which never sends event.photo_file_id anywhere.
func TestSendReminder_UsesSendPhotoWhenEventHasPhoto(t *testing.T) {
	db := testDB(t)
	offset := 0
	sendTime := "09:00"
	user, event := seedUserEventRule(t, db, 1, "Asia/Jerusalem", sendTime, &offset, &sendTime, nil)

	photoID := "AgADabc123"
	event.PhotoFileID = &photoID
	rule := &models.ReminderRule{OffsetDays: &offset, SendTime: &sendTime, Enabled: true, EventID: &event.ID}

	client := &fakeBotClient{}
	bot := testBot(client)

	if ok := SendReminder(context.Background(), bot, db, user, event, rule, 1, *event.NextOccurrence, event.CalendarType, 20); !ok {
		t.Fatal("SendReminder returned false, want true")
	}

	if len(client.calls) != 1 || client.calls[0]["method"] != "sendPhoto" {
		t.Fatalf("calls = %+v, want exactly one sendPhoto call", client.calls)
	}
	params := client.calls[0]["params"].(map[string]any)
	// params["photo"] is an *gotgbot.FileReader wrapping the file_id in an
	// unexported field, so compare via its string form rather than the value directly.
	if got := fmt.Sprintf("%v", params["photo"]); !strings.Contains(got, photoID) {
		t.Errorf("photo = %v, want it to contain %q", got, photoID)
	}
	if _, ok := params["caption"]; !ok {
		t.Error("sendPhoto call is missing a caption")
	}
}

// TestSendReminder_UsesSendMessageWhenEventHasNoPhoto is the counterpart:
// an event with no photo keeps using plain SendMessage, unchanged.
func TestSendReminder_UsesSendMessageWhenEventHasNoPhoto(t *testing.T) {
	db := testDB(t)
	offset := 0
	sendTime := "09:00"
	user, event := seedUserEventRule(t, db, 1, "Asia/Jerusalem", sendTime, &offset, &sendTime, nil)
	rule := &models.ReminderRule{OffsetDays: &offset, SendTime: &sendTime, Enabled: true, EventID: &event.ID}

	client := &fakeBotClient{}
	bot := testBot(client)

	if ok := SendReminder(context.Background(), bot, db, user, event, rule, 1, *event.NextOccurrence, event.CalendarType, 20); !ok {
		t.Fatal("SendReminder returned false, want true")
	}

	if len(client.calls) != 1 || client.calls[0]["method"] != "sendMessage" {
		t.Fatalf("calls = %+v, want exactly one sendMessage call", client.calls)
	}
}
