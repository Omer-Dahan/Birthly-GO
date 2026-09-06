package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"birthly/internal/bot/fsm"
	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
)

func TestFinalizeEvent_CreatesEventAndClearsState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1001, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(0, 200)
	fsmStore.SetState(user.ID, fsm.AddEventName)
	fsmStore.UpdateData(user.ID, map[string]any{"first_name": "Dana", "last_name": "Cohen"})

	event, limitErr, err := finalizeEvent(ctx, db, user, fsmStore, 3, 15, nil, "gregorian", nil, nil, 1000)
	if err != nil {
		t.Fatalf("finalizeEvent: %v", err)
	}
	if limitErr {
		t.Fatal("unexpected limitErr=true")
	}
	if event.FirstName != "Dana" || event.LastName == nil || *event.LastName != "Cohen" {
		t.Errorf("unexpected event: %+v", event)
	}
	if event.NextOccurrence == nil {
		t.Error("expected NextOccurrence to be set")
	}

	if _, ok := fsmStore.GetState(user.ID); ok {
		t.Error("expected FSM state cleared after finalizeEvent")
	}
	// event_id is stashed back into FSM data for the caller's own use.
	data := fsmStore.GetData(user.ID)
	if data["event_id"] != event.ID {
		t.Errorf("event_id not stashed in FSM data: %v", data)
	}
}

func TestFinalizeEvent_LimitReached(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1002, nil, "X", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(0, 200)
	fsmStore.SetState(user.ID, fsm.AddEventDate)
	fsmStore.UpdateData(user.ID, map[string]any{"first_name": "First"})
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Existing", Month: 1, Day: 1}, 1); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	_, limitErr, err := finalizeEvent(ctx, db, user, fsmStore, 2, 2, nil, "gregorian", nil, nil, 1)
	if err != nil {
		t.Fatalf("finalizeEvent: %v", err)
	}
	if !limitErr {
		t.Error("expected limitErr=true when at the 1-event cap")
	}
	if _, ok := fsmStore.GetState(user.ID); ok {
		t.Error("expected FSM state cleared even on limit-reached")
	}
}

func TestRenderSavedText_IncludesNameDateAndReminder(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1003, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	year := 1990
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15, Year: &year}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text := renderSavedText(user, event)
	if !strings.Contains(text, "Dana") {
		t.Errorf("renderSavedText missing name: %q", text)
	}
	if !strings.Contains(text, "🎂") {
		t.Errorf("renderSavedText missing birthday emoji: %q", text)
	}
}

// TestFinalizeSecondaryFlow_HebrewPrimaryWithGregorianSecondary covers the
// one-directional dual-date offer: a hebrew-primary event can gain a second,
// gregorian-calendar recurring date, and both occurrences get computed.
func TestFinalizeSecondaryFlow_HebrewPrimaryWithGregorianSecondary(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1004, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(0, 200)
	fsmStore.SetState(user.ID, fsm.AddEventHebYear)
	fsmStore.UpdateData(user.ID, map[string]any{
		"first_name": "Dana", "heb_month": 7, "heb_day": 1, "heb_year": 5750,
	})

	secMonth, secDay := 3, 15
	event, limitErr, err := finalizeSecondaryFlow(ctx, db, user, fsmStore, 1000, &secMonth, &secDay)
	if err != nil {
		t.Fatalf("finalizeSecondaryFlow: %v", err)
	}
	if limitErr {
		t.Fatal("unexpected limitErr=true")
	}
	if event.CalendarType != core.CalendarTypeHebrew {
		t.Errorf("CalendarType = %q, want hebrew", event.CalendarType)
	}
	if event.SecondaryMonth == nil || *event.SecondaryMonth != 3 || event.SecondaryDay == nil || *event.SecondaryDay != 15 {
		t.Errorf("secondary date not stored: month=%v day=%v", event.SecondaryMonth, event.SecondaryDay)
	}
	if event.SecondaryCalendarType == nil || *event.SecondaryCalendarType != core.CalendarTypeGregorian {
		t.Errorf("SecondaryCalendarType = %v, want gregorian", event.SecondaryCalendarType)
	}
	if event.SecondaryNextOccurrence == nil {
		t.Error("expected SecondaryNextOccurrence to be computed")
	}
	if event.NextOccurrence == nil {
		t.Error("expected primary NextOccurrence to be computed")
	}

	text := renderSavedText(user, event)
	if !strings.Contains(text, "🗓") {
		t.Errorf("renderSavedText missing secondary-date line for a dual-date event: %q", text)
	}
}

// TestFinalizeSecondaryFlow_NoSecondaryLeavesFieldsNil covers "no thanks":
// a hebrew-primary event with no secondary date has no secondary fields set.
func TestFinalizeSecondaryFlow_NoSecondaryLeavesFieldsNil(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1005, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(0, 200)
	fsmStore.SetState(user.ID, fsm.AddEventSecondaryPrompt)
	fsmStore.UpdateData(user.ID, map[string]any{
		"first_name": "Dana", "heb_month": 7, "heb_day": 1,
	})

	event, limitErr, err := finalizeSecondaryFlow(ctx, db, user, fsmStore, 1000, nil, nil)
	if err != nil {
		t.Fatalf("finalizeSecondaryFlow: %v", err)
	}
	if limitErr {
		t.Fatal("unexpected limitErr=true")
	}
	if event.SecondaryMonth != nil || event.SecondaryNextOccurrence != nil {
		t.Errorf("expected no secondary date, got month=%v occurrence=%v", event.SecondaryMonth, event.SecondaryNextOccurrence)
	}
}

// TestGregorianPrimary_NeverGetsSecondaryOffer documents the one-directional
// invariant at the service layer: even if a caller mistakenly passes a
// secondary date alongside a gregorian primary, CreateMinimalEvent ignores
// it rather than persisting an inconsistent dual-date event.
func TestGregorianPrimary_NeverGetsSecondaryOffer(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 1006, nil, "Yossi", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	secMonth, secDay := 5, 5
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{
		FirstName: "Yossi", Month: 3, Day: 15, CalendarType: core.CalendarTypeGregorian,
		SecondaryMonth: &secMonth, SecondaryDay: &secDay,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}
	if event.SecondaryMonth != nil || event.SecondaryNextOccurrence != nil {
		t.Errorf("gregorian-primary event should never get a secondary date, got month=%v occurrence=%v", event.SecondaryMonth, event.SecondaryNextOccurrence)
	}
}
