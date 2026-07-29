package keyboards

import (
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/i18n"
)

func skipButton(lang string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{
		Text:         i18n.T("onboarding.skip_all", lang, nil),
		CallbackData: callbacks.Nav{Action: "cancel"}.Encode(),
	}
}

func settingsButton(text, action, value string) gotgbot.InlineKeyboardButton {
	v := value
	return gotgbot.InlineKeyboardButton{
		Text:         text,
		CallbackData: callbacks.Settings{Action: action, Value: &v}.Encode(),
	}
}

// LanguageKeyboard is the first onboarding screen.
func LanguageKeyboard() *gotgbot.InlineKeyboardMarkup {
	he := settingsButton(i18n.T("onboarding.language.he", "he", nil), "lang", "he")
	en := settingsButton(i18n.T("onboarding.language.en", "en", nil), "lang", "en")
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{he, en}, {skipButton("he")}},
	}
}

// TimezoneKeyboard is the second onboarding screen.
func TimezoneKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	israel := settingsButton(i18n.T("onboarding.timezone.israel", lang, nil), "tz", "Asia/Jerusalem")
	other := settingsButton(i18n.T("onboarding.timezone.other", lang, nil), "tz", "other")
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{israel, other}, {skipButton(lang)}},
	}
}

// NotifyTimeKeyboard is the third onboarding screen.
func NotifyTimeKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	btn := func(hhmm string) gotgbot.InlineKeyboardButton {
		return settingsButton(hhmm, "time", strings.ReplaceAll(hhmm, ":", "-"))
	}
	other := settingsButton(i18n.T("onboarding.notify_time.other", lang, nil), "time", "other")
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{btn("09:00"), btn("12:00"), btn("18:00")},
			{other},
			{skipButton(lang)},
		},
	}
}
