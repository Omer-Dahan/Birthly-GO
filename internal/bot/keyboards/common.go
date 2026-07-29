// Package keyboards builds the inline keyboards.
package keyboards

import (
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
)

func BackButton(text string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.Nav{Action: "back"}.Encode()}
}

func HomeButton(text string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.Nav{Action: "home"}.Encode()}
}

func CancelButton(text string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.Nav{Action: "cancel"}.Encode()}
}

func SingleRowKeyboard(buttons ...gotgbot.InlineKeyboardButton) *gotgbot.InlineKeyboardMarkup {
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons}}
}

// BuildGrid lays out buttons in rows of columns (SPEC.md UX rule: max 2 per row).
func BuildGrid(buttons []gotgbot.InlineKeyboardButton, columns int) [][]gotgbot.InlineKeyboardButton {
	if columns <= 0 {
		columns = 2
	}
	var rows [][]gotgbot.InlineKeyboardButton
	for i := 0; i < len(buttons); i += columns {
		end := i + columns
		if end > len(buttons) {
			end = len(buttons)
		}
		rows = append(rows, buttons[i:end])
	}
	return rows
}

// PageRow renders the [◀️] [n/total] [▶️] row. Prev/next are noop buttons
// at the edges (SPEC.md pagination pattern) — port of app/keyboards/pagination.py.
func PageRow(page, totalPages int) []gotgbot.InlineKeyboardButton {
	prevData := callbacks.EncodeNoop()
	if page > 0 {
		prevData = callbacks.List{Action: "p", Value: strconv.Itoa(page - 1)}.Encode()
	}

	displayTotal := totalPages
	if displayTotal < 1 {
		displayTotal = 1
	}

	nextData := callbacks.EncodeNoop()
	if page+1 < totalPages {
		nextData = callbacks.List{Action: "p", Value: strconv.Itoa(page + 1)}.Encode()
	}

	return []gotgbot.InlineKeyboardButton{
		{Text: "◀️", CallbackData: prevData},
		{Text: strconv.Itoa(page+1) + "/" + strconv.Itoa(displayTotal), CallbackData: callbacks.EncodeNoop()},
		{Text: "▶️", CallbackData: nextData},
	}
}
