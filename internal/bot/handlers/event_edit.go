package handlers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// RegisterEventEdit wires the S6 "more details" field editor — port of
// app/handlers/event_edit.py.
func RegisterEventEdit(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("e"), cbEventEditMenu))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("more"), cbMoreDetails))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("skip"), cbFieldSkipOrDone))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("field"), cbFieldPick))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("cat"), cbFieldSetCategory))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("gender"), cbFieldSetGender))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("type"), cbFieldSetEventType))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("clear"), cbFieldClear))
	dispatcher.AddHandler(handlers.NewMessage(hasPhoto, msgFieldPhoto))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgFieldValue))
}

func hasPhoto(msg *gotgbot.Message) bool { return len(msg.Photo) > 0 }

var timeRE = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

var fieldPromptKeys = map[string]string{
	"phone":      "edit.phone_prompt",
	"event_time": "edit.time_prompt",
	"photo":      "edit.photo_prompt",
	"year":       "edit.year_prompt",
}

func eventFieldValues(event *models.Event) keyboards.EventFieldValues {
	return keyboards.EventFieldValues{
		Category: &event.Category, Relation: event.Relation, Phone: event.Phone,
		Nickname: event.Nickname, Notes: event.Notes, Gender: event.Gender,
		EventTime: event.EventTime, EventType: &event.EventType,
		HasPhoto: event.PhotoFileID != nil && *event.PhotoFileID != "",
		Year:     event.Year,
	}
}

func renderMoreDetails(ctx context.Context, b *gotgbot.Bot, cq *gotgbot.CallbackQuery, db repo.DBTX, user *models.User, eventID int64) error {
	event, err := services.GetOwnedEvent(ctx, db, user, eventID)
	if err != nil {
		return err
	}
	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	text := i18n.T("more.title", user.Language, map[string]any{"name": name})
	return EditOrIgnore(b, cq, text, keyboards.MoreDetailsKeyboard(user.Language, eventID, eventFieldValues(event)))
}

func cbEventEditMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	eventID := mustEventID(ctx)

	store.SetState(user.ID, fsm.EditEventChoosingField)
	store.UpdateData(user.ID, map[string]any{"event_id": eventID})

	if err := renderMoreDetails(context.Background(), b, ctx.CallbackQuery, db, user, eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbMoreDetails(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)

	eventID, ok := fsmEventID(store, user.ID)
	if !ok {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	store.SetState(user.ID, fsm.EditEventChoosingField)

	if err := renderMoreDetails(context.Background(), b, ctx.CallbackQuery, db, user, eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// cbFieldSkipOrDone handles the "skip" action at two different steps:
// EditEvent.choosing_field ("✅ done" — leave edit mode, back to the card)
// and EditEvent.entering_value ("skip this field" — back to the field list).
// Mirrors cbAddNoYear's pattern of branching on caller state since Python
// distinguishes the two via separate per-state handler registrations.
func cbFieldSkipOrDone(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	state, ok := store.GetState(user.ID)
	if !ok {
		return ext.ContinueGroups
	}

	switch state {
	case fsm.EditEventChoosingField:
		eventID, ok := fsmEventID(store, user.ID)
		store.Clear(user.ID)
		if !ok {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		text, kb, err := RenderCard(context.Background(), db, user, eventID)
		if err != nil {
			if isNotFound(err) {
				NotFoundAlert(b, ctx.CallbackQuery, user.Language)
				return nil
			}
			return err
		}
		if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil

	case fsm.EditEventEnteringValue:
		eventID, ok := fsmEventID(store, user.ID)
		if !ok {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		store.SetState(user.ID, fsm.EditEventChoosingField)
		if err := renderMoreDetails(context.Background(), b, ctx.CallbackQuery, db, user, eventID); err != nil {
			if isNotFound(err) {
				NotFoundAlert(b, ctx.CallbackQuery, user.Language)
				return nil
			}
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil

	default:
		return ext.ContinueGroups
	}
}

func fsmEventID(store *fsm.Store, userID int64) (int64, bool) {
	data := store.GetData(userID)
	id, ok := data["event_id"].(int64)
	return id, ok
}

func cbFieldPick(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)

	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil {
		return err
	}
	raw := ""
	if decoded.Flow.Value != nil {
		raw = *decoded.Flow.Value
	}
	field, pickedID, ok := keyboards.ParseFieldPickValue(raw)
	if !ok {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}

	// The button must belong to the event currently being edited; an old
	// picker message for another event fails cleanly.
	eventID, ok := fsmEventID(store, user.ID)
	if state, _ := store.GetState(user.ID); !ok || eventID != pickedID || state != fsm.EditEventChoosingField {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	if _, err := services.GetOwnedEvent(context.Background(), db, user, eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	store.UpdateData(user.ID, map[string]any{"editing_field": field, "prompt_event_id": eventID})
	if field == "year" {
		// The year the user is looking at; the write only lands if it is
		// still the stored year.
		base := ""
		if ev, err := services.GetOwnedEvent(context.Background(), db, user, eventID); err == nil && ev.Year != nil {
			base = strconv.Itoa(*ev.Year)
		}
		store.UpdateData(user.ID, map[string]any{"year_base": base})
	}
	store.SetState(user.ID, fsm.EditEventEnteringValue)

	switch field {
	case "category":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_category", user.Language, nil), keyboards.CategoryPickerKeyboard(user.Language, eventID)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	case "gender":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_gender", user.Language, nil), keyboards.GenderPickerKeyboard(user.Language, eventID)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	case "event_type":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_event_type", user.Language, nil), keyboards.EventTypePickerKeyboard(user.Language, eventID)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	hasValue := fieldHasValue(db, user, eventID, field)
	promptKey := fieldPromptKeys[field]
	if promptKey == "" {
		promptKey = "field.prompt"
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T(promptKey, user.Language, nil), keyboards.FieldPromptKeyboard(user.Language, field, eventID, hasValue)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func fieldHasValue(db repo.DBTX, user *models.User, eventID int64, field string) bool {
	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		return false
	}
	v := eventFieldValues(event)
	switch field {
	case "phone":
		return v.Phone != nil && *v.Phone != ""
	case "event_time":
		return v.EventTime != nil && *v.EventTime != ""
	case "photo":
		return v.HasPhoto
	case "year":
		return v.Year != nil
	case "relation":
		return v.Relation != nil && *v.Relation != ""
	case "nickname":
		return v.Nickname != nil && *v.Nickname != ""
	case "notes":
		return v.Notes != nil && *v.Notes != ""
	default:
		return false
	}
}

func cbFieldSetCategory(b *gotgbot.Bot, ctx *ext.Context) error {
	return applyFieldFromCallback(b, ctx, "category")
}
func cbFieldSetGender(b *gotgbot.Bot, ctx *ext.Context) error {
	return applyFieldFromCallback(b, ctx, "gender")
}
func cbFieldSetEventType(b *gotgbot.Bot, ctx *ext.Context) error {
	return applyFieldFromCallback(b, ctx, "event_type")
}

func applyFieldFromCallback(b *gotgbot.Bot, ctx *ext.Context, field string) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.EditEventEnteringValue {
		return ext.ContinueGroups
	}

	value, ok := boundFieldValue(b, ctx, field)
	if !ok {
		return nil
	}
	return applyField(b, ctx, field, &value)
}

func cbFieldClear(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.EditEventEnteringValue {
		return ext.ContinueGroups
	}

	// For clear the bound value is the field itself; it must also be the
	// field the edit state is currently on.
	field, ok := boundFieldValue(b, ctx, "")
	if !ok {
		return nil
	}
	if editing, _ := store.GetData(user.ID)["editing_field"].(string); editing != field {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	return applyField(b, ctx, field, nil)
}

// boundFieldValue decodes a "<value>.<event id>" callback value and checks
// that it belongs to the event being edited and, when wantField is set, that
// the edit state is on that field. A stale button gets a not-found alert.
func boundFieldValue(b *gotgbot.Bot, ctx *ext.Context, wantField string) (string, bool) {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil || decoded.Flow.Value == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return "", false
	}
	value, boundID, ok := keyboards.ParseFieldPickValue(*decoded.Flow.Value)
	eventID, hasEvent := fsmEventID(store, user.ID)
	if !ok || !hasEvent || boundID != eventID {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return "", false
	}
	if wantField != "" {
		if editing, _ := store.GetData(user.ID)["editing_field"].(string); editing != wantField {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return "", false
		}
	}
	return value, true
}

// yearBase returns the year recorded when the year prompt opened. Without a
// record (state set up some other way) it falls back to the year just read.
func yearBase(store *fsm.Store, userID int64, current *int) *int {
	raw, ok := store.GetData(userID)["year_base"].(string)
	if !ok {
		return current
	}
	if raw == "" {
		return nil
	}
	y, err := strconv.Atoi(raw)
	if err != nil {
		return current
	}
	return &y
}

// applyField persists field=value on the event currently being edited
// (tracked in FSM data), then returns to the S6 field picker.
func applyField(b *gotgbot.Bot, ctx *ext.Context, field string, value *string) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)

	eventID, ok := fsmEventID(store, user.ID)
	if !ok {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	if field == "year" {
		_, err = services.UpdateEventYear(context.Background(), db, user, eventID, yearBase(store, user.ID, event.Year), nil)
	} else {
		setEventField(event, field, value)
		_, err = services.UpdateEvent(context.Background(), db, user, event)
	}
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		if isConflict(err) {
			AnswerCallback(b, ctx.CallbackQuery, i18n.T("error.conflict", user.Language, nil))
			return nil
		}
		return err
	}
	router.LoggerFromContext(ctx).Info("event_updated",
		"user_id", user.ID,
		"event_id", eventID,
		"field", field,
		"cleared", value == nil,
	)

	store.SetState(user.ID, fsm.EditEventChoosingField)
	if err := renderMoreDetails(context.Background(), b, ctx.CallbackQuery, db, user, eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// setEventField mutates the one Event struct field named by field — Go's
// equivalent of Python's update_event_field(session, user, event_id,
// **{field: value}) dynamic setattr.
func setEventField(event *models.Event, field string, value *string) {
	switch field {
	case "category":
		if value != nil {
			event.Category = *value
		}
	case "relation":
		event.Relation = value
	case "phone":
		event.Phone = value
	case "nickname":
		event.Nickname = value
	case "notes":
		event.Notes = value
	case "gender":
		event.Gender = value
	case "event_time":
		event.EventTime = value
	case "event_type":
		if value != nil {
			event.EventType = *value
		}
	case "photo":
		event.PhotoFileID = value
	case "year":
		// Callers validate the number first; an unparsable value is ignored
		// rather than silently clearing the year.
		if value == nil {
			event.Year = nil
		} else if y, err := strconv.Atoi(*value); err == nil {
			event.Year = &y
		}
	}
}

// parseEditYear validates a typed birth year for event: a Gregorian year
// for a Gregorian event, a Hebrew year (Hebrew or Gregorian input accepted,
// like the add flow) for a Hebrew event. Future years and a Feb 29 birthday
// in a non-leap Gregorian year are rejected.
func parseEditYear(event *models.Event, raw string, now time.Time, adarPolicy string) (int, error) {
	if event.CalendarType == core.CalendarTypeHebrew {
		year, err := core.ParseHebrewYearInputAnchored(raw, event.Month, event.Day, adarPolicy, event.SecondaryMonth, event.SecondaryDay)
		if err != nil {
			return 0, err
		}
		if currentHYear, _, _ := core.ToHebrew(now); year > currentHYear {
			return 0, errBadYear
		}
		return year, nil
	}
	cleaned := strings.TrimSpace(raw)
	year, err := strconv.Atoi(cleaned)
	if err != nil || strconv.Itoa(year) != cleaned {
		return 0, errBadYear
	}
	if _, err := core.ValidateYear(year, false); err != nil {
		return 0, err
	}
	if year > now.Year() {
		return 0, errBadYear
	}
	if event.Month == 2 && event.Day == 29 && time.Date(year, 2, 29, 0, 0, 0, 0, time.UTC).Month() != time.February {
		return 0, errBadYear
	}
	return year, nil
}

var errBadYear = errors.New("❌ שנה לא תקינה.")

// yearErrorText is the user-facing text for a year parse error. An ambiguous
// Gregorian year (a Tevet birthday near New Year can sit in two Hebrew
// years) asks which date is right instead of guessing.
func yearErrorText(err error, user *models.User) string {
	var amb *core.AmbiguousHebrewYearError
	if errors.As(err, &amb) {
		return fmt.Sprintf("🤔 השנה הזו יכולה להתאים לשתי שנים עבריות:\n• %d · %s\n• %d · %s\n\nאיזה תאריך נכון? שלח את השנה העברית הנכונה (למשל %d).",
			amb.Candidates[0], core.FormatDate(amb.Dates[0], user.DateFormat),
			amb.Candidates[1], core.FormatDate(amb.Dates[1], user.DateFormat),
			amb.Candidates[1])
	}
	return err.Error()
}

// anchorHebrewYear uses a stored Gregorian secondary date as the anchor when
// a Hebrew event's year was typed as a Gregorian year: of the two candidate
// Hebrew years (greg+3760, greg+3761) it picks the one whose exact birth date
// falls on the stored month/day. The stored date itself is never modified; a
// year edit must not overwrite it. Returns year unchanged when there is no
// secondary date, no typed Gregorian year, or no candidate matches.
func anchorHebrewYear(event *models.Event, raw string, year int, user *models.User) int {
	if event.CalendarType != core.CalendarTypeHebrew ||
		event.SecondaryMonth == nil || event.SecondaryDay == nil {
		return year
	}
	greg, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || greg < core.MinYearGregorian || greg > core.MaxYearGregorian ||
		(greg >= core.MinYearHebrew && greg <= core.MaxYearHebrew) {
		return year
	}
	for _, h := range [2]int{greg + 3760, greg + 3761} {
		d, err := exactGregorianBirthDate(h, event.Month, event.Day, user.AdarPolicy)
		if err == nil && d.Year() == greg &&
			int(d.Month()) == *event.SecondaryMonth && d.Day() == *event.SecondaryDay {
			return h
		}
	}
	return year
}

// promptMatchesEvent reports whether the open field prompt was asked for the
// event now being edited.
func promptMatchesEvent(data map[string]any, eventID int64) bool {
	id, ok := data["prompt_event_id"].(int64)
	return ok && id == eventID
}

func refuseStalePrompt(b *gotgbot.Bot, ctx *ext.Context, user *models.User) error {
	_, err := ctx.EffectiveMessage.Reply(b, i18n.T("edit.stale_prompt", user.Language, nil), nil)
	return err
}

func msgFieldValue(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.EditEventEnteringValue {
		return ext.ContinueGroups
	}

	data := store.GetData(user.ID)
	field, _ := data["editing_field"].(string)
	eventID, hasEventID := fsmEventID(store, user.ID)
	if field == "" || !hasEventID || field == "photo" {
		return ext.ContinueGroups
	}
	if !promptMatchesEvent(data, eventID) {
		return refuseStalePrompt(b, ctx, user)
	}

	raw := ctx.EffectiveMessage.Text
	var value string

	if field == "year" {
		// Parsed against the event's calendar, so load it first.
		ev, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
		if err != nil {
			if isNotFound(err) {
				_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
				return sendErr
			}
			return err
		}
		year, err := parseEditYear(ev, raw, timeNow(), user.AdarPolicy)
		if err != nil {
			_, sendErr := ctx.EffectiveMessage.Reply(b, yearErrorText(err, user), nil)
			return sendErr
		}
		year = anchorHebrewYear(ev, raw, year, user)
		value = strconv.Itoa(year)
	} else if field == "event_time" {
		m := timeRE.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			_, err := ctx.EffectiveMessage.Reply(b, i18n.T("edit.time_error", user.Language, nil), nil)
			return err
		}
		hour, _ := strconv.Atoi(m[1])
		value = fmt.Sprintf("%02d:%s", hour, m[2])
	} else {
		var err error
		switch field {
		case "relation":
			value, err = core.ValidateRelation(raw)
		case "phone":
			value, err = core.ValidatePhone(raw)
		case "nickname":
			value, err = core.ValidateNickname(raw)
		case "notes":
			value, err = core.ValidateNotes(raw)
		default:
			return ext.ContinueGroups
		}
		if err != nil {
			_, sendErr := ctx.EffectiveMessage.Reply(b, err.Error(), nil)
			return sendErr
		}
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
			return sendErr
		}
		return err
	}
	var updated *models.Event
	if field == "year" {
		y, _ := strconv.Atoi(value)
		updated, err = services.UpdateEventYear(context.Background(), db, user, eventID, yearBase(store, user.ID, event.Year), &y)
	} else {
		setEventField(event, field, &value)
		updated, err = services.UpdateEvent(context.Background(), db, user, event)
	}
	if err != nil {
		if isNotFound(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
			return sendErr
		}
		if isConflict(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.conflict", user.Language, nil), nil)
			return sendErr
		}
		return err
	}
	router.LoggerFromContext(ctx).Info("event_updated",
		"user_id", user.ID,
		"event_id", eventID,
		"field", field,
		"cleared", false,
	)

	store.SetState(user.ID, fsm.EditEventChoosingField)
	name := core.Esc(core.FormatName(updated.FirstName, updated.LastName))
	text := i18n.T("edit.field_saved", user.Language, nil) + "\n\n" + i18n.T("more.title", user.Language, map[string]any{"name": name})
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: keyboards.MoreDetailsKeyboard(user.Language, updated.ID, eventFieldValues(updated)), ParseMode: "HTML",
	})
	return sendErr
}

// msgFieldPhoto handles the one field msgFieldValue deliberately punts on
// (see its "field == photo" early return) — port of app/handlers/
// event_edit.py's msg_field_photo. A photo-only Telegram message carries no
// Text (only Photo/Caption), so it would never have matched msgFieldValue's
// anyText filter anyway; this was previously entirely unregistered, so a
// user replying to "שלח תמונה:" with an actual photo fell through every
// handler to the global fallback ("לא הבנתי").
func msgFieldPhoto(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.EditEventEnteringValue {
		return ext.ContinueGroups
	}

	data := store.GetData(user.ID)
	field, _ := data["editing_field"].(string)
	eventID, hasEventID := fsmEventID(store, user.ID)
	if field != "photo" || !hasEventID {
		return ext.ContinueGroups
	}
	if !promptMatchesEvent(data, eventID) {
		return refuseStalePrompt(b, ctx, user)
	}

	photos := ctx.EffectiveMessage.Photo
	fileID := photos[len(photos)-1].FileId // largest available resolution

	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
			return sendErr
		}
		return err
	}
	setEventField(event, "photo", &fileID)
	updated, err := services.UpdateEvent(context.Background(), db, user, event)
	if err != nil {
		if isNotFound(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
			return sendErr
		}
		return err
	}
	router.LoggerFromContext(ctx).Info("event_updated",
		"user_id", user.ID,
		"event_id", eventID,
		"field", "photo",
		"cleared", false,
	)

	store.SetState(user.ID, fsm.EditEventChoosingField)
	name := core.Esc(core.FormatName(updated.FirstName, updated.LastName))
	text := i18n.T("edit.field_saved", user.Language, nil) + "\n\n" + i18n.T("more.title", user.Language, map[string]any{"name": name})
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: keyboards.MoreDetailsKeyboard(user.Language, updated.ID, eventFieldValues(updated)), ParseMode: "HTML",
	})
	return sendErr
}
