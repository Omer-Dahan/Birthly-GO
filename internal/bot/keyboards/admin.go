package keyboards

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
)

func adminBtn(text, action string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.Admin{Action: action}.Encode()}
}

// AdminHomeKeyboard is the admin panel home screen (2,2,1,1 layout, matching
// the Python InlineKeyboardBuilder.adjust(2, 2, 1, 1) call).
func AdminHomeKeyboard() *gotgbot.InlineKeyboardMarkup {
	stats := adminBtn("📊 סטטיסטיקות", "stats")
	bc := adminBtn("📢 הודעה לכולם", "bc")
	logs := adminBtn("📜 לוגים", "logs")
	userSearch := adminBtn("👤 חיפוש משתמש", "u")
	backup := adminBtn("💾 גיבוי עכשיו", "backup")
	home := gotgbot.InlineKeyboardButton{Text: "🏠 בית", CallbackData: callbacks.Nav{Action: "home"}.Encode()}
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{stats, bc}, {logs, userSearch}, {backup}, {home}},
	}
}

func BroadcastConfirmKeyboard() *gotgbot.InlineKeyboardMarkup {
	send := adminBtn("✅ שלח תפוצה", "bc_ok")
	cancel := adminBtn("❌ ביטול", "home")
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{send, cancel}}}
}

func BackToAdminKeyboard() *gotgbot.InlineKeyboardMarkup {
	back := adminBtn("← חזרה לאדמין", "home")
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{back}}}
}
