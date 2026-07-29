package handlers

import (
	"context"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/repo"
)

// RegisterStart wires /start and the onboarding flow into the dispatcher —
// port of app/handlers/start.py.
//
// gotgbot's callback filters (unlike aiogram's) have no built-in per-state
// matching, so each onboarding step's filter matches on the callback action
// alone; the handler body itself checks the caller's FSM state and returns
// ext.ContinueGroups when it doesn't match, letting the dispatcher fall
// through to check other group-0 handlers (e.g. the live settings screen's
// own "lang"/"tz"/"time" handlers) for the same callback_data shape.
func RegisterStart(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCommand("start", cmdStart))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("lang"), cbOnboardingLanguage))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tz"), cbOnboardingTimezone))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("time"), cbOnboardingNotifyTime))
	dispatcher.AddHandler(handlers.NewCallback(navActionFilter("cancel"), cbOnboardingCancel))
}

func cmdStart(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)

	if user.Onboarded {
		text, kb, err := RenderHome(context.Background(), db, user)
		if err != nil {
			return err
		}
		_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: kb, ParseMode: "HTML"})
		return err
	}

	store := router.FSMFromContext(ctx)
	store.SetState(user.ID, fsm.OnboardingLanguage)

	welcome := i18n.T("onboarding.welcome", user.Language, nil)
	prompt := i18n.T("onboarding.language.prompt", user.Language, nil)
	_, err := ctx.EffectiveMessage.Reply(b, welcome+"\n\n"+prompt, &gotgbot.SendMessageOpts{
		ReplyMarkup: keyboards.LanguageKeyboard(), ParseMode: "HTML",
	})
	return err
}

func settingsActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixSettings {
			return false
		}
		s, err := callbacks.DecodeSettings(cq.Data)
		return err == nil && s.Action == action
	}
}

func navActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixNav {
			return false
		}
		n, err := callbacks.DecodeNav(cq.Data)
		return err == nil && n.Action == action
	}
}

func cbOnboardingLanguage(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.OnboardingLanguage {
		return ext.ContinueGroups
	}

	db := router.DBFromContext(ctx)
	sc, err := callbacks.DecodeSettings(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	lang := "he"
	if sc.Value != nil {
		lang = *sc.Value
	}
	if err := services.SetLanguage(context.Background(), db, user, lang); err != nil {
		return err
	}

	store.SetState(user.ID, fsm.OnboardingTimezone)
	prompt := i18n.T("onboarding.timezone.prompt", user.Language, nil)
	if err := EditOrIgnore(b, ctx.CallbackQuery, prompt, keyboards.TimezoneKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbOnboardingTimezone(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.OnboardingTimezone {
		return ext.ContinueGroups
	}

	db := router.DBFromContext(ctx)
	sc, err := callbacks.DecodeSettings(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	if sc.Value != nil && *sc.Value != "other" {
		if err := services.SetTimezone(context.Background(), db, user, *sc.Value); err != nil {
			return err
		}
	}

	store.SetState(user.ID, fsm.OnboardingNotifyTime)
	prompt := i18n.T("onboarding.notify_time.prompt", user.Language, nil)
	if err := EditOrIgnore(b, ctx.CallbackQuery, prompt, keyboards.NotifyTimeKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbOnboardingNotifyTime(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.OnboardingNotifyTime {
		return ext.ContinueGroups
	}

	db := router.DBFromContext(ctx)
	sc, err := callbacks.DecodeSettings(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	if sc.Value != nil && *sc.Value != "other" {
		hhmm := strings.ReplaceAll(*sc.Value, "-", ":")
		if err := services.SetDefaultNotifyTime(context.Background(), db, user, hhmm); err != nil {
			return err
		}
	}

	return finishOnboarding(b, ctx, store, db, user.ID)
}

func cbOnboardingCancel(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	state, ok := store.GetState(user.ID)
	if !ok || (state != fsm.OnboardingLanguage && state != fsm.OnboardingTimezone && state != fsm.OnboardingNotifyTime) {
		return ext.ContinueGroups
	}

	db := router.DBFromContext(ctx)
	return finishOnboarding(b, ctx, store, db, user.ID)
}

func finishOnboarding(b *gotgbot.Bot, ctx *ext.Context, store *fsm.Store, db repo.DBTX, userID int64) error {
	user := User(ctx)
	if err := services.CompleteOnboarding(context.Background(), db, user); err != nil {
		return err
	}
	store.Clear(userID)

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
