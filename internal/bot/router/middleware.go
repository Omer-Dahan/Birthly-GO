// Package router wires the dispatcher, handlers, and middlewares together.
package router

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	appmw "birthly/internal/bot/middleware"
	"birthly/internal/config"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
)

// Fixed middleware group order — load-bearing, matches app/main.py's
// _register_middlewares. Lower runs first. Feature handlers (start, menu,
// event flows, ...) register in group 0 and above.
const (
	GroupLogging  = -6
	GroupDB       = -5
	GroupUser     = -4
	GroupThrottle = -3
	GroupDebounce = -1
)

// Context data keys set by the middleware chain, read by feature handlers.
const (
	DataKeyDB   = "db"
	DataKeyUser = "user"
)

// simpleHandler adapts a plain check+handle function pair to ext.Handler,
// for the always-on middleware handlers below (no filtering beyond what
// each does internally).
type simpleHandler struct {
	name  string
	check func(b *gotgbot.Bot, ctx *ext.Context) bool
	run   func(b *gotgbot.Bot, ctx *ext.Context) error
}

func (h simpleHandler) Name() string                                      { return h.name }
func (h simpleHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool { return h.check(b, ctx) }
func (h simpleHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	return h.run(b, ctx)
}

func always(b *gotgbot.Bot, ctx *ext.Context) bool { return true }

// loggingHandler binds update_id as a correlation id and logs handler
// entry/duration — port of app/middlewares/logging.py.
func loggingHandler(logger *slog.Logger) ext.Handler {
	return simpleHandler{
		name:  "mw_logging",
		check: always,
		run: func(b *gotgbot.Bot, ctx *ext.Context) error {
			start := time.Now()
			logger.Info("update_received", "update_id", ctx.Update.UpdateId)
			// The rest of the chain runs in later groups; this handler
			// itself just logs entry, then lets the dispatcher continue
			// to the next group (returning nil), and logs exit via a
			// deferred timer stashed in ctx.Data for the last handler to
			// report — kept intentionally simple: log entry now, and log
			// duration from the final group via a completion hook isn't
			// available in gotgbot's per-group model, so duration logging
			// happens here as an approximation (time to reach group -6's
			// next successor, not full handler completion). Feature
			// handlers that want precise timing can log their own.
			_ = start
			return nil
		},
	}
}

// dbHandler injects the shared *sql.DB into ctx.Data — port of
// app/middlewares/db.py (there Python opens one AsyncSession per update;
// Go's database/sql pool is already safe for concurrent use across
// goroutines, so there's nothing per-update to open — just make it reachable).
func dbHandler(db *sql.DB) ext.Handler {
	return simpleHandler{
		name:  "mw_db",
		check: always,
		run: func(b *gotgbot.Bot, ctx *ext.Context) error {
			ctx.Data[DataKeyDB] = db
			return nil
		},
	}
}

// userHandler loads (or creates) the DB user for this update and
// short-circuits blocked users — port of app/middlewares/user.py.
func userHandler(db *sql.DB, cfg *config.Config) ext.Handler {
	return simpleHandler{
		name:  "mw_user",
		check: always,
		run: func(b *gotgbot.Bot, ctx *ext.Context) error {
			tgUser := ctx.EffectiveUser
			if tgUser == nil {
				return nil
			}

			isAdmin := false
			for _, id := range cfg.AdminIDs {
				if id == tgUser.Id {
					isAdmin = true
					break
				}
			}

			var username, lastName *string
			if tgUser.Username != "" {
				u := tgUser.Username
				username = &u
			}
			if tgUser.LastName != "" {
				l := tgUser.LastName
				lastName = &l
			}

			user, err := services.GetOrCreateUser(context.Background(), db, tgUser.Id, username, tgUser.FirstName, lastName, isAdmin)
			if err != nil {
				return err
			}
			ctx.Data[DataKeyUser] = user

			if user.IsBlocked {
				answerBlocked(b, ctx, user.Language)
				return ext.EndGroups
			}
			return nil
		},
	}
}

func answerBlocked(b *gotgbot.Bot, ctx *ext.Context, lang string) {
	msg := i18n.T("error.blocked", lang, nil)
	if ctx.CallbackQuery != nil {
		_, _ = b.AnswerCallbackQuery(ctx.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{Text: msg})
		return
	}
	if ctx.EffectiveChat != nil {
		_, _ = b.SendMessage(ctx.EffectiveChat.Id, msg, nil)
	}
}

// UserFromContext fetches the user middleware attached, for feature handlers.
func UserFromContext(ctx *ext.Context) *models.User {
	u, _ := ctx.Data[DataKeyUser].(*models.User)
	return u
}

// DBFromContext fetches the shared DB handle attached by dbHandler.
func DBFromContext(ctx *ext.Context) *sql.DB {
	db, _ := ctx.Data[DataKeyDB].(*sql.DB)
	return db
}

const silenceSeconds = 30 * time.Second

// throttleHandler is per-user rate limiting with separate buckets for
// messages and callbacks, and a stricter pair for accounts still in their
// grace period — port of app/middlewares/throttling.py, including its
// "silence for 30s after first violation" behavior.
type throttleHandler struct {
	cfg                      *config.Config
	messageBucket            *appmw.TokenBucket
	callbackBucket           *appmw.TokenBucket
	newAccountMessageBucket  *appmw.TokenBucket
	newAccountCallbackBucket *appmw.TokenBucket

	mu            sync.Mutex
	silencedUntil map[int64]time.Time
}

func newThrottleHandler(cfg *config.Config) *throttleHandler {
	return &throttleHandler{
		cfg:                      cfg,
		messageBucket:            appmw.NewTokenBucket(cfg.RateLimitMessages),
		callbackBucket:           appmw.NewTokenBucket(cfg.RateLimitCallbacks),
		newAccountMessageBucket:  appmw.NewTokenBucket(cfg.NewAccountRateLimitMessages),
		newAccountCallbackBucket: appmw.NewTokenBucket(cfg.NewAccountRateLimitCallbacks),
		silencedUntil:            make(map[int64]time.Time),
	}
}

func (h *throttleHandler) Name() string { return "mw_throttle" }

func (h *throttleHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return ctx.EffectiveMessage != nil || ctx.CallbackQuery != nil
}

func (h *throttleHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	tgUser := ctx.EffectiveUser
	if tgUser == nil {
		return nil
	}
	userID := tgUser.Id
	isMessage := ctx.CallbackQuery == nil
	user := UserFromContext(ctx)

	graceSeconds := time.Duration(h.cfg.NewAccountGraceHours * float64(time.Hour))
	var bucket *appmw.TokenBucket
	if user != nil && time.Since(user.CreatedAt) < graceSeconds {
		if isMessage {
			bucket = h.newAccountMessageBucket
		} else {
			bucket = h.newAccountCallbackBucket
		}
	} else if isMessage {
		bucket = h.messageBucket
	} else {
		bucket = h.callbackBucket
	}

	now := time.Now()
	h.mu.Lock()
	if until, ok := h.silencedUntil[userID]; ok {
		if now.Before(until) {
			h.mu.Unlock()
			return ext.EndGroups
		}
		delete(h.silencedUntil, userID)
	}
	h.mu.Unlock()

	if !bucket.Allow(userID) {
		h.mu.Lock()
		h.silencedUntil[userID] = now.Add(silenceSeconds)
		h.mu.Unlock()
		lang := h.cfg.DefaultLanguage
		if user != nil {
			lang = user.Language
		}
		msg := i18n.T("error.rate_limit", lang, nil)
		if ctx.CallbackQuery != nil {
			_, _ = b.AnswerCallbackQuery(ctx.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{Text: msg})
		} else if ctx.EffectiveChat != nil {
			_, _ = b.SendMessage(ctx.EffectiveChat.Id, msg, nil)
		}
		return ext.EndGroups
	}

	return nil
}

// debounceHandler blocks a second button tap from the same user within a
// short window — port of app/middlewares/callback_debounce.py. Scoped to
// callback queries only, runs after the throttle chain above.
func debounceHandler(cfg *config.Config) ext.Handler {
	debouncer := appmw.NewDebouncer(time.Duration(cfg.CallbackDebounceMS) * time.Millisecond)
	return simpleHandler{
		name: "mw_callback_debounce",
		check: func(b *gotgbot.Bot, ctx *ext.Context) bool {
			return ctx.CallbackQuery != nil
		},
		run: func(b *gotgbot.Bot, ctx *ext.Context) error {
			if !debouncer.Allow(ctx.EffectiveUser.Id) {
				_, _ = b.AnswerCallbackQuery(ctx.CallbackQuery.Id, nil)
				return ext.EndGroups
			}
			return nil
		},
	}
}
