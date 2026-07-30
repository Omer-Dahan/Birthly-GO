package keyboards

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/core"
	"birthly/internal/i18n"
)

// channelURL is the operator's personal Telegram channel for bot updates —
// linked from the help screen (SPEC ask: give users a way to follow along
// beyond this one bot).
const channelURL = "https://t.me/YD_IL_BOTS"

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
func HelpKeyboard(lang, botUsername string) *gotgbot.InlineKeyboardMarkup {
	botLink := "https://t.me/" + botUsername
	shareBtn := gotgbot.InlineKeyboardButton{
		Text: i18n.T("greeting.share_us", lang, nil),
		Url:  core.ShareURLWithLink(botLink, i18n.T("greeting.share_us_text", lang, map[string]any{"link": botLink})),
	}
	channelBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("help.channel_btn", lang, nil), Url: channelURL}
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{Text: i18n.T("help.add_first", lang, nil), CallbackData: callbacks.Menu{Action: "add"}.Encode()},
				HomeButton(i18n.T("common.home", lang, nil)),
			},
			{shareBtn, channelBtn},
		},
	}
}
