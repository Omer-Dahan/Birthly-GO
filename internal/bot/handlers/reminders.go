package handlers

import (
	"context"
	"strconv"
	"strings"

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

// RegisterReminders wires the S11 reminder rules screens — port of
// app/handlers/reminders.py.
func RegisterReminders(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("rem"), cbMenuReminders))
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("rem"), cbEventReminders))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("back"), cbRemBack))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("add"), cbRemAdd))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("off"), cbRemPickOffset))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("menu"), cbRemRuleMenu))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("tog"), cbRemToggle))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("del"), cbRemDelete))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("time_menu"), cbRemTimeMenu))
	dispatcher.AddHandler(handlers.NewCallback(reminderActionFilter("time"), cbRemSetTime))
}

func reminderActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixReminder {
			return false
		}
		r, err := callbacks.DecodeReminder(cq.Data)
		return err == nil && r.Action == action
	}
}

func decodeReminderCB(ctx *ext.Context) (callbacks.Reminder, error) {
	return callbacks.DecodeReminder(ctx.CallbackQuery.Data)
}

func renderRules(ctx context.Context, b *gotgbot.Bot, cq *gotgbot.CallbackQuery, db repo.DBTX, user *models.User, eventID *int64) error {
	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)
	var text string
	var rules []*models.ReminderRule
	var err error

	if eventID == nil {
		rules, err = ruleRepo.ListGlobal(ctx)
		if err != nil {
			return err
		}
		text = i18n.T("rem.global.title", user.Language, nil) + "\n\n" + i18n.T("rem.global.hint", user.Language, nil)
	} else {
		event, gerr := services.GetOwnedEvent(ctx, db, user, *eventID)
		if gerr != nil {
			return gerr
		}
		rules, err = ruleRepo.ListForEvent(ctx, *eventID)
		if err != nil {
			return err
		}
		name := core.Esc(core.FormatName(event.FirstName, event.LastName))
		text = i18n.T("rem.event.title", user.Language, map[string]any{"name": name})
	}

	return EditOrIgnore(b, cq, text, keyboards.RulesListKeyboard(user.Language, rules, eventID))
}

func cbMenuReminders(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	store.UpdateData(user.ID, map[string]any{"rem_event_id": nil})

	if err := renderRules(context.Background(), b, ctx.CallbackQuery, db, user, nil); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbEventReminders(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	eventID := mustEventID(ctx)
	store.UpdateData(user.ID, map[string]any{"rem_event_id": eventID})

	if err := renderRules(context.Background(), b, ctx.CallbackQuery, db, user, &eventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemBack(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	if err := renderRules(context.Background(), b, ctx.CallbackQuery, db, user, rc.EventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemAdd(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("rem.add_title", user.Language, nil), keyboards.AddRuleKeyboard(user.Language, rc.EventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemPickOffset(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	offsetDays := 0
	if rc.Value != nil {
		offsetDays, _ = strconv.Atoi(*rc.Value)
	}

	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)
	var existing, limit int
	if rc.EventID != nil {
		existing, err = ruleRepo.CountForEvent(context.Background(), *rc.EventID)
		limit = core.MaxReminderRulesPerEvent
	} else {
		existing, err = ruleRepo.CountGlobal(context.Background())
		limit = core.MaxReminderRulesGlobal
	}
	if err != nil {
		return err
	}
	if existing >= limit {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("rem.max_reached", user.Language, map[string]any{"max": limit}))
		return nil
	}

	if rc.EventID != nil {
		if _, err := services.GetOwnedEvent(context.Background(), db, user, *rc.EventID); err != nil {
			if isNotFound(err) {
				NotFoundAlert(b, ctx.CallbackQuery, user.Language)
				return nil
			}
			return err
		}
	}

	if _, err := ruleRepo.Create(context.Background(), &models.ReminderRule{
		EventID: rc.EventID, OffsetDays: &offsetDays, Enabled: true,
	}); err != nil {
		return err
	}

	label := services.OffsetLabel(offsetDays, user.Language)
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("rem.added", user.Language, map[string]any{"label": label, "time": user.DefaultNotifyTime}))
	return renderRules(context.Background(), b, ctx.CallbackQuery, db, user, rc.EventID)
}

func cbRemRuleMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	ruleID := int64(0)
	if rc.Value != nil {
		ruleID, _ = strconv.ParseInt(*rc.Value, 10, 64)
	}

	rule, err := repo.NewReminderRuleRepo(db, user.ID).GetOwned(context.Background(), ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}

	label := "?"
	if rule.OffsetDays != nil {
		label = services.OffsetLabel(*rule.OffsetDays, user.Language)
	}
	timeStr := user.DefaultNotifyTime
	if rule.SendTime != nil && *rule.SendTime != "" {
		timeStr = *rule.SendTime
	}
	text := i18n.T("rem.rule_menu_title", user.Language, map[string]any{"label": label, "time": timeStr})
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.RuleMenuKeyboard(user.Language, ruleID, rc.EventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	ruleID := int64(0)
	if rc.Value != nil {
		ruleID, _ = strconv.ParseInt(*rc.Value, 10, 64)
	}

	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)
	rule, err := ruleRepo.GetOwned(context.Background(), ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	if _, err := ruleRepo.Toggle(context.Background(), ruleID); err != nil {
		return err
	}
	if err := renderRules(context.Background(), b, ctx.CallbackQuery, db, user, rc.EventID); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemDelete(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	ruleID := int64(0)
	if rc.Value != nil {
		ruleID, _ = strconv.ParseInt(*rc.Value, 10, 64)
	}

	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)
	rule, err := ruleRepo.GetOwned(context.Background(), ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	if err := ruleRepo.Delete(context.Background(), ruleID); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("rem.deleted", user.Language, nil))
	return renderRules(context.Background(), b, ctx.CallbackQuery, db, user, rc.EventID)
}

func cbRemTimeMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	ruleID := int64(0)
	if rc.Value != nil {
		ruleID, _ = strconv.ParseInt(*rc.Value, 10, 64)
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("rem.change_time", user.Language, nil), keyboards.TimeChoiceKeyboard(user.Language, ruleID, rc.EventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbRemSetTime(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	rc, err := decodeReminderCB(ctx)
	if err != nil {
		return err
	}
	if rc.Value == nil || rc.RuleID == nil {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("error.generic", user.Language, nil))
		return nil
	}
	hh, mm, found := strings.Cut(*rc.Value, "-")
	if !found || !isAllDigits(hh) || !isAllDigits(mm) {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("error.generic", user.Language, nil))
		return nil
	}
	hour, _ := strconv.Atoi(hh)
	sendTime := fmtHHMM(hour, mm)
	ruleID := *rc.RuleID

	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)
	rule, err := ruleRepo.GetOwned(context.Background(), ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}
	rule.SendTime = &sendTime
	if _, err := ruleRepo.Update(context.Background(), rule); err != nil {
		return err
	}

	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return renderRules(context.Background(), b, ctx.CallbackQuery, db, user, rc.EventID)
}

func fmtHHMM(hour int, mm string) string {
	h := strconv.Itoa(hour)
	if hour < 10 {
		h = "0" + h
	}
	return h + ":" + mm
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
