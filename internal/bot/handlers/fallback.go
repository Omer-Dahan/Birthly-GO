package handlers

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"birthly/internal/bot/router"
)

// RegisterFallback wires the catch-all handlers — port of
// app/handlers/fallback.py. Registration order matters: gotgbot only runs
// the first matching handler per group, and every RegisterX in
// cmd/birthly/main.go shares the default group (0). These handlers must
// therefore be registered last (see registerHandlers), so every
// feature-specific command/message/callback filter gets a chance to match
// before falling through to these always-true catches.
func RegisterFallback(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCommand("cancel", cmdCancel))
	dispatcher.AddHandler(handlers.NewMessage(anyMessage, fallbackMessage))
	dispatcher.AddHandler(handlers.NewCallback(anyCallback, fallbackCallback))
}

func anyMessage(msg *gotgbot.Message) bool       { return true }
func anyCallback(cq *gotgbot.CallbackQuery) bool { return true }

const cancelConfirmationText = "✅ הפעולה בוטלה."

func cmdCancel(b *gotgbot.Bot, ctx *ext.Context) error {
	router.FSMFromContext(ctx).Clear(ctx.EffectiveUser.Id)
	_, err := ctx.EffectiveMessage.Reply(b, cancelConfirmationText, nil)
	return err
}

const (
	fallbackWithStateText    = "לא הבנתי. אנא השב בהתאם או לחץ על /cancel כדי לבטל את הפעולה הנוכחית."
	fallbackWithoutStateText = "לא הבנתי את הפקודה. נסה להשתמש בתפריט /start או /help."
)

func fallbackMessageText(hasState bool) string {
	if hasState {
		return fallbackWithStateText
	}
	return fallbackWithoutStateText
}

func fallbackMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	_, hasState := router.FSMFromContext(ctx).GetState(ctx.EffectiveUser.Id)
	_, err := ctx.EffectiveMessage.Reply(b, fallbackMessageText(hasState), nil)
	return err
}

const fallbackCallbackAlertText = "הפעולה אינה זמינה כרגע."

func fallbackCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	AnswerCallbackAlert(b, ctx.CallbackQuery, fallbackCallbackAlertText)
	return nil
}
