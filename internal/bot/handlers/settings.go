package handlers

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
)

// RegisterSettings wires the S20 settings screens — port of app/handlers/settings.py.
func RegisterSettings(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("set"), cbMenuSettings))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("back"), cbSettingsBack))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("lang_menu"), cbLangMenu))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("lang"), cbLangSet))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tz_menu"), cbTzMenu))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tz"), cbTzSet))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tz_other"), cbTzOther))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("df_menu"), cbDfMenu))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("df"), cbDfSet))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tf_menu"), cbTfMenu))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("tf"), cbTfSet))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("time_menu"), cbSettingsTimeMenu))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("time"), cbSettingsTimeSet))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("time_other"), cbTimeOther))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("hebd_toggle"), cbHebdToggle))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("notif_toggle"), cbNotifToggle))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("silent_toggle"), cbSilentToggle))
	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("digest_toggle"), cbDigestToggle))

	dispatcher.AddHandler(handlers.NewCallback(settingsActionFilter("wipe"), cbWipeWarning))

	dispatcher.AddHandler(handlers.NewMessage(anyText, msgTzCustom))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgSettingsTimeCustom))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgWipeConfirm))
}

var settingsTimeRE = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

func renderSettingsHome(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, user *models.User) error {
	return EditOrIgnore(b, cq, i18n.T("settings.title", user.Language, nil), keyboards.SettingsKeyboard(user))
}

func cbMenuSettings(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).Clear(user.ID)
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbSettingsBack(b *gotgbot.Bot, ctx *ext.Context) error {
	return cbMenuSettings(b, ctx)
}

func cbLangMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.pick_language", user.Language, nil), keyboards.LanguagePickerKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func settingsCallbackValue(ctx *ext.Context) (*string, error) {
	sc, err := callbacks.DecodeSettings(ctx.CallbackQuery.Data)
	if err != nil {
		return nil, err
	}
	return sc.Value, nil
}

func cbLangSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	value, err := settingsCallbackValue(ctx)
	if err != nil {
		return err
	}
	if value != nil {
		if err := services.SetLanguage(context.Background(), db, user, *value); err != nil {
			return err
		}
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}

func cbTzMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.pick_timezone", user.Language, nil), keyboards.TimezonePickerKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbTzSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	value, err := settingsCallbackValue(ctx)
	if err != nil {
		return err
	}
	if value != nil {
		if err := services.SetTimezone(context.Background(), db, user, *value); err != nil {
			return err
		}
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}

func cbTzOther(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.SettingsFlowCustomTimezone)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.timezone_prompt", user.Language, nil), nil); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgTzCustom(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.SettingsFlowCustomTimezone {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	tz, err := services.ValidateTimezone(ctx.EffectiveMessage.Text)
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("settings.timezone_invalid", user.Language, nil), nil)
		return sendErr
	}

	if err := services.SetTimezone(context.Background(), db, user, tz); err != nil {
		return err
	}
	store.Clear(user.ID)
	_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("common.saved", user.Language, nil), &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SettingsKeyboard(user)})
	return sendErr
}

func cbDfMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.pick_date_format", user.Language, nil), keyboards.DateFormatPickerKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbDfSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	value, err := settingsCallbackValue(ctx)
	if err != nil {
		return err
	}
	if value != nil {
		user.DateFormat = *value
		if err := services.UpdateSettings(context.Background(), db, user); err != nil {
			return err
		}
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}

func cbTfMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.pick_time_format", user.Language, nil), keyboards.TimeFormatPickerKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbTfSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	value, err := settingsCallbackValue(ctx)
	if err != nil {
		return err
	}
	if value != nil {
		user.TimeFormat = *value
		if err := services.UpdateSettings(context.Background(), db, user); err != nil {
			return err
		}
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}

func cbSettingsTimeMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.pick_notify_time", user.Language, nil), keyboards.NotifyTimePickerKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbSettingsTimeSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	value, err := settingsCallbackValue(ctx)
	if err != nil {
		return err
	}
	packed := ""
	if value != nil {
		packed = *value
	}
	hh, mm, found := strings.Cut(packed, "-")
	if !found || !isAllDigits(hh) || !isAllDigits(mm) {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("error.generic", user.Language, nil))
		return nil
	}
	hour, _ := strconv.Atoi(hh)
	if err := services.SetDefaultNotifyTime(context.Background(), db, user, fmtHHMM(hour, mm)); err != nil {
		return err
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}

func cbTimeOther(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.SettingsFlowCustomTime)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.notify_time_prompt", user.Language, nil), nil); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgSettingsTimeCustom(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.SettingsFlowCustomTime {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	match := settingsTimeRE.FindStringSubmatch(strings.TrimSpace(ctx.EffectiveMessage.Text))
	if match == nil {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("edit.time_error", user.Language, nil), nil)
		return err
	}
	hour, _ := strconv.Atoi(match[1])
	timeStr := fmtHHMM(hour, match[2])

	if err := services.SetDefaultNotifyTime(context.Background(), db, user, timeStr); err != nil {
		return err
	}
	store.Clear(user.ID)
	_, err := ctx.EffectiveMessage.Reply(b, i18n.T("common.saved", user.Language, nil), &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SettingsKeyboard(user)})
	return err
}

func cbHebdToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	return toggleSetting(b, ctx, func(u *models.User) { u.ShowHebrewDate = !u.ShowHebrewDate })
}
func cbNotifToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	return toggleSetting(b, ctx, func(u *models.User) { u.NotificationsEnabled = !u.NotificationsEnabled })
}
func cbSilentToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	return toggleSetting(b, ctx, func(u *models.User) { u.SilentNotifications = !u.SilentNotifications })
}
func cbDigestToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	return toggleSetting(b, ctx, func(u *models.User) { u.DailyDigestEnabled = !u.DailyDigestEnabled })
}

func toggleSetting(b *gotgbot.Bot, ctx *ext.Context, mutate func(*models.User)) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	mutate(user)
	if err := services.UpdateSettings(context.Background(), db, user); err != nil {
		return err
	}
	if err := renderSettingsHome(b, ctx.CallbackQuery, user); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbWipeWarning(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.SettingsFlowWipeConfirm)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("settings.wipe_warning", user.Language, nil), keyboards.WipeWarningKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgWipeConfirm(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.SettingsFlowWipeConfirm {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	expected := "delete"
	if user.Language == "he" {
		expected = "מחק"
	}
	if strings.TrimSpace(ctx.EffectiveMessage.Text) != expected {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("settings.wipe_confirm_wrong", user.Language, nil), nil)
		return err
	}

	if err := services.WipeAccount(context.Background(), db, user); err != nil {
		return err
	}
	store.Clear(user.ID)
	_, err := ctx.EffectiveMessage.Reply(b, i18n.T("settings.wipe_done", user.Language, nil), nil)
	return err
}
