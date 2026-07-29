package keyboards

import (
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/i18n"
)

var tones = []string{"warm", "funny", "formal", "short"}

func tplBtn(text, action string, value *string, eventID, excludeID *int64) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{
		Text:         text,
		CallbackData: callbacks.Template{Action: action, Value: value, EventID: eventID, ExcludeID: excludeID}.Encode(),
	}
}

// GreetingStyleKeyboard is the S14 tone picker.
func GreetingStyleKeyboard(lang string, eventID int64) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(tones))
	for i, tone := range tones {
		v := tone
		buttons[i] = tplBtn(i18n.T("greeting.tone."+tone, lang, nil), "pick", &v, &eventID, nil)
	}
	aiBtn := tplBtn(i18n.T("greeting.tone.ai", lang, nil), "ai", nil, &eventID, nil)
	personalBtn := tplBtn(i18n.T("greeting.tone.personal", lang, nil), "list", nil, &eventID, nil)
	backBtn := cardBtn(i18n.T("common.back", lang, nil), "v", eventID)

	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			buttons[:2], buttons[2:], {aiBtn, personalBtn}, {backBtn},
		},
	}
}

// GreetingResultKeyboard is the S14 result screen: another / share / back.
func GreetingResultKeyboard(lang string, eventID int64, tone string, lastTplID int64, greetingText string) *gotgbot.InlineKeyboardMarkup {
	toneVal := tone
	anotherBtn := tplBtn(i18n.T("greeting.another", lang, nil), "pick", &toneVal, &eventID, &lastTplID)
	shareBtn := gotgbot.InlineKeyboardButton{
		Text: i18n.T("greeting.share", lang, nil),
		SwitchInlineQueryChosenChat: &gotgbot.SwitchInlineQueryChosenChat{
			Query: greetingText, AllowUserChats: true, AllowGroupChats: true,
		},
	}
	backBtn := tplBtn(i18n.T("common.back", lang, nil), "style", nil, &eventID, nil)
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{anotherBtn, shareBtn}, {backBtn}}}
}

// AIPromptKeyboard is shown under the copy-paste AI prompt.
func AIPromptKeyboard(lang string, eventID int64) *gotgbot.InlineKeyboardMarkup {
	backBtn := tplBtn(i18n.T("common.back", lang, nil), "style", nil, &eventID, nil)
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{backBtn}}}
}

// TemplateRow pairs a personal template id with its preview text.
type TemplateRow struct {
	ID      int64
	Preview string
}

// PersonalTemplatesKeyboard is the S14 "✏️ שלי" list — one row per personal
// template with a delete button.
func PersonalTemplatesKeyboard(lang string, eventID int64, templateRows []TemplateRow) *gotgbot.InlineKeyboardMarkup {
	rows := make([][]gotgbot.InlineKeyboardButton, 0, len(templateRows)+2)
	for _, r := range templateRows {
		v := strconv.FormatInt(r.ID, 10)
		useBtn := tplBtn(r.Preview, "use", &v, &eventID, nil)
		delBtn := tplBtn("🗑", "del", &v, &eventID, nil)
		rows = append(rows, []gotgbot.InlineKeyboardButton{useBtn, delBtn})
	}
	newBtn := tplBtn(i18n.T("greeting.new.create", lang, nil), "new", nil, &eventID, nil)
	backBtn := tplBtn(i18n.T("common.back", lang, nil), "style", nil, &eventID, nil)
	rows = append(rows, []gotgbot.InlineKeyboardButton{newBtn}, []gotgbot.InlineKeyboardButton{backBtn})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// TemplateNewKeyboard is shown when creating a new personal template.
func TemplateNewKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(tones))
	for i, tone := range tones {
		v := tone
		buttons[i] = tplBtn(i18n.T("greeting.tone."+tone, lang, nil), "new_tone", &v, nil, nil)
	}
	cancelBtn := CancelButton(i18n.T("common.cancel", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons[:2], buttons[2:], {cancelBtn}}}
}
