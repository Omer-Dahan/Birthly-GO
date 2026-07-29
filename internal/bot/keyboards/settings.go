package keyboards

import (
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/store/models"
)

func mark(value bool) string {
	if value {
		return "✅"
	}
	return "⬜"
}

func settingsNavBtn(key, action, value, lang string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{
		Text:         i18n.T(key, lang, map[string]any{"value": value}),
		CallbackData: callbacks.Settings{Action: action}.Encode(),
	}
}

// SettingsKeyboard is the S20 settings home screen.
func SettingsKeyboard(user *models.User) *gotgbot.InlineKeyboardMarkup {
	lang := user.Language
	home := HomeButton(i18n.T("common.home", lang, nil))
	backupBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("settings.backup_menu", lang, nil), CallbackData: callbacks.Backup{Action: "home"}.Encode()}
	wipeBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("settings.wipe_account", lang, nil), CallbackData: callbacks.Settings{Action: "wipe"}.Encode()}

	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{settingsNavBtn("settings.language", "lang_menu", i18n.T("onboarding.language."+user.Language, lang, nil), lang)},
			{settingsNavBtn("settings.timezone", "tz_menu", user.Timezone, lang)},
			{settingsNavBtn("settings.notify_time", "time_menu", user.DefaultNotifyTime, lang)},
			{settingsNavBtn("settings.date_format", "df_menu", user.DateFormat, lang)},
			{settingsNavBtn("settings.time_format", "tf_menu", user.TimeFormat, lang)},
			{settingsNavBtn("settings.show_hebrew_date", "hebd_toggle", mark(user.ShowHebrewDate), lang)},
			{settingsNavBtn("settings.notifications", "notif_toggle", mark(user.NotificationsEnabled), lang)},
			{settingsNavBtn("settings.silent_notifications", "silent_toggle", mark(user.SilentNotifications), lang)},
			{settingsNavBtn("settings.daily_digest", "digest_toggle", mark(user.DailyDigestEnabled), lang)},
			{backupBtn},
			{wipeBtn},
			{home},
		},
	}
}

func settingsBackBtn(lang string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: i18n.T("common.back", lang, nil), CallbackData: callbacks.Settings{Action: "back"}.Encode()}
}

func LanguagePickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.LanguageValues))
	for i, lg := range core.LanguageValues {
		v := lg
		buttons[i] = gotgbot.InlineKeyboardButton{
			Text:         i18n.T("onboarding.language."+lg, lang, nil),
			CallbackData: callbacks.Settings{Action: "lang", Value: &v}.Encode(),
		}
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons, {settingsBackBtn(lang)}}}
}

func TimezonePickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	tzBtn := func(key, tz string) gotgbot.InlineKeyboardButton {
		v := tz
		return gotgbot.InlineKeyboardButton{Text: i18n.T(key, lang, nil), CallbackData: callbacks.Settings{Action: "tz", Value: &v}.Encode()}
	}
	israel := tzBtn("settings.timezone_israel", "Asia/Jerusalem")
	ny := tzBtn("settings.timezone_ny", "America/New_York")
	london := tzBtn("settings.timezone_london", "Europe/London")
	other := gotgbot.InlineKeyboardButton{Text: i18n.T("settings.timezone_other", lang, nil), CallbackData: callbacks.Settings{Action: "tz_other"}.Encode()}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{israel, ny}, {london, other}, {settingsBackBtn(lang)}}}
}

func DateFormatPickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.DateFormatValues))
	for i, fmtVal := range core.DateFormatValues {
		v := fmtVal
		buttons[i] = gotgbot.InlineKeyboardButton{Text: fmtVal, CallbackData: callbacks.Settings{Action: "df", Value: &v}.Encode()}
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons, {settingsBackBtn(lang)}}}
}

func TimeFormatPickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.TimeFormatValues))
	for i, fmtVal := range core.TimeFormatValues {
		v := fmtVal
		buttons[i] = gotgbot.InlineKeyboardButton{Text: fmtVal, CallbackData: callbacks.Settings{Action: "tf", Value: &v}.Encode()}
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons, {settingsBackBtn(lang)}}}
}

var notifyTimeChoices = []string{"08:00", "09:00", "10:00", "18:00", "20:00"}

func NotifyTimePickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(notifyTimeChoices))
	for i, tm := range notifyTimeChoices {
		v := strings.ReplaceAll(tm, ":", "-")
		buttons[i] = gotgbot.InlineKeyboardButton{Text: tm, CallbackData: callbacks.Settings{Action: "time", Value: &v}.Encode()}
	}
	other := gotgbot.InlineKeyboardButton{Text: i18n.T("settings.notify_time_other", lang, nil), CallbackData: callbacks.Settings{Action: "time_other"}.Encode()}
	rows := BuildGrid(buttons, 3)
	rows = append(rows, []gotgbot.InlineKeyboardButton{other}, []gotgbot.InlineKeyboardButton{settingsBackBtn(lang)})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func WipeWarningKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{settingsBackBtn(lang)}}}
}
