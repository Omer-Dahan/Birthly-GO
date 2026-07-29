package keyboards

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/i18n"
)

func backupBtn(text, action string, value *string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.Backup{Action: action, Value: value}.Encode()}
}

// BackupHomeKeyboard is the S15 main backup screen.
func BackupHomeKeyboard(lang string, autoEnabled bool) *gotgbot.InlineKeyboardMarkup {
	jsonBtn := backupBtn(i18n.T("backup.export_json", lang, nil), "exp", strPtr("json"))
	csvBtn := backupBtn(i18n.T("backup.export_csv", lang, nil), "exp", strPtr("csv"))
	xlsxBtn := backupBtn(i18n.T("backup.export_xlsx", lang, nil), "exp", strPtr("xlsx"))
	importBtn := backupBtn(i18n.T("backup.import", lang, nil), "imp", nil)

	autoLabel := "backup.auto_off"
	autoNext := "1"
	if autoEnabled {
		autoLabel = "backup.auto_on"
		autoNext = "0"
	}
	autoBtn := backupBtn(i18n.T(autoLabel, lang, nil), "auto", &autoNext)
	backBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("common.back", lang, nil), CallbackData: callbacks.Nav{Action: "home"}.Encode()}

	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{jsonBtn, csvBtn, xlsxBtn}, {importBtn}, {autoBtn}, {backBtn},
		},
	}
}

// ImportModeKeyboard is shown after the import file is parsed: add / replace / cancel.
func ImportModeKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	addBtn := backupBtn(i18n.T("backup.import_add", lang, nil), "imp_do", strPtr("add"))
	replaceBtn := backupBtn(i18n.T("backup.import_replace", lang, nil), "imp_do", strPtr("replace"))
	cancelBtn := CancelButton(i18n.T("common.cancel", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{addBtn}, {replaceBtn}, {cancelBtn}}}
}

// ImportReplaceConfirmKeyboard is the extra confirmation for destructive
// "replace all" import.
func ImportReplaceConfirmKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	confirmBtn := backupBtn(i18n.T("backup.import_replace_confirm", lang, nil), "imp_do", strPtr("replace_confirmed"))
	cancelBtn := CancelButton(i18n.T("common.cancel", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{confirmBtn}, {cancelBtn}}}
}

func strPtr(s string) *string { return &s }
