package services

import (
	"strings"
	"time"

	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/store/models"
)

var categoryKeys = map[string]string{
	core.CategoryFamily:  "category.family",
	core.CategoryFriends: "category.friends",
	core.CategoryWork:    "category.work",
	core.CategoryClients: "category.clients",
	core.CategorySchool:  "category.school",
	core.CategoryOther:   "category.other",
}

var eventTypeKeys = map[string]string{
	core.EventTypeBirthday:    "event_type.birthday",
	core.EventTypeAnniversary: "event_type.anniversary",
	core.EventTypeWedding:     "event_type.wedding",
	core.EventTypeMemorial:    "event_type.memorial",
	core.EventTypeCustom:      "event_type.custom",
}

func categoryKey(category string) string {
	if k, ok := categoryKeys[category]; ok {
		return k
	}
	return category
}

func eventTypeKey(eventType string) string {
	if k, ok := eventTypeKeys[eventType]; ok {
		return k
	}
	return eventType
}

// genderKwargs builds a kwargs map with "gender" set only when gender is
// non-nil — i18n.T's gendered-key lookup expects a plain string, and a
// *string stored directly would never match its type assertion.
func genderKwargs(gender *string, extra map[string]any) map[string]any {
	kwargs := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		kwargs[k] = v
	}
	if gender != nil {
		kwargs["gender"] = *gender
	}
	return kwargs
}

// hebrewEquivalentDate returns the Hebrew calendar date to show alongside a
// Gregorian-primary event when the user has ShowHebrewDate on. When the
// birth year is known, it anchors to the actual birth date (the real Hebrew
// day the person was born on) and finds that day's own next occurrence in
// the Hebrew calendar, so the result stays correct year over year even as
// the two calendars drift apart. Without a birth year there's no anchor to
// convert, so it falls back to converting the upcoming Gregorian occurrence
// itself, same as before.
func hebrewEquivalentDate(event *models.Event, user *models.User, next, today time.Time) string {
	if event.Year != nil {
		birth := time.Date(*event.Year, time.Month(event.Month), event.Day, 0, 0, 0, 0, time.UTC)
		_, anchorMonth, anchorDay := core.ToHebrew(birth)
		if occ, err := core.NextOccurrence(core.CalendarTypeHebrew, anchorMonth, anchorDay, today, user.AdarPolicy, user.Feb29Policy); err == nil {
			hebYear, hebMonth, hebDay := core.ToHebrew(occ)
			return core.FormatHebrewDate(hebYear, hebMonth, hebDay, true)
		}
	}
	hebYear, hebMonth, hebDay := core.ToHebrew(next)
	return core.FormatHebrewDate(hebYear, hebMonth, hebDay, true)
}

// RenderCardText renders the S8 event card body. Empty fields are omitted
// entirely. event.NextOccurrence must be non-nil (guaranteed by callers that
// only render cards for events with a computed occurrence).
func RenderCardText(user *models.User, event *models.Event, rules []*models.ReminderRule) string {
	lang := user.Language

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	header := "🎂 <b>" + name + "</b>"
	if event.Nickname != nil && *event.Nickname != "" {
		header += "  <i>(" + core.Esc(*event.Nickname) + ")</i>"
	}

	lines := []string{header, ""}

	today := UserToday(user)
	next := *event.NextOccurrence
	switch {
	case event.CalendarType == core.CalendarTypeHebrew:
		hebYear := 0
		if event.Year != nil {
			hebYear = *event.Year
		} else {
			hebYear, _, _ = core.ToHebrew(next)
		}
		hebStr := core.FormatHebrewDate(hebYear, event.Month, event.Day, event.Year != nil)
		lines = append(lines, "📅 "+core.FormatDate(next, user.DateFormat)+"  ·  "+hebStr)
	case user.ShowHebrewDate:
		// Gregorian-calendar event, but the user opted into seeing the
		// Hebrew equivalent too. Shown on its own line rather than joined to
		// the Gregorian date with "·": the two calendars drift against each
		// other, so the Hebrew date usually falls a few days off from the
		// Gregorian one shown next to it, and joining them implied a
		// same-day correspondence that isn't real.
		lines = append(lines, "📅 "+core.FormatDate(next, user.DateFormat))
		lines = append(lines, i18n.T("card.hebrew_equivalent", lang, map[string]any{"date": hebrewEquivalentDate(event, user, next, today)}))
	default:
		lines = append(lines, "📅 "+core.FormatDate(next, user.DateFormat))
	}

	if event.SecondaryMonth != nil && event.SecondaryNextOccurrence != nil {
		secNext := *event.SecondaryNextOccurrence
		lines = append(lines, i18n.T("card.secondary_date", lang, genderKwargs(event.Gender, map[string]any{
			"date": core.FormatDate(secNext, user.DateFormat),
		})))
	}

	countdown := core.FormatCountdown(core.DaysUntil(next, today))
	lines = append(lines, i18n.T("card.days_until", lang, genderKwargs(event.Gender, map[string]any{"countdown": countdown})))

	if age, ok := core.AgeAt(event.CalendarType, event.Year, next); ok {
		ageKey := "card.age"
		if event.EventType == core.EventTypeMemorial {
			ageKey = "card.age_memorial"
		}
		lines = append(lines, i18n.T(ageKey, lang, genderKwargs(event.Gender, map[string]any{"age": age})))
	}

	var tagParts []string
	if event.Category != "" && event.Category != core.CategoryOther {
		tagParts = append(tagParts, i18n.T(categoryKey(event.Category), lang, nil))
	}
	if event.Relation != nil && *event.Relation != "" {
		tagParts = append(tagParts, core.Esc(*event.Relation))
	}
	if len(tagParts) > 0 {
		lines = append(lines, "🏷 "+strings.Join(tagParts, " · "))
	}

	if event.Phone != nil && *event.Phone != "" {
		lines = append(lines, "📞 "+core.FormatPhone(*event.Phone))
	}
	if event.TelegramUsername != nil && *event.TelegramUsername != "" {
		lines = append(lines, "💬 @"+core.Esc(*event.TelegramUsername))
	}
	if event.Notes != nil && *event.Notes != "" {
		lines = append(lines, "📝 "+core.Esc(*event.Notes))
	}

	remindersSummary := SummarizeRules(rules, lang)
	if remindersSummary != "" {
		lines = append(lines, "")
		lines = append(lines, i18n.T("card.reminders", lang, map[string]any{"rules": remindersSummary}))
	}

	return strings.Join(lines, "\n")
}

func EventTypeLabel(eventType, lang string) string {
	return i18n.T(eventTypeKey(eventType), lang, nil)
}

func CategoryLabel(category, lang string) string {
	return i18n.T(categoryKey(category), lang, nil)
}
