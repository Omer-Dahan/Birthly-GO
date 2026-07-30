package handlers

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// RenderHome builds the S1 home screen text + keyboard.
func RenderHome(ctx context.Context, db repo.DBTX, user *models.User) (string, *gotgbot.InlineKeyboardMarkup, error) {
	summary, err := services.GetHomeSummary(ctx, db, user)
	if err != nil {
		return "", nil, err
	}
	lang := user.Language

	lines := []string{i18n.T("screen.home.title", lang, nil), ""}

	switch {
	case summary.TotalCount == 0:
		lines = append(lines, i18n.T("screen.home.empty", lang, nil))
	case summary.NearestEvent == nil:
		lines = append(lines, i18n.T("screen.home.none_soon", lang, map[string]any{"count": summary.TotalCount}))
	default:
		lines = append(lines, i18n.T("screen.home.today", lang, map[string]any{"count": summary.TodayCount}))
		lines = append(lines, i18n.T("screen.home.week", lang, map[string]any{"count": summary.WeekCount}))
		name := core.FormatName(summary.NearestEvent.FirstName, summary.NearestEvent.LastName)
		days := 0
		if summary.NearestDaysUntil != nil {
			days = *summary.NearestDaysUntil
		}
		countdown := core.FormatCountdown(days)
		lines = append(lines, i18n.T("screen.home.nearest", lang, map[string]any{"name": name, "countdown": countdown}))
	}

	lines = append(lines, "", i18n.T("screen.home.prompt", lang, nil))

	text := joinLines(lines)
	return text, keyboards.HomeKeyboard(lang), nil
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func renderHomeAndEdit(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	text, kb, err := RenderHome(context.Background(), db, user)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func helpTextAndKeyboard(lang, botUsername string) (string, *gotgbot.InlineKeyboardMarkup) {
	text := i18n.T("help.title", lang, nil) + "\n\n" + i18n.T("help.body", lang, nil)
	return text, keyboards.HelpKeyboard(lang, botUsername)
}

// RegisterMenu wires the menu/home/help handlers into the dispatcher.
func RegisterMenu(dispatcher *ext.Dispatcher) {
	// Edge pagination arrows and the page indicator use a noop callback_data
	// when there's nothing to navigate to. Without a dedicated handler these
	// fall through to fallbackCallback's "action unavailable" alert, which
	// reads as an error even though tapping a disabled-looking arrow is a
	// normal no-op.
	dispatcher.AddHandler(handlers.NewCallback(
		func(cq *gotgbot.CallbackQuery) bool { return cq.Data == callbacks.PrefixNoop },
		func(b *gotgbot.Bot, ctx *ext.Context) error {
			AnswerCallback(b, ctx.CallbackQuery, "")
			return nil
		},
	))
	dispatcher.AddHandler(handlers.NewCallback(
		callbackPrefixAction(callbacks.PrefixMenu, "home"),
		func(b *gotgbot.Bot, ctx *ext.Context) error { return renderHomeAndEdit(b, ctx) },
	))
	dispatcher.AddHandler(handlers.NewCallback(
		callbackPrefixAction(callbacks.PrefixNav, "home"),
		func(b *gotgbot.Bot, ctx *ext.Context) error { return renderHomeAndEdit(b, ctx) },
	))
	dispatcher.AddHandler(handlers.NewCallback(
		callbackPrefixAction(callbacks.PrefixNav, "cancel"),
		func(b *gotgbot.Bot, ctx *ext.Context) error {
			if store := router.FSMFromContext(ctx); store != nil {
				store.Clear(ctx.EffectiveUser.Id)
			}
			return renderHomeAndEdit(b, ctx)
		},
	))
	dispatcher.AddHandler(handlers.NewCallback(
		callbackPrefixAction(callbacks.PrefixMenu, "help"),
		func(b *gotgbot.Bot, ctx *ext.Context) error {
			user := User(ctx)
			text, kb := helpTextAndKeyboard(user.Language, router.ConfigFromContext(ctx).BotUsername)
			if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
				return err
			}
			AnswerCallback(b, ctx.CallbackQuery, "")
			return nil
		},
	))
	dispatcher.AddHandler(handlers.NewCommand("help", func(b *gotgbot.Bot, ctx *ext.Context) error {
		user := User(ctx)
		text, kb := helpTextAndKeyboard(user.Language, router.ConfigFromContext(ctx).BotUsername)
		_, err := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: kb, ParseMode: "HTML"})
		return err
	}))
}

// callbackPrefixAction builds a filters.CallbackQuery matching callback_data
// with the given prefix and action token, decoded via the callbacks package
// rather than a raw string prefix check (keeps every handler's filter
// consistent with how the payload is actually parsed downstream).
func callbackPrefixAction(prefix, action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != prefix {
			return false
		}
		switch prefix {
		case callbacks.PrefixMenu:
			m, err := callbacks.DecodeMenu(cq.Data)
			return err == nil && m.Action == action
		case callbacks.PrefixNav:
			n, err := callbacks.DecodeNav(cq.Data)
			return err == nil && n.Action == action
		default:
			return false
		}
	}
}
