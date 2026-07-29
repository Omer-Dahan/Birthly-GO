package keyboards

import (
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/store/models"
)

var timeChoices = []string{"08:00", "09:00", "10:00", "12:00", "18:00", "20:00"}

func remBtn(text, action string, value *string, eventID *int64) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{
		Text:         text,
		CallbackData: callbacks.Reminder{Action: action, Value: value, EventID: eventID}.Encode(),
	}
}

func ruleLabel(rule *models.ReminderRule, lang string) string {
	label := "?"
	if rule.OffsetDays != nil {
		label = i18n.T("rem.offset."+strconv.Itoa(*rule.OffsetDays), lang, nil)
	}
	timeStr := "—"
	if rule.SendTime != nil && *rule.SendTime != "" {
		timeStr = *rule.SendTime
	}
	key := "rem.rule_off"
	if rule.Enabled {
		key = "rem.rule_on"
	}
	return i18n.T(key, lang, map[string]any{"label": label, "time": timeStr})
}

// RulesListKeyboard is the S11 reminders screen: one toggle button per rule
// + add/home.
func RulesListKeyboard(lang string, rules []*models.ReminderRule, eventID *int64) *gotgbot.InlineKeyboardMarkup {
	rows := make([][]gotgbot.InlineKeyboardButton, 0, len(rules)+2)
	for _, rule := range rules {
		idStr := strconv.FormatInt(rule.ID, 10)
		rows = append(rows, []gotgbot.InlineKeyboardButton{remBtn(ruleLabel(rule, lang), "menu", &idStr, eventID)})
	}
	addBtn := remBtn(i18n.T("rem.add", lang, nil), "add", nil, eventID)
	home := HomeButton(i18n.T("common.home", lang, nil))
	rows = append(rows, []gotgbot.InlineKeyboardButton{addBtn}, []gotgbot.InlineKeyboardButton{home})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func AddRuleKeyboard(lang string, eventID *int64) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.ReminderOffsetChoices))
	for i, offset := range core.ReminderOffsetChoices {
		v := strconv.Itoa(offset)
		buttons[i] = remBtn(i18n.T("rem.offset."+v, lang, nil), "off", &v, eventID)
	}
	rows := BuildGrid(buttons, 2)
	back := remBtn(i18n.T("common.back", lang, nil), "back", nil, eventID)
	rows = append(rows, []gotgbot.InlineKeyboardButton{back})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func RuleMenuKeyboard(lang string, ruleID int64, eventID *int64) *gotgbot.InlineKeyboardMarkup {
	idStr := strconv.FormatInt(ruleID, 10)
	toggle := remBtn(i18n.T("rem.toggle", lang, nil), "tog", &idStr, eventID)
	changeTime := remBtn(i18n.T("rem.change_time", lang, nil), "time_menu", &idStr, eventID)
	del := remBtn(i18n.T("rem.delete_rule", lang, nil), "del", &idStr, eventID)
	back := remBtn(i18n.T("common.back", lang, nil), "back", nil, eventID)
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{toggle}, {changeTime}, {del}, {back}}}
}

// TimeChoiceKeyboard: value packs "<HH-MM>:<rule_id>" — SPEC.md's
// "rem:time:<HH-MM>" schema extended with the rule id, since ReminderCallback
// has no third slot.
func TimeChoiceKeyboard(lang string, ruleID int64, eventID *int64) *gotgbot.InlineKeyboardMarkup {
	idStr := strconv.FormatInt(ruleID, 10)
	buttons := make([]gotgbot.InlineKeyboardButton, len(timeChoices))
	for i, tm := range timeChoices {
		packed := strings.ReplaceAll(tm, ":", "-") + ":" + idStr
		buttons[i] = remBtn(tm, "time", &packed, eventID)
	}
	rows := BuildGrid(buttons, 3)
	back := remBtn(i18n.T("common.back", lang, nil), "menu", &idStr, eventID)
	rows = append(rows, []gotgbot.InlineKeyboardButton{back})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}
