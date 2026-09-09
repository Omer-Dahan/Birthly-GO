package handlers

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

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

// RegisterEventAdd wires the add-event flow — port of app/handlers/event_add.py.
func RegisterEventAdd(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("add"), cbMenuAdd))

	dispatcher.AddHandler(handlers.NewMessage(anyText, msgAddName))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgAddDate))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgAddHebrewYear))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgAddSecondaryDate))

	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("noyear"), cbAddNoYear))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("heb"), cbAddHebrewTrack))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("hm"), cbAddHebrewMonth))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("hd"), cbAddHebrewDay))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("secyes"), cbAddSecondaryYes))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("secno"), cbAddSecondaryNo))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("autoyes"), cbAddSecondaryAutoYes))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("automanual"), cbAddSecondaryAutoManual))
	dispatcher.AddHandler(handlers.NewCallback(eventFlowActionFilter("autono"), cbAddSecondaryAutoNo))
}

func anyText(msg *gotgbot.Message) bool { return msg.Text != "" }

func menuActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixMenu {
			return false
		}
		m, err := callbacks.DecodeMenu(cq.Data)
		return err == nil && m.Action == action
	}
}

// eventFlowActionFilter matches "ev:<action>:..." payloads that decode to
// the EventFlow shape specifically (not EventCard — see the callbacks
// package doc on why the two share a prefix).
func eventFlowActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		decoded, err := callbacks.DecodeEvent(cq.Data)
		return err == nil && decoded.Flow != nil && decoded.Flow.Action == action
	}
}

func cbMenuAdd(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.AddEventName)

	text := i18n.T("add.name.title", user.Language, nil) + "\n<i>" + i18n.T("add.name.hint", user.Language, nil) + "</i>"
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.NameStepKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgAddName(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventName {
		return ext.ContinueGroups
	}

	cleaned, err := core.ValidateName(ctx.EffectiveMessage.Text)
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, err.Error(), nil)
		return sendErr
	}

	firstName, lastName := core.SplitName(cleaned)
	store.UpdateData(user.ID, map[string]any{"first_name": firstName, "last_name": lastName})
	store.SetState(user.ID, fsm.AddEventDate)

	gender, _ := core.DetectGender(firstName)
	genderKw := map[string]any{}
	if gender != "" {
		genderKw["gender"] = gender
	}
	title := i18n.T("add.date.title", user.Language, mergeKwargs(map[string]any{"name": core.Esc(firstName)}, genderKw))
	text := title + "\n\n" + i18n.T("add.date.hint", user.Language, nil)
	_, err2 := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.DateStepKeyboard(user.Language), ParseMode: "HTML"})
	return err2
}

func mergeKwargs(base map[string]any, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func msgAddDate(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventDate {
		return ext.ContinueGroups
	}

	parsed, err := core.ParseGregorian(ctx.EffectiveMessage.Text, timeNow())
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, err.Error(), nil)
		return sendErr
	}

	db := router.DBFromContext(ctx)
	maxEvents := router.ConfigFromContext(ctx).MaxEventsPerUser
	event, limitErr, err := finalizeEvent(context.Background(), db, user, store, parsed.Month, parsed.Day, parsed.Year, core.CalendarTypeGregorian, nil, nil, maxEvents)
	if err != nil {
		return err
	}
	if limitErr {
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.limit_reached", user.Language, map[string]any{"max": maxEvents}), nil)
		return sendErr
	}
	text := renderSavedText(user, event)
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SavedKeyboard(user.Language, event.ID), ParseMode: "HTML"})
	return sendErr
}

func cbAddNoYear(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	state, ok := store.GetState(user.ID)
	if !ok {
		return ext.ContinueGroups
	}

	switch state {
	case fsm.AddEventDate:
		text := i18n.T("add.date.no_year_prompt", user.Language, nil)
		if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.DateStepKeyboard(user.Language)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil

	case fsm.AddEventHebYear:
		store.SetState(user.ID, fsm.AddEventSecondaryPrompt)
		text := i18n.T("add.secondary.prompt", user.Language, nil)
		if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.SecondaryDatePromptKeyboard(user.Language)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil

	default:
		return ext.ContinueGroups
	}
}

