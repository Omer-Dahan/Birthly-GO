package handlers

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	"birthly/internal/bot/router"
	"birthly/internal/config"
	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
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

// TestExactGregorianBirthDate_MatchesKnownHebrewDate pins the same
// hand-checked conversion as internal/core's golden test (20 Av 5781 -> 29
// July 2021) through the handler-level helper that wires ResolveMonth +
// ToGregorian together for the auto-confirm feature.
func TestExactGregorianBirthDate_MatchesKnownHebrewDate(t *testing.T) {
	got, err := exactGregorianBirthDate(5781, 5, 20, core.AdarPolicyAdarII)
	if err != nil {
		t.Fatalf("exactGregorianBirthDate: %v", err)
	}
	want := time.Date(2021, time.July, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("exactGregorianBirthDate(5781, Av, 20) = %v, want %v", got, want)
	}
}

// TestExactGregorianBirthDate_ResolvesAdarAcrossLeapStatus covers the case
// the auto-confirm feature must not blow up on: the user picked an Adar
// variant (12 or 13) while the day-keyboard was built against the *current*
// hebrew year's leap status, which can differ from the birth year's. 5781
// is not a leap year, so both Adar I (12) and Adar II (13) must resolve to
// its single Adar (12) rather than erroring.
func TestExactGregorianBirthDate_ResolvesAdarAcrossLeapStatus(t *testing.T) {
	for _, srcMonth := range []int{12, 13} {
		got, err := exactGregorianBirthDate(5781, srcMonth, 10, core.AdarPolicyAdarII)
		if err != nil {
			t.Fatalf("exactGregorianBirthDate(5781, %d, 10): %v", srcMonth, err)
		}
		wantYear, wantMonth, wantDay := core.ToHebrew(got)
		if wantYear != 5781 || wantMonth != 12 || wantDay != 10 {
			t.Errorf("exactGregorianBirthDate(5781, %d, 10) round-trips to (%d,%d,%d), want (5781,12,10)", srcMonth, wantYear, wantMonth, wantDay)
		}
	}
}

// newAddFlowCallbackContext builds an ext.Context for driving a callback
// handler in the add-event flow directly, without a live dispatcher.
func newAddFlowCallbackContext(user *models.User, db *sql.DB, fsmStore *fsm.Store) (*ext.Context, *gotgbot.CallbackQuery) {
	cq := &gotgbot.CallbackQuery{
		Id:      "cq1",
		Message: gotgbot.Message{MessageId: 42, Chat: gotgbot.Chat{Id: user.ID}},
	}
	gctx := &ext.Context{
		Update: &gotgbot.Update{CallbackQuery: cq},
		Data: map[string]any{
			router.DataKeyUser: user, router.DataKeyDB: db, router.DataKeyFSM: fsmStore,
			router.DataKeyConfig: &config.Config{MaxEventsPerUser: 1000},
		},
		EffectiveUser: &gotgbot.User{Id: user.ID},
		EffectiveChat: &gotgbot.Chat{Id: user.ID},
	}
	return gctx, cq
}

