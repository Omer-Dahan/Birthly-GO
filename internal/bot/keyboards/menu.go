package keyboards

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/i18n"
)

// HomeKeyboard is the S1 home screen keyboard: 2 buttons per row (SPEC.md
// chapter 15) — port of app/keyboards/menu.py's home_keyboard.
func HomeKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	btn := func(key, action string) gotgbot.InlineKeyboardButton {
		return gotgbot.InlineKeyboardButton{
			Text:         i18n.T(key, lang, nil),
			CallbackData: callbacks.Menu{Action: action}.Encode(),
		}
	}
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{btn("menu.add", "add"), btn("menu.list", "list")},
			{btn("menu.search", "srch"), btn("menu.reminders", "rem")},
			{btn("menu.stats", "stat"), btn("menu.settings", "set")},
			{btn("menu.help", "help")},
		},
	}
}

// HelpKeyboard is the keyboard shown alongside the /help screen.
func HelpKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: i18n.T("help.add_first", lang, nil), CallbackData: callbacks.Menu{Action: "add"}.Encode()},
				HomeButton(i18n.T("common.home", lang, nil)),
			},
		},
	}
}
