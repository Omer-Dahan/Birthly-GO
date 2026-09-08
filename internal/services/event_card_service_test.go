package services

import (
	"context"
	"strings"
	"testing"

	"birthly/internal/core"
)

// TestRenderCardText_HebrewEquivalentUsesRealBirthDate reproduces a real
// customer report: a Gregorian event born on 04/11/2002 anchors to Hebrew
// 29 Cheshvan (כ״ט בחשוון), not the 24 Cheshvan (כ״ד בחשוון) you get by
// reconverting this year's Gregorian occurrence instead of the birth date.
func TestRenderCardText_HebrewEquivalentUsesRealBirthDate(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	user := mustUser(t, ctx, db, 3001)
	user.ShowHebrewDate = true

	year := 2002
	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{
		FirstName: "Dana", Month: 11, Day: 4, Year: &year,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text := RenderCardText(user, event, nil)

	if !strings.Contains(text, "כ״ט בחשוון") {
		t.Errorf("card should show the real birth-anchored Hebrew date (כ״ט בחשוון), got:\n%s", text)
	}
	if strings.Contains(text, "כ״ד בחשוון") {
		t.Errorf("card should not show the reconverted-occurrence Hebrew date (כ״ד בחשוון), got:\n%s", text)
	}
	// The Hebrew line must stand on its own, not be "·"-joined onto the
	// Gregorian date line as if the two coincide on the same day.
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "04/11") && strings.Contains(line, "בחשוון") {
			t.Errorf("Hebrew equivalent must not be joined onto the Gregorian date line, got line: %q", line)
		}
	}
}

// TestRenderCardText_HebrewEquivalentFallsBackWithoutBirthYear covers an
// event with no known birth year: there's no anchor to convert from, so the
// card falls back to approximating from the upcoming occurrence, same as
// before this fix.
func TestRenderCardText_HebrewEquivalentFallsBackWithoutBirthYear(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	user := mustUser(t, ctx, db, 3002)
	user.ShowHebrewDate = true

	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{
		FirstName: "Yossi", Month: 11, Day: 4,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text := RenderCardText(user, event, nil)

	hebYear, hebMonth, hebDay := core.ToHebrew(*event.NextOccurrence)
	want := core.FormatHebrewDate(hebYear, hebMonth, hebDay, true)
	if !strings.Contains(text, want) {
		t.Errorf("expected fallback Hebrew date %q in card, got:\n%s", want, text)
	}
}

// TestRenderCardText_NoHebrewLineWhenPreferenceOff confirms a plain
// Gregorian event with ShowHebrewDate off gets no Hebrew line at all.
func TestRenderCardText_NoHebrewLineWhenPreferenceOff(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	user := mustUser(t, ctx, db, 3003)
	user.ShowHebrewDate = false

	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{
		FirstName: "Noa", Month: 6, Day: 20,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text := RenderCardText(user, event, nil)
	if strings.Contains(text, "בלוח העברי") {
		t.Errorf("card should not show a Hebrew equivalent when the preference is off, got:\n%s", text)
	}
}

// TestRenderCardText_DualDateEventShowsRealStoredDates covers a genuine
// dual-date event (hebrew primary + gregorian secondary): the card must
// show both real stored dates, not any converted "equivalent".
func TestRenderCardText_DualDateEventShowsRealStoredDates(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	user := mustUser(t, ctx, db, 3004)

	secMonth, secDay := 11, 4
	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{
		FirstName: "Tal", Month: 8, Day: 29, CalendarType: core.CalendarTypeHebrew,
		SecondaryMonth: &secMonth, SecondaryDay: &secDay,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text := RenderCardText(user, event, nil)

	if !strings.Contains(text, "כ״ט בחשוון") {
		t.Errorf("card should show the real stored Hebrew date (כ״ט בחשוון), got:\n%s", text)
	}
	if !strings.Contains(text, "גם בלוח הלועזי") {
		t.Errorf("card should show the real stored secondary Gregorian date, got:\n%s", text)
	}
	if strings.Contains(text, "בלוח העברי") {
		t.Errorf("dual-date card must not show a converted Hebrew equivalent line, got:\n%s", text)
	}
}