func cbAddHebrewTrack(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventDate {
		return ext.ContinueGroups
	}

	store.SetState(user.ID, fsm.AddEventHebMonth)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("add.heb_month.title", user.Language, nil), keyboards.HebrewMonthKeyboard()); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbAddHebrewMonth(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventHebMonth {
		return ext.ContinueGroups
	}

	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil {
		return err
	}
	month := 0
	if decoded.Flow.Value != nil {
		month, _ = strconv.Atoi(*decoded.Flow.Value)
	}

	data := store.GetData(user.ID)
	adarShown, _ := data["adar_choice_shown"].(bool)
	if month == 12 && !adarShown {
		store.UpdateData(user.ID, map[string]any{"adar_choice_shown": true})
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("add.heb_month.title", user.Language, nil), keyboards.HebrewAdarChoiceKeyboard()); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	store.UpdateData(user.ID, map[string]any{"heb_month": month})
	store.SetState(user.ID, fsm.AddEventHebDay)

	currentHebYear, _, _ := core.ToHebrew(timeNow())
	daysInMonth, err := core.MonthLength(currentHebYear, month)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("add.heb_day.title", user.Language, nil), keyboards.HebrewDayKeyboard(daysInMonth)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbAddHebrewDay(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventHebDay {
		return ext.ContinueGroups
	}

	decoded, err := callbacks.DecodeEvent(ctx.CallbackQuery.Data)
	if err != nil || decoded.Flow == nil {
		return err
	}
	day := 0
	if decoded.Flow.Value != nil {
		day, _ = strconv.Atoi(*decoded.Flow.Value)
	}

	store.UpdateData(user.ID, map[string]any{"heb_day": day})
	store.SetState(user.ID, fsm.AddEventHebYear)

	text := i18n.T("add.heb_year.title", user.Language, nil) + "\n<i>" + i18n.T("add.heb_year.hint", user.Language, nil) + "</i>"
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.HebrewYearKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// exactGregorianBirthDate resolves the exact civil date a hebrew birthday
// (year+month+day) fell on. ResolveMonth always yields a month that exists
// in hebYear (adar_i/adar_ii only matter in leap years; both resolve to the
// single Adar in a non-leap year), so ToGregorian here should never error in
// practice. Callers keep the error return for the same defensive reason
// ToGregorian itself does.
func exactGregorianBirthDate(hebYear, hebMonth, hebDay int, adarPolicy string) (time.Time, error) {
	resolvedMonth := core.ResolveMonth(hebYear, hebMonth, adarPolicy)
	return core.ToGregorian(hebYear, resolvedMonth, hebDay)
}

func msgAddHebrewYear(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventHebYear {
		return ext.ContinueGroups
	}

	year, err := core.ParseHebrewYearInput(ctx.EffectiveMessage.Text)
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, err.Error(), nil)
		return sendErr
	}

	store.UpdateData(user.ID, map[string]any{"heb_year": year})

	data := store.GetData(user.ID)
	hebMonth, _ := data["heb_month"].(int)
	hebDay, _ := data["heb_day"].(int)
	if greg, gErr := exactGregorianBirthDate(year, hebMonth, hebDay, user.AdarPolicy); gErr == nil {
		store.UpdateData(user.ID, map[string]any{
			"auto_sec_month": int(greg.Month()),
			"auto_sec_day":   greg.Day(),
		})
		store.SetState(user.ID, fsm.AddEventSecondaryAutoConfirm)

		text := i18n.T("add.secondary_auto.prompt", user.Language, map[string]any{"date": core.FormatDate(greg, user.DateFormat)})
		_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SecondaryAutoConfirmKeyboard(user.Language)})
		return sendErr
	}

	store.SetState(user.ID, fsm.AddEventSecondaryPrompt)
	text := i18n.T("add.secondary.prompt", user.Language, nil)
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SecondaryDatePromptKeyboard(user.Language)})
	return sendErr
}

