package keyboards

import (
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
	"birthly/internal/core"
	"birthly/internal/i18n"
)

// MoreDetailsFields are the S6 "more details" fields, in display order
// (SPEC.md 15/S6).
var MoreDetailsFields = []string{"category", "relation", "phone", "nickname", "photo", "notes", "gender", "event_time"}

func flowBtn(text, action string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.EventFlow{Action: action}.Encode()}
}

func flowValBtn(text, action, value string) gotgbot.InlineKeyboardButton {
	v := value
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.EventFlow{Action: action, Value: &v}.Encode()}
}

func cardBtn(text, action string, eventID int64) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: text, CallbackData: callbacks.EventCard{Action: action, EventID: eventID}.Encode()}
}

func NameStepKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{CancelButton(i18n.T("common.cancel", lang, nil))}}}
}

func DateStepKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	noYear := flowBtn(i18n.T("add.date.no_year", lang, nil), "noyear")
	hebrew := flowBtn(i18n.T("add.date.hebrew", lang, nil), "heb")
	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{noYear}, {hebrew}, {CancelButton(i18n.T("common.cancel", lang, nil))}},
	}
}

// HebrewMonthKeyboard is a 3-per-row grid of the 12 base Hebrew months
// (Adar's sub-choice is a separate step).
func HebrewMonthKeyboard() *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, 12)
	for i := 1; i <= 12; i++ {
		buttons[i-1] = flowValBtn(core.HebrewMonthNames[i-1], "hm", strconv.Itoa(i))
	}
	rows := BuildGrid(buttons, 3)
	rows = append(rows, []gotgbot.InlineKeyboardButton{CancelButton(i18n.T("common.cancel", "he", nil))})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func HebrewAdarChoiceKeyboard() *gotgbot.InlineKeyboardMarkup {
	adarI := flowValBtn("אדר א׳", "hm", "12")
	adarII := flowValBtn("אדר ב׳", "hm", "13")
	plain := flowValBtn("סתם אדר", "hm", "12")
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{adarI, adarII}, {plain}}}
}

func HebrewDayKeyboard(monthLength int) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, monthLength)
	for day := 1; day <= monthLength; day++ {
		buttons[day-1] = flowValBtn(core.Gematria(day), "hd", strconv.Itoa(day))
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: BuildGrid(buttons, 5)}
}

func HebrewYearKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	noYear := flowBtn(i18n.T("add.date.no_year", lang, nil), "noyear")
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{noYear}}}
}

func SavedKeyboard(lang string, eventID int64) *gotgbot.InlineKeyboardMarkup {
	more := flowBtn(i18n.T("add.more_details", lang, nil), "more")
	changeReminder := cardBtn(i18n.T("add.change_reminder", lang, nil), "rem", eventID)
	addAnother := gotgbot.InlineKeyboardButton{Text: i18n.T("add.add_another", lang, nil), CallbackData: callbacks.Nav{Action: "cancel"}.Encode()}
	home := HomeButton(i18n.T("common.home", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{more, changeReminder}, {addAnother, home}}}
}

// CardKeyboard is the S8 event card action row.
func CardKeyboard(lang string, eventID int64, isActive bool, shareText string) *gotgbot.InlineKeyboardMarkup {
	btn := func(action, key string) gotgbot.InlineKeyboardButton {
		return cardBtn(i18n.T(key, lang, nil), action, eventID)
	}

	muteKey := "card.mute"
	if !isActive {
		muteKey = "card.unmute"
	}
	// A t.me/share/url link, not SwitchInlineQueryChosenChat: this bot has
	// no inline_query handler, so a switch-inline button would leave
	// "@BotUsername <shareText>" sitting unresolved in the target chat's
	// compose box — hitting send posts that literally (see
	// GreetingResultKeyboard's doc comment for the same issue).
	shareBtn := gotgbot.InlineKeyboardButton{
		Text: i18n.T("card.share", lang, nil),
		Url:  core.ShareURL(shareText),
	}
	back := gotgbot.InlineKeyboardButton{Text: i18n.T("card.back_to_list", lang, nil), CallbackData: callbacks.Menu{Action: "list"}.Encode()}
	home := HomeButton(i18n.T("common.home", lang, nil))

	return &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{btn("e", "card.edit"), btn("gr", "card.greeting")},
			{btn("rem", "card.reminders_btn"), btn("mute", muteKey)},
			{shareBtn, btn("d", "card.delete")},
			{back, home},
		},
	}
}

