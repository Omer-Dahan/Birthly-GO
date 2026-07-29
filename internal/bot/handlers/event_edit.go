package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
}

func eventFieldValues(event *models.Event) keyboards.EventFieldValues {
	return keyboards.EventFieldValues{
		Category: &event.Category, Relation: event.Relation, Phone: event.Phone,
		Nickname: event.Nickname, Notes: event.Notes, Gender: event.Gender,
		EventTime: event.EventTime, EventType: &event.EventType,
		HasPhoto: event.PhotoFileID != nil && *event.PhotoFileID != "",
	}
}

func renderMoreDetails(ctx context.Context, b *gotgbot.Bot, cq *gotgbot.CallbackQuery, db repo.DBTX, user *models.User, eventID int64) error {
	event, err := services.GetOwnedEvent(ctx, db, user, eventID)
	if err != nil {
		return err
	}
	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	text := i18n.T("more.title", user.Language, map[string]any{"name": name})
	return EditOrIgnore(b, cq, text, keyboards.MoreDetailsKeyboard(user.Language, eventFieldValues(event)))
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
	field := ""
	if decoded.Flow.Value != nil {
		field = *decoded.Flow.Value
	}

	eventID, ok := fsmEventID(store, user.ID)
	if !ok {
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

	store.UpdateData(user.ID, map[string]any{"editing_field": field})
	store.SetState(user.ID, fsm.EditEventEnteringValue)

	switch field {
	case "category":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_category", user.Language, nil), keyboards.CategoryPickerKeyboard(user.Language)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	case "gender":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_gender", user.Language, nil), keyboards.GenderPickerKeyboard(user.Language)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	case "event_type":
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("edit.pick_event_type", user.Language, nil), keyboards.EventTypePickerKeyboard(user.Language)); err != nil {
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
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T(promptKey, user.Language, nil), keyboards.FieldPromptKeyboard(user.Language, field, hasValue)); err != nil {
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

	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil {
		return err
	}
	return applyField(b, ctx, field, decoded.Flow.Value)
}

func cbFieldClear(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.EditEventEnteringValue {
		return ext.ContinueGroups
	}

	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil {
		return err
	}
	field := ""
	if decoded.Flow.Value != nil {
		field = *decoded.Flow.Value
	}
	return applyField(b, ctx, field, nil)
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
	setEventField(event, field, value)
	if _, err := services.UpdateEvent(context.Background(), db, user, event); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
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
	}
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

	raw := ctx.EffectiveMessage.Text
	var value string

	if field == "event_time" {
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
	setEventField(event, field, &value)
	updated, err := services.UpdateEvent(context.Background(), db, user, event)
	if err != nil {
		if isNotFound(err) {
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.not_found", user.Language, nil), nil)
			return sendErr
		}
		return err
	}

	store.SetState(user.ID, fsm.EditEventChoosingField)
	name := core.Esc(core.FormatName(updated.FirstName, updated.LastName))
	text := i18n.T("edit.field_saved", user.Language, nil) + "\n\n" + i18n.T("more.title", user.Language, map[string]any{"name": name})
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: keyboards.MoreDetailsKeyboard(user.Language, eventFieldValues(updated)), ParseMode: "HTML",
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

	store.SetState(user.ID, fsm.EditEventChoosingField)
	name := core.Esc(core.FormatName(updated.FirstName, updated.LastName))
	text := i18n.T("edit.field_saved", user.Language, nil) + "\n\n" + i18n.T("more.title", user.Language, map[string]any{"name": name})
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: keyboards.MoreDetailsKeyboard(user.Language, eventFieldValues(updated)), ParseMode: "HTML",
	})
	return sendErr
}
