// Package handlers contains the route handlers, grouped by feature area.
package handlers

import (
	"errors"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/router"
	"birthly/internal/store/models"
)

// EditOrIgnore edits the message behind a callback, if it's still editable.
// callback.Message can be an InaccessibleMessage (too old to edit) — that
// case is silently skipped rather than erroring, per SPEC.md chapter 26 (a
// single update must never crash the bot). Telegram also rejects an edit
// whose text and keyboard are byte-identical to the current message (e.g.
// re-tapping a button that leads back to the same screen) with "message is
// not modified" — that's a no-op from the user's perspective, so it's
// swallowed the same way. Port of app/utils/telegram.py's edit_or_ignore.
func EditOrIgnore(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, text string, keyboard *gotgbot.InlineKeyboardMarkup) error {
	msg, ok := cq.Message.(gotgbot.Message)
	if !ok {
		return nil
	}

	opts := &gotgbot.EditMessageTextOpts{
		ChatId:    msg.Chat.Id,
		MessageId: msg.MessageId,
		ParseMode: "HTML",
	}
	if keyboard != nil {
		opts.ReplyMarkup = *keyboard
	}

	_, _, err := b.EditMessageText(text, opts)
	if err != nil {
		var tgErr *gotgbot.TelegramError
		if errors.As(err, &tgErr) && strings.Contains(tgErr.Description, "message is not modified") {
			return nil
		}
		return err
	}
	return nil
}

// AnswerCallback is a small convenience wrapper around bot.AnswerCallbackQuery.
func AnswerCallback(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, text string) {
	opts := &gotgbot.AnswerCallbackQueryOpts{}
	if text != "" {
		opts.Text = text
	}
	_, _ = b.AnswerCallbackQuery(cq.Id, opts)
}

// User resolves the middleware-attached user for a handler.
func User(ctx *ext.Context) *models.User { return router.UserFromContext(ctx) }
