package services

import (
	"strings"
	"testing"

	"birthly/internal/core"
	"birthly/internal/store/models"
)

func rendererFixtures() (*models.User, *models.Event, *models.ReminderRule) {
	user := &models.User{Language: core.LanguageHe}
	event := &models.Event{
		EventType: core.EventTypeBirthday, FirstName: "Dana", Category: core.CategoryOther,
	}
	offset := 0
	rule := &models.ReminderRule{OffsetDays: &offset}
	return user, event, rule
}

func TestRenderReminder_BirthdayToday(t *testing.T) {
	user, event, rule := rendererFixtures()
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "🎂") {
		t.Errorf("birthday-today reminder missing 🎂: %q", text)
	}
	if !strings.Contains(text, "היום") {
		t.Errorf("birthday-today reminder missing 'היום': %q", text)
	}
}

func TestRenderReminder_BirthdayTomorrow(t *testing.T) {
	user, event, rule := rendererFixtures()
	offset := 1
	rule.OffsetDays = &offset
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "מחר") {
		t.Errorf("birthday-tomorrow reminder missing 'מחר': %q", text)
	}
}

func TestRenderReminder_AgeIncludedWhenYearKnown(t *testing.T) {
	user, event, rule := rendererFixtures()
	year := 1990
	event.Year = &year
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "36") {
		t.Errorf("reminder with known birth year missing computed age (36): %q", text)
	}
}

func TestRenderReminder_NoAgeLineWhenYearUnknown(t *testing.T) {
	user, event, rule := rendererFixtures()
	text := RenderReminder(user, event, rule, 2026)
	if strings.Contains(text, "🎈") {
		t.Errorf("reminder with unknown birth year should have no age line: %q", text)
	}
}

func TestRenderReminder_FemaleGenderPhrase(t *testing.T) {
	user, event, rule := rendererFixtures()
	year := 1990
	event.Year = &year
	gender := core.GenderFemale
	event.Gender = &gender
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "היא חוגגת") {
		t.Errorf("female-gender reminder missing 'היא חוגגת': %q", text)
	}
}

func TestRenderReminder_MaleGenderPhrase(t *testing.T) {
	user, event, rule := rendererFixtures()
	year := 1990
	event.Year = &year
	gender := core.GenderMale
	event.Gender = &gender
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "הוא חוגג") {
		t.Errorf("male-gender reminder missing 'הוא חוגג': %q", text)
	}
}

func TestRenderReminder_MemorialUsesCandleNotCake(t *testing.T) {
	user, event, rule := rendererFixtures()
	event.EventType = core.EventTypeMemorial
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, "🕯") {
		t.Errorf("memorial reminder missing 🕯: %q", text)
	}
	if strings.Contains(text, "🎂") {
		t.Errorf("memorial reminder should not use the birthday cake emoji: %q", text)
	}
}

func TestRenderReminder_RelationAndPhoneAppendix(t *testing.T) {
	user, event, rule := rendererFixtures()
	relation := "אמא"
	phone := "050-1234567"
	event.Relation = &relation
	event.Phone = &phone
	text := RenderReminder(user, event, rule, 2026)
	if !strings.Contains(text, relation) {
		t.Errorf("reminder missing relation line: %q", text)
	}
	if !strings.Contains(text, phone) {
		t.Errorf("reminder missing phone line: %q", text)
	}
}
