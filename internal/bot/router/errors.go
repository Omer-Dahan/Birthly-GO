package router

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/config"
)

// errorReporter is the gotgbot equivalent of app/handlers/errors.py's
// global_error_handler (an aiogram @router.errors() hook): it runs for any
// handler that returns a non-nil error, tells the user something broke,
// and — deduplicated per exception signature for an hour — pages the admins.
type errorReporter struct {
	cfg    *config.Config
	logger *slog.Logger

	mu         sync.Mutex
	nextReport map[string]time.Time
}

func newErrorReporter(cfg *config.Config, logger *slog.Logger) *errorReporter {
	return &errorReporter{cfg: cfg, logger: logger, nextReport: make(map[string]time.Time)}
}

// dedupWindow mirrors errors.py's timedelta(hours=1) admin-alert silencing.
const dedupWindow = time.Hour

func (r *errorReporter) handle(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
	errorID := shortErrorID()
	signature := fmt.Sprintf("%T:%s", err, truncateRunes(err.Error(), 50))

	r.logger.Error("unhandled_exception", "error_id", errorID, "signature", signature, "error", err)

	var userID int64
	switch {
	case ctx.Update.Message != nil:
		userID = ctx.Update.Message.Chat.Id
	case ctx.Update.CallbackQuery != nil:
		if msg, ok := ctx.Update.CallbackQuery.Message.(gotgbot.Message); ok {
			userID = msg.Chat.Id
		}
		if _, aErr := b.AnswerCallbackQuery(ctx.Update.CallbackQuery.Id, &gotgbot.AnswerCallbackQueryOpts{
			Text: "שגיאה, נסה שוב.", ShowAlert: true,
		}); aErr != nil {
			r.logger.Error("answer_callback_after_error_failed", "error", aErr)
		}
	}

	if userID != 0 {
		text := fmt.Sprintf("😕 משהו השתבש. נסה שוב בעוד רגע. (קוד: %s)", errorID)
		if _, sErr := b.SendMessage(userID, text, nil); sErr != nil {
			r.logger.Error("notify_user_after_error_failed", "user_id", userID, "error", sErr)
		}
	}

	r.notifyAdmins(b, errorID, signature, err)
	return ext.DispatcherActionNoop
}

func (r *errorReporter) notifyAdmins(b *gotgbot.Bot, errorID, signature string, err error) {
	if !r.cfg.ReportErrorsToAdmin || len(r.cfg.AdminIDs) == 0 {
		return
	}

	now := time.Now()
	r.mu.Lock()
	next, seen := r.nextReport[signature]
	if seen && now.Before(next) {
		r.mu.Unlock()
		return
	}
	r.nextReport[signature] = now.Add(dedupWindow)
	r.mu.Unlock()

	adminMsg := fmt.Sprintf(
		"🚨 <b>שגיאה חדשה במערכת</b> (ID: <code>%s</code>)\n\n"+
			"<b>סוג:</b> %s\n"+
			"<b>פרטים:</b> <code>%s</code>\n\n"+
			"<i>* התראה זו הושתקה לשעה הקרובה.</i>",
		errorID, strings.SplitN(signature, ":", 2)[0], truncateRunes(err.Error(), 500),
	)
	for _, adminID := range r.cfg.AdminIDs {
		if _, sErr := b.SendMessage(adminID, adminMsg, &gotgbot.SendMessageOpts{ParseMode: "HTML"}); sErr != nil {
			r.logger.Error("notify_admin_after_error_failed", "admin_id", adminID, "error", sErr)
		}
	}
}

// shortErrorID is the Go equivalent of Python's str(uuid.uuid4())[:4].upper()
// — not cryptographically meaningful, just a short human-readable tag to
// correlate a user-facing message with the corresponding log line.
func shortErrorID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return strings.ToUpper(hex.EncodeToString(b[:]))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