// cbAddSecondaryYes handles "yes, add a Gregorian date too" from the
// secondary-date prompt shown after a hebrew-primary date is collected.
func cbAddSecondaryYes(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryPrompt {
		return ext.ContinueGroups
	}

	store.SetState(user.ID, fsm.AddEventSecondaryDate)
	text := i18n.T("add.secondary.date_title", user.Language, nil) + "\n\n" + i18n.T("add.secondary.date_hint", user.Language, nil)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.SecondaryDateStepKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// cbAddSecondaryNo handles "no thanks" from the secondary-date prompt:
// finalizes the event with only its hebrew primary date.
func cbAddSecondaryNo(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryPrompt {
		return ext.ContinueGroups
	}
	return finalizeSecondaryAndRespond(b, ctx, user, store, nil, nil)
}

// cbAddSecondaryAutoYes accepts the gregorian date auto-computed from the
// hebrew year/month/day just entered as the event's secondary date.
func cbAddSecondaryAutoYes(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryAutoConfirm {
		return ext.ContinueGroups
	}

	data := store.GetData(user.ID)
	month, _ := data["auto_sec_month"].(int)
	day, _ := data["auto_sec_day"].(int)
	return finalizeSecondaryAndRespond(b, ctx, user, store, &month, &day)
}

// cbAddSecondaryAutoManual lets the user discard the auto-computed date and
// type a gregorian secondary date manually instead.
func cbAddSecondaryAutoManual(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryAutoConfirm {
		return ext.ContinueGroups
	}

	store.SetState(user.ID, fsm.AddEventSecondaryDate)
	text := i18n.T("add.secondary.date_title", user.Language, nil) + "\n\n" + i18n.T("add.secondary.date_hint", user.Language, nil)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.SecondaryDateStepKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// cbAddSecondaryAutoNo declines the auto-computed date and finalizes the
// event with only its hebrew primary date.
func cbAddSecondaryAutoNo(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryAutoConfirm {
		return ext.ContinueGroups
	}
	return finalizeSecondaryAndRespond(b, ctx, user, store, nil, nil)
}

// finalizeSecondaryAndRespond finalizes a hebrew-primary event (optionally
// with a gregorian secondary date) and edits the triggering callback's
// message into the saved-event card, shared by every terminal branch of the
// secondary-date prompt (manual "no thanks", auto-confirm accept/decline).
func finalizeSecondaryAndRespond(b *gotgbot.Bot, ctx *ext.Context, user *models.User, store *fsm.Store, secondaryMonth, secondaryDay *int) error {
	db := router.DBFromContext(ctx)
	maxEvents := router.ConfigFromContext(ctx).MaxEventsPerUser
	event, limitErr, err := finalizeSecondaryFlow(context.Background(), db, user, store, maxEvents, secondaryMonth, secondaryDay)
	if err != nil {
		return err
	}
	if limitErr {
		if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("error.limit_reached", user.Language, map[string]any{"max": maxEvents}), nil); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	text := renderSavedText(user, event)
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.SavedKeyboard(user.Language, event.ID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

// msgAddSecondaryDate reads the free-text Gregorian date for a hebrew-primary
// event's secondary track, then finalizes the event with both dates.
func msgAddSecondaryDate(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AddEventSecondaryDate {
		return ext.ContinueGroups
	}

	parsed, err := core.ParseGregorian(ctx.EffectiveMessage.Text, timeNow())
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, err.Error(), nil)
		return sendErr
	}

	db := router.DBFromContext(ctx)
	maxEvents := router.ConfigFromContext(ctx).MaxEventsPerUser
	month, day := parsed.Month, parsed.Day
	event, limitErr, err := finalizeSecondaryFlow(context.Background(), db, user, store, maxEvents, &month, &day)
	if err != nil {
		return err
	}
	if limitErr {
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.limit_reached", user.Language, map[string]any{"max": maxEvents}), nil)
		return sendErr
	}
	text := renderSavedText(user, event)
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.SavedKeyboard(user.Language, event.ID), ParseMode: "HTML"})
	return sendErr
}

