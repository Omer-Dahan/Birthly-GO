package handlers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// editYearCtx builds a callback ctx (data = cbData) or, when text != "", a
// text-message ctx for the edit flow.
func editYearCtx(t *testing.T, db repo.DBTX, userID int64, fsmStore *fsm.Store, cbData, text string, user *models.User) *ext.Context {
	t.Helper()
	data := map[string]any{router.DataKeyUser: user, router.DataKeyDB: db, router.DataKeyFSM: fsmStore}
	g := &ext.Context{Data: data, EffectiveUser: &gotgbot.User{Id: userID}, EffectiveChat: &gotgbot.Chat{Id: 555}}
	if text != "" {
		msg := &gotgbot.Message{Chat: gotgbot.Chat{Id: 555}, Text: text}
		g.Update = &gotgbot.Update{Message: msg}
		g.EffectiveMessage = msg
		return g
	}
	cq := &gotgbot.CallbackQuery{Id: "cq", Data: cbData, Message: gotgbot.Message{MessageId: 42, Chat: gotgbot.Chat{Id: 555}}}
	g.Update = &gotgbot.Update{CallbackQuery: cq}
	return g
}

func TestEditFlow_AddYearToYearlessEvent(t *testing.T) {
	if err := i18n.LoadLocales(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 4101, nil, "U", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if event.Year != nil {
		t.Fatal("precondition: event must have no year")
	}
	before, _, err := RenderCard(ctx, db, user, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(before, "<b>3") || strings.Contains(before, "🎈") {
		t.Fatalf("card without a year should show no age line:\n%s", before)
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.EditEventChoosingField)
	fsmStore.UpdateData(user.ID, map[string]any{"event_id": event.ID})
	bot := testEditBot(&fakeEditClient{})

	// Real callback path: the "year" button on the field picker.
	if err := cbFieldPick(bot, editYearCtx(t, db, user.ID, fsmStore, "ev:field:year", "", user)); err != nil {
		t.Fatalf("cbFieldPick: %v", err)
	}
	if state, _ := fsmStore.GetState(user.ID); state != fsm.EditEventEnteringValue {
		t.Fatalf("state = %q, want entering_value", state)
	}
	if f, _ := fsmStore.GetData(user.ID)["editing_field"].(string); f != "year" {
		t.Fatalf("editing_field = %q, want year", f)
	}

	// Invalid input keeps the state and the event untouched.
	for _, bad := range []string{"abc", "1850", fmt.Sprint(time.Now().Year() + 1)} {
		if err := msgFieldValue(bot, editYearCtx(t, db, user.ID, fsmStore, "", bad, user)); err != nil {
			t.Fatalf("msgFieldValue(%q): %v", bad, err)
		}
		if ev, _ := services.GetOwnedEvent(ctx, db, user, event.ID); ev.Year != nil {
			t.Fatalf("year %q must be rejected, got %v", bad, *ev.Year)
		}
	}

	if err := msgFieldValue(bot, editYearCtx(t, db, user.ID, fsmStore, "", "1990", user)); err != nil {
		t.Fatalf("msgFieldValue: %v", err)
	}
	got, err := services.GetOwnedEvent(ctx, db, user, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Year == nil || *got.Year != 1990 {
		t.Fatalf("Year = %v, want 1990", got.Year)
	}

	after, _, err := RenderCard(ctx, db, user, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantAge := got.NextOccurrence.Year() - 1990
	if !strings.Contains(after, fmt.Sprintf("<b>%d</b>", wantAge)) || !strings.Contains(after, "🎈") {
		t.Errorf("card should show age %d after adding the year:\n%s", wantAge, after)
	}
}

func TestEditFlow_HebrewYearKeepsStoredGregorianDate(t *testing.T) {
	if err := i18n.LoadLocales(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 4102, nil, "U", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// 5750 / 10 Tishrei fell on 1989-10-09; store a deliberately different
	// month/day (the user's own recorded date) as the anchor.
	src, err := exactGregorianBirthDate(5750, 7, 10, user.AdarPolicy)
	if err != nil {
		t.Fatal(err)
	}
	sm, sd := int(src.Month()), src.Day()
	for _, tc := range []struct {
		name, input string
		wantYear    int
	}{{"hebrew input", "5750", 5750}, {"gregorian input", "1989", 5750}} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{
				FirstName: "Noa", Month: 7, Day: 10, CalendarType: core.CalendarTypeHebrew,
				SecondaryMonth: &sm, SecondaryDay: &sd,
			}, 1000)
			if err != nil {
				t.Fatal(err)
			}
			fsmStore := fsm.NewStore(time.Hour, 200)
			fsmStore.SetState(user.ID, fsm.EditEventEnteringValue)
			fsmStore.UpdateData(user.ID, map[string]any{"event_id": event.ID, "editing_field": "year"})
			bot := testEditBot(&fakeEditClient{})
			if err := msgFieldValue(bot, editYearCtx(t, db, user.ID, fsmStore, "", tc.input, user)); err != nil {
				t.Fatalf("msgFieldValue: %v", err)
			}
			got, err := services.GetOwnedEvent(ctx, db, user, event.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Year == nil || *got.Year != tc.wantYear {
				t.Fatalf("Year = %v, want %d", got.Year, tc.wantYear)
			}
			if got.SecondaryMonth == nil || got.SecondaryDay == nil ||
				*got.SecondaryMonth != sm || *got.SecondaryDay != sd {
				t.Errorf("secondary = %v/%v, want %d/%d untouched", got.SecondaryDay, got.SecondaryMonth, sd, sm)
			}
			if d, err := exactGregorianBirthDate(*got.Year, 7, 10, user.AdarPolicy); err != nil ||
				int(d.Month()) != sm || d.Day() != sd {
				t.Errorf("source date moved: year %d resolves to %v, stored %d/%d", *got.Year, d, sd, sm)
			}
		})
	}
}

func TestParseEditYear(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	greg := func(m, d int) *testEvent { return &testEvent{cal: core.CalendarTypeGregorian, m: m, d: d} }
	cases := []struct {
		name string
		ev   *testEvent
		in   string
		ok   bool
	}{
		{"valid", greg(3, 15), "1990", true},
		{"future", greg(3, 15), "2027", false},
		{"too old", greg(3, 15), "1800", false},
		{"text", greg(3, 15), "19x0", false},
		{"feb29 non-leap", greg(2, 29), "1990", false},
		{"feb29 leap", greg(2, 29), "1992", true},
		{"hebrew year", &testEvent{cal: core.CalendarTypeHebrew, m: 7, d: 10}, "5750", true},
		{"hebrew from gregorian", &testEvent{cal: core.CalendarTypeHebrew, m: 7, d: 10}, "1990", true},
		{"hebrew future", &testEvent{cal: core.CalendarTypeHebrew, m: 7, d: 10}, "5860", false},
	}
	for _, c := range cases {
		_, err := parseEditYear(c.ev.model(), c.in, now, core.AdarPolicyAdarII)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

type testEvent struct {
	cal  string
	m, d int
}

func (e *testEvent) model() *models.Event {
	return &models.Event{CalendarType: e.cal, Month: e.m, Day: e.d}
}