// TestCbAddSecondaryAutoYes_FinalizesWithComputedSecondary covers the "yes"
// branch of the auto-confirm prompt: the gregorian date computed earlier in
// msgAddHebrewYear (stashed as auto_sec_month/auto_sec_day) is saved as the
// event's secondary date without the user re-typing it.
func TestCbAddSecondaryAutoYes_FinalizesWithComputedSecondary(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2001, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.AddEventSecondaryAutoConfirm)
	fsmStore.UpdateData(user.ID, map[string]any{
		"first_name": "Dana", "heb_month": 5, "heb_day": 20, "heb_year": 5781,
		"auto_sec_month": 7, "auto_sec_day": 29,
	})

	gctx, _ := newAddFlowCallbackContext(user, db, fsmStore)
	bot := testEditBot(&fakeEditClient{})

	if err := cbAddSecondaryAutoYes(bot, gctx); err != nil {
		t.Fatalf("cbAddSecondaryAutoYes: %v", err)
	}

	if _, ok := fsmStore.GetState(user.ID); ok {
		t.Error("expected FSM state cleared after finalizing")
	}

	events, err := repo.NewEventRepo(db, user.ID).ListNotDeleted(ctx)
	if err != nil {
		t.Fatalf("ListNotDeleted: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	event := events[0]
	if event.SecondaryMonth == nil || *event.SecondaryMonth != 7 || event.SecondaryDay == nil || *event.SecondaryDay != 29 {
		t.Errorf("secondary date = (%v, %v), want (7, 29)", event.SecondaryMonth, event.SecondaryDay)
	}
}

// TestCbAddSecondaryAutoManual_SwitchesToManualEntry covers the "I'll type
// it in myself" override: it must not finalize the event, only move the FSM
// into the existing free-text secondary-date state.
func TestCbAddSecondaryAutoManual_SwitchesToManualEntry(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2002, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.AddEventSecondaryAutoConfirm)
	fsmStore.UpdateData(user.ID, map[string]any{
		"first_name": "Dana", "heb_month": 5, "heb_day": 20, "heb_year": 5781,
		"auto_sec_month": 7, "auto_sec_day": 29,
	})

	gctx, _ := newAddFlowCallbackContext(user, db, fsmStore)
	bot := testEditBot(&fakeEditClient{})

	if err := cbAddSecondaryAutoManual(bot, gctx); err != nil {
		t.Fatalf("cbAddSecondaryAutoManual: %v", err)
	}

	state, ok := fsmStore.GetState(user.ID)
	if !ok || state != fsm.AddEventSecondaryDate {
		t.Errorf("state = %q, ok=%v, want AddEventSecondaryDate", state, ok)
	}

	events, err := repo.NewEventRepo(db, user.ID).ListNotDeleted(ctx)
	if err != nil {
		t.Fatalf("ListNotDeleted: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected no event created yet, got %d", len(events))
	}
}

// TestCbAddSecondaryAutoNo_FinalizesWithoutSecondary covers declining the
// computed date entirely: the event is saved with only its hebrew primary
// date, same as the manual secondary-prompt's "no thanks".
func TestCbAddSecondaryAutoNo_FinalizesWithoutSecondary(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2003, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.AddEventSecondaryAutoConfirm)
	fsmStore.UpdateData(user.ID, map[string]any{
		"first_name": "Dana", "heb_month": 5, "heb_day": 20, "heb_year": 5781,
		"auto_sec_month": 7, "auto_sec_day": 29,
	})

	gctx, _ := newAddFlowCallbackContext(user, db, fsmStore)
	bot := testEditBot(&fakeEditClient{})

	if err := cbAddSecondaryAutoNo(bot, gctx); err != nil {
		t.Fatalf("cbAddSecondaryAutoNo: %v", err)
	}

	events, err := repo.NewEventRepo(db, user.ID).ListNotDeleted(ctx)
	if err != nil {
		t.Fatalf("ListNotDeleted: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].SecondaryMonth != nil {
		t.Errorf("expected no secondary date, got month=%v", events[0].SecondaryMonth)
	}
}

// TestCbAddNoYear_HebYearBranchUnaffected documents that the pre-existing
// "I don't know the hebrew year" branch (no full date, so no auto-compute is
// possible) still goes straight to the manual secondary-date prompt.
func TestCbAddNoYear_HebYearBranchUnaffected(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2004, nil, "Dana", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.AddEventHebYear)
	fsmStore.UpdateData(user.ID, map[string]any{"first_name": "Dana", "heb_month": 5, "heb_day": 20})

	gctx, _ := newAddFlowCallbackContext(user, db, fsmStore)
	bot := testEditBot(&fakeEditClient{})

	if err := cbAddNoYear(bot, gctx); err != nil {
		t.Fatalf("cbAddNoYear: %v", err)
	}

	state, ok := fsmStore.GetState(user.ID)
	if !ok || state != fsm.AddEventSecondaryPrompt {
		t.Errorf("state = %q, ok=%v, want AddEventSecondaryPrompt", state, ok)
	}
}