// EventFieldValues is the minimal read-only view more_details_keyboard needs
// of an event to render each field's current display value.
type EventFieldValues struct {
	Category, Relation, Phone, Nickname, Notes, Gender, EventTime, EventType *string
	HasPhoto                                                                 bool
}

func fieldDisplayValue(lang, field string, ev EventFieldValues) string {
	var raw *string
	switch field {
	case "category":
		raw = ev.Category
	case "relation":
		raw = ev.Relation
	case "phone":
		raw = ev.Phone
	case "nickname":
		raw = ev.Nickname
	case "notes":
		raw = ev.Notes
	case "gender":
		raw = ev.Gender
	case "event_time":
		raw = ev.EventTime
	case "event_type":
		raw = ev.EventType
	case "photo":
		if ev.HasPhoto {
			return i18n.T("common.saved", lang, nil)
		}
		return "—"
	}
	if raw == nil || *raw == "" {
		if field == "event_type" {
			return i18n.T("common.unknown", lang, nil)
		}
		return "—"
	}
	switch field {
	case "category":
		return i18n.T("category."+*raw, lang, nil)
	case "gender":
		return i18n.T("gender."+*raw, lang, nil)
	case "event_type":
		return i18n.T("event_type."+*raw, lang, nil)
	default:
		return *raw
	}
}

// MoreDetailsKeyboard is the S6 field picker. Each button shows the field's
// current value.
func MoreDetailsKeyboard(lang string, ev EventFieldValues) *gotgbot.InlineKeyboardMarkup {
	fieldBtn := func(field string) gotgbot.InlineKeyboardButton {
		value := fieldDisplayValue(lang, field, ev)
		return flowValBtn(i18n.T("more."+field, lang, map[string]any{"value": value}), "field", field)
	}
	buttons := make([]gotgbot.InlineKeyboardButton, len(MoreDetailsFields))
	for i, f := range MoreDetailsFields {
		buttons[i] = fieldBtn(f)
	}
	rows := BuildGrid(buttons, 2)

	typeBtn := flowValBtn(i18n.T("more.event_type", lang, map[string]any{"value": fieldDisplayValue(lang, "event_type", ev)}), "field", "event_type")
	doneBtn := flowBtn(i18n.T("more.done", lang, nil), "skip")
	rows = append(rows, []gotgbot.InlineKeyboardButton{typeBtn}, []gotgbot.InlineKeyboardButton{doneBtn})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// FieldPromptKeyboard is the [skip] [clear] [back] row under a field's
// text-entry prompt.
func FieldPromptKeyboard(lang, field string, hasValue bool) *gotgbot.InlineKeyboardMarkup {
	skip := flowBtn(i18n.T("common.skip", lang, nil), "skip")
	back := flowBtn(i18n.T("common.back", lang, nil), "more")
	row := []gotgbot.InlineKeyboardButton{skip}
	if hasValue {
		row = append(row, flowValBtn(i18n.T("field.clear", lang, nil), "clear", field))
	}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{row, {back}}}
}

func CategoryPickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.CategoryValues))
	for i, c := range core.CategoryValues {
		buttons[i] = flowValBtn(i18n.T("category."+c, lang, nil), "cat", c)
	}
	rows := BuildGrid(buttons, 2)
	rows = append(rows, []gotgbot.InlineKeyboardButton{flowBtn(i18n.T("common.back", lang, nil), "more")})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func GenderPickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.GenderValues))
	for i, g := range core.GenderValues {
		buttons[i] = flowValBtn(i18n.T("gender."+g, lang, nil), "gender", g)
	}
	back := flowBtn(i18n.T("common.back", lang, nil), "more")
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons, {back}}}
}

func EventTypePickerKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(core.EventTypeValues))
	for i, e := range core.EventTypeValues {
		buttons[i] = flowValBtn(i18n.T("event_type."+e, lang, nil), "type", e)
	}
	rows := BuildGrid(buttons, 2)
	rows = append(rows, []gotgbot.InlineKeyboardButton{flowBtn(i18n.T("common.back", lang, nil), "more")})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func DeleteConfirmKeyboard(lang string, eventID int64) *gotgbot.InlineKeyboardMarkup {
	yes := cardBtn(i18n.T("delete.confirm.yes", lang, nil), "dy", eventID)
	cancel := CancelButton(i18n.T("common.cancel", lang, nil))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{yes, cancel}}}
}

func DeleteDoneKeyboard(lang string, eventID int64) *gotgbot.InlineKeyboardMarkup {
	undo := cardBtn(i18n.T("delete.undo", lang, nil), "undo", eventID)
	back := gotgbot.InlineKeyboardButton{Text: i18n.T("card.back_to_list", lang, nil), CallbackData: callbacks.Menu{Action: "list"}.Encode()}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{undo}, {back}}}
}

// EventRowLabel pairs an event id with its display label for ListKeyboard.
type EventRowLabel struct {
	EventID int64
	Label   string
}

// ListKeyboard is the S3 event list screen. rowLabels is the current page
// (up to page_size).
func ListKeyboard(lang string, rowLabels []EventRowLabel, page, totalPages int) *gotgbot.InlineKeyboardMarkup {
	rows := make([][]gotgbot.InlineKeyboardButton, 0, len(rowLabels)+2)
	for _, rl := range rowLabels {
		rows = append(rows, []gotgbot.InlineKeyboardButton{cardBtn(rl.Label, "v", rl.EventID)})
	}

	sortBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("list.sort_btn", lang, nil), CallbackData: callbacks.List{Action: "sort", Value: "menu"}.Encode()}
	filterBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("list.filter_btn", lang, nil), CallbackData: callbacks.List{Action: "filt", Value: "menu"}.Encode()}
	searchBtn := gotgbot.InlineKeyboardButton{Text: i18n.T("list.search_btn", lang, nil), CallbackData: callbacks.Menu{Action: "srch"}.Encode()}
	home := HomeButton(i18n.T("common.home", lang, nil))

	if totalPages > 1 {
		rows = append(rows, PageRow(page, totalPages))
	}
	rows = append(rows, []gotgbot.InlineKeyboardButton{sortBtn, filterBtn, searchBtn})
	rows = append(rows, []gotgbot.InlineKeyboardButton{home})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

var sortOptions = []string{"upcoming", "name", "age", "created"}
var filterOptions = []string{"all", "muted"}

func SortMenuKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(sortOptions))
	for i, opt := range sortOptions {
		buttons[i] = gotgbot.InlineKeyboardButton{
			Text:         i18n.T("list.sort_"+opt, lang, nil),
			CallbackData: callbacks.List{Action: "fset_sort", Value: opt}.Encode(),
		}
	}
	rows := BuildGrid(buttons, 2)
	back := gotgbot.InlineKeyboardButton{Text: i18n.T("common.back", lang, nil), CallbackData: callbacks.Menu{Action: "list"}.Encode()}
	rows = append(rows, []gotgbot.InlineKeyboardButton{back})
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func FilterMenuKeyboard(lang string) *gotgbot.InlineKeyboardMarkup {
	buttons := make([]gotgbot.InlineKeyboardButton, len(filterOptions))
	for i, opt := range filterOptions {
		buttons[i] = gotgbot.InlineKeyboardButton{
			Text:         i18n.T("list.filter_option_"+opt, lang, nil),
			CallbackData: callbacks.List{Action: "fset_filt", Value: opt}.Encode(),
		}
	}
	back := gotgbot.InlineKeyboardButton{Text: i18n.T("common.back", lang, nil), CallbackData: callbacks.Menu{Action: "list"}.Encode()}
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{buttons, {back}}}
}
