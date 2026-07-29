// Package scheduler runs the background jobs, including the reminder tick.
//
// notify.go lives here (not internal/services) because it touches gotgbot's
// Bot and error types directly — services must stay framework-agnostic
// (SPEC.md ch.5), matching why Python's equivalent send logic lives in
// app/scheduler/notify.py rather than app/services.
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// sendRateLimiter is a token-bucket limiter for the global outbound message
// rate (SPEC.md ch.18: max broadcastRatePerSec messages/second across all
// sends). Port of app/utils/ratelimit.py's AsyncRateLimiter — Go doesn't
// need Python's lazy-lock-creation workaround for asyncio's per-event-loop
// binding, so this is the direct version.
type sendRateLimiter struct {
	mu         sync.Mutex
	rate       float64
	capacity   float64
	tokens     float64
	lastRefill time.Time
}

func newSendRateLimiter(ratePerSecond int) *sendRateLimiter {
	capacity := float64(ratePerSecond)
	if capacity < 1 {
		capacity = 1
	}
	return &sendRateLimiter{
		rate: float64(ratePerSecond), capacity: capacity,
		tokens: capacity, lastRefill: time.Now(),
	}
}

// Acquire blocks until a token is available, then consumes it.
func (l *sendRateLimiter) Acquire() {
	for {
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.lastRefill).Seconds()
		l.tokens = min(l.capacity, l.tokens+elapsed*l.rate)
		l.lastRefill = now

		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return
		}
		deficit := 1 - l.tokens
		wait := time.Duration(deficit / l.rate * float64(time.Second))
		l.mu.Unlock()
		time.Sleep(wait)
	}
}

// ReminderKeyboard is the action keyboard shown below a reminder push
// (SPEC.md S16). Uses the raw "ev:action:id" wire format directly (matching
// callbacks.EventCard.Encode()) to avoid an import cycle with
// internal/bot/keyboards, which itself may eventually depend on scheduler
// for shared render helpers.
func ReminderKeyboard(eventID int64, lang string) gotgbot.InlineKeyboardMarkup {
	idStr := strconv.FormatInt(eventID, 10)
	greetingText, cardText, muteText := "💌 ברכה", "👤 הכרטיס", "🔕 השתק"
	if lang != core.LanguageHe {
		greetingText, cardText, muteText = "💌 Greeting", "👤 Card", "🔕 Mute"
	}
	return gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: greetingText, CallbackData: "ev:gr:" + idStr},
				{Text: cardText, CallbackData: "ev:v:" + idStr},
			},
			{{Text: muteText, CallbackData: "ev:mute:" + idStr}},
		},
	}
}

var globalSendLimiter *sendRateLimiter
var globalSendLimiterOnce sync.Once

func sendLimiter(ratePerSecond int) *sendRateLimiter {
	globalSendLimiterOnce.Do(func() {
		globalSendLimiter = newSendRateLimiter(ratePerSecond)
	})
	return globalSendLimiter
}

// SendReminder sends a single reminder message and updates the log row.
// Returns true on success, false on failure. Handles the bot-blocked (403)
// and flood-control (429/RetryAfter) cases explicitly, matching
// app/scheduler/notify.py's send().
func SendReminder(ctx context.Context, bot *gotgbot.Bot, db *sql.DB, user *models.User, event *models.Event, rule *models.ReminderRule, logID int64, occurrenceYear, broadcastRatePerSec int) bool {
	notifRepo := repo.NewNotificationRepo(db)

	text := services.RenderReminder(user, event, rule, occurrenceYear)
	kb := ReminderKeyboard(event.ID, user.Language)
	limiter := sendLimiter(broadcastRatePerSec)

	for attempt := 0; attempt < 3; attempt++ {
		limiter.Acquire()
		_, err := bot.SendMessage(user.ID, text, &gotgbot.SendMessageOpts{
			ReplyMarkup:         kb,
			ParseMode:           "HTML",
			DisableNotification: user.SilentNotifications,
		})
		if err == nil {
			_ = notifRepo.MarkSent(ctx, logID)
			return true
		}

		var tgErr *gotgbot.TelegramError
		if errors.As(err, &tgErr) {
			if tgErr.Code == 403 {
				// User blocked the bot — disable notifications permanently.
				users := repo.NewUserRepo(db)
				_ = users.SetBotBlockedByUser(ctx, user.ID, true)
				_ = notifRepo.MarkFailed(ctx, logID, "bot_blocked_by_user")
				return false
			}
			if tgErr.Code == 429 && tgErr.ResponseParams != nil {
				wait := time.Duration(tgErr.ResponseParams.RetryAfter+1) * time.Second
				if attempt < 2 {
					time.Sleep(wait)
					continue
				}
				_ = notifRepo.MarkFailed(ctx, logID, "RetryAfter after 3 attempts")
				return false
			}
		}

		errText := err.Error()
		if len(errText) > 400 {
			errText = errText[:400]
		}
		_ = notifRepo.MarkFailed(ctx, logID, errText)
		return false
	}
	return false
}
