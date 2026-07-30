package handlers

import (
	"context"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// RegisterEventCard wires the event card view/mute/delete/undo actions —
// port of app/handlers/event_card.py.
func RegisterEventCard(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("v"), cbEventView))
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("mute"), cbEventMute))
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("d"), cbEventDeleteConfirm))
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("dy"), cbEventDeleteConfirmed))
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("undo"), cbEventRestore))
}

// eventCardActionFilter matches "ev:<action>:<id>" payloads that decode to
// the EventCard shape specifically (see the callbacks package doc).
func eventCardActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		decoded, err := callbacks.DecodeEvent(cq.Data)
		return err == nil && decoded.Card != nil && decoded.Card.Action == action
	}
}

func shareText(user *models.User, event *models.Event) string {
	name := core.FormatName(event.FirstName, event.LastName)
	today := services.UserToday(user)
	countdown := core.FormatCountdown(core.DaysUntil(*event.NextOccurrence, today))
	dateStr := core.FormatDate(*event.NextOccurrence, user.DateFormat)
	if age, ok := core.AgeAt(event.CalendarType, event.Year, *event.NextOccurrence); ok {
		return "🎂 " + name + " · " + dateStr + " (" + strconv.Itoa(age) + ") · " + countdown
	}
	return "🎂 " + name + " · " + dateStr + " · " + countdown
}

// RenderCard builds the S8 event card text + keyboard for eventID, scoped
// to user. Returns services.NotFoundErr if the event doesn't exist or isn't
// owned by user.
func RenderCard(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (string, *gotgbot.InlineKeyboardMarkup, error) {
	event, err := services.GetOwnedEvent(ctx, db, user, eventID)
	if err != nil {
		return "", nil, err
	}
	rules, err := repo.NewReminderRuleRepo(db, user.ID).ListForEvent(ctx, eventID)
	if err != nil {
		return "", nil, err
	}
	text := services.RenderCardText(user, event, rules)
	kb := keyboards.CardKeyboard(user.Language, eventID, event.IsActive, shareText(user, event))
	return text, kb, nil
}

func isNotFound(err error) bool {
	_, ok := err.(*services.NotFoundErr)
	return ok
}

func cbEventView(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

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
}

func cbEventMute(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

	if _, err := services.ToggleMute(context.Background(), db, user, eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
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
}

func cbEventDeleteConfirm(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	text := i18n.T("delete.confirm.title", user.Language, map[string]any{"name": name}) + "\n\n" + i18n.T("delete.confirm.body", user.Language, nil)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.DeleteConfirmKeyboard(user.Language, eventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbEventDeleteConfirmed(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

	event, err := services.DeleteEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	kw := map[string]any{"name": name}
	if event.Gender != nil {
		kw["gender"] = *event.Gender
	}
	text := i18n.T("delete.done", user.Language, kw)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.DeleteDoneKeyboard(user.Language, eventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbEventRestore(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

	event, err := services.RestoreEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	kw := map[string]any{"name": name}
	if event.Gender != nil {
		kw["gender"] = *event.Gender
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("delete.restored", user.Language, kw))

	text, kb, err := RenderCard(context.Background(), db, user, eventID)
	if err != nil {
		return err
	}
	return EditOrIgnore(b, ctx.CallbackQuery, text, kb)
}

func mustEventID(ctx *ext.Context) int64 {
	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Card == nil {
		return 0
	}
	return decoded.Card.EventID
}