// finalizeSecondaryFlow reads the hebrew month/day/year accumulated earlier
// in the flow back out of FSM data and finalizes the event, optionally with
// a secondary gregorian date.
func finalizeSecondaryFlow(ctx context.Context, db repo.DBTX, user *models.User, store *fsm.Store, maxEventsPerUser int, secondaryMonth, secondaryDay *int) (*models.Event, bool, error) {
	data := store.GetData(user.ID)
	month, _ := data["heb_month"].(int)
	day, _ := data["heb_day"].(int)
	var year *int
	if y, ok := data["heb_year"].(int); ok {
		year = &y
	}
	return finalizeEvent(ctx, db, user, store, month, day, year, core.CalendarTypeHebrew, secondaryMonth, secondaryDay, maxEventsPerUser)
}

// finalizeEvent creates the event and clears FSM state. limitErr is true if
// the user's event limit was reached (caller renders the limit-reached
// message) — port of event_add.py's _finalize_event.
func finalizeEvent(ctx context.Context, db repo.DBTX, user *models.User, store *fsm.Store, month, day int, year *int, calendarType string, secondaryMonth, secondaryDay *int, maxEventsPerUser int) (*models.Event, bool, error) {
	data := store.GetData(user.ID)
	firstName, _ := data["first_name"].(string)
	var lastName *string
	if v, ok := data["last_name"].(string); ok && v != "" {
		lastName = &v
	}

	gender, _ := core.DetectGender(firstName)
	var genderPtr *string
	if gender != "" {
		genderPtr = &gender
	}

	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{
		FirstName: firstName, LastName: lastName, Month: month, Day: day, Year: year,
		CalendarType: calendarType, Gender: genderPtr,
		SecondaryMonth: secondaryMonth, SecondaryDay: secondaryDay,
	}, maxEventsPerUser)
	if err != nil {
		if _, ok := err.(*services.LimitErr); ok {
			store.Clear(user.ID)
			return nil, true, nil
		}
		return nil, false, err
	}

	store.Clear(user.ID)
	store.UpdateData(user.ID, map[string]any{"event_id": event.ID})
	return event, false, nil
}

func renderSavedText(user *models.User, event *models.Event) string {
	lang := user.Language
	gender := event.Gender
	kw := map[string]any{}
	if gender != nil {
		kw["gender"] = *gender
	}
	lines := []string{i18n.T("add.saved.title", lang, kw), ""}

	name := core.Esc(event.FirstName)
	if event.LastName != nil && *event.LastName != "" {
		name += " " + core.Esc(*event.LastName)
	}
	lines = append(lines, "🎂 <b>"+name+"</b>")

	next := *event.NextOccurrence
	if event.CalendarType == core.CalendarTypeHebrew {
		hebYear := 0
		if event.Year != nil {
			hebYear = *event.Year
		} else {
			hebYear, _, _ = core.ToHebrew(next)
		}
		hebStr := core.FormatHebrewDate(hebYear, event.Month, event.Day, event.Year != nil)
		lines = append(lines, "📅 "+core.FormatDate(next, user.DateFormat)+"  ("+hebStr+")")
	} else {
		lines = append(lines, "📅 "+core.FormatDate(next, user.DateFormat))
	}

	if event.SecondaryMonth != nil && event.SecondaryNextOccurrence != nil {
		secNext := *event.SecondaryNextOccurrence
		secCountdown := core.FormatCountdown(core.DaysUntil(secNext, services.UserToday(user)))
		lines = append(lines, i18n.T("card.secondary_date", lang, mergeKwargs(map[string]any{
			"date": core.FormatDate(secNext, user.DateFormat), "countdown": secCountdown,
		}, kw)))
	}

	countdown := core.FormatCountdown(core.DaysUntil(next, services.UserToday(user)))
	lines = append(lines, i18n.T("card.days_until", lang, mergeKwargs(map[string]any{"countdown": countdown}, kw)))

	if age, ok := core.AgeAt(event.CalendarType, event.Year, next); ok {
		lines = append(lines, i18n.T("card.age", lang, mergeKwargs(map[string]any{"age": age}, kw)))
	}

	lines = append(lines, "")
	lines = append(lines, i18n.T("add.saved.reminder", lang, mergeKwargs(map[string]any{"reminder": services.DescribeDefaultReminder(user)}, kw)))

	return strings.Join(lines, "\n")
}

func timeNow() time.Time { return time.Now() }
