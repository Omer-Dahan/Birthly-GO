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
	"birthly/internal/bot/keyboards"
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
	if err := cbFieldPick(bot, editYearCtx(t, db, user.ID, fsmStore, "ev:field:"+keyboards.FieldPickValue("year", event.ID), "", user)); err != nil {
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
	// 10 Tishrei 5750 fell on 1989-10-09. Store 8 Oct, the civil day
	// before (an after-sunset birth): consistent with the year, yet not what
	// a recomputation from the new year produces, so code that rewrites the
	// stored date from the year is caught.
	sm, sd := 10, 8
	if src, err := exactGregorianBirthDate(5750, 7, 10, user.AdarPolicy); err != nil ||
		(int(src.Month()) == sm && src.Day() == sd) {
		t.Fatalf("fixture must differ from the recomputed date, got %v (%v)", src, err)
	}
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
			fsmStore.UpdateData(user.ID, map[string]any{"event_id": event.ID, "editing_field": "year", "prompt_event_id": event.ID})
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

func TestFieldPick_OldPickerCannotActOnOtherEvent(t *testing.T) {
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
	first, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "A", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	second, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "B", Month: 5, Day: 6}, 1001)
	if err != nil {
		t.Fatal(err)
	}
	// Picker rendered for the first event.
	oldData := keyboards.MoreDetailsKeyboard(user.Language, first.ID, eventFieldValues(first)).InlineKeyboard[0][0].CallbackData
	// Flow then moves to the second event.
	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.EditEventChoosingField)
	fsmStore.UpdateData(user.ID, map[string]any{"event_id": second.ID})
	bot := testEditBot(&fakeEditClient{})

	if err := cbFieldPick(bot, editYearCtx(t, db, user.ID, fsmStore, oldData, "", user)); err != nil {
		t.Fatalf("cbFieldPick: %v", err)
	}
	if state, _ := fsmStore.GetState(user.ID); state != fsm.EditEventChoosingField {
		t.Fatalf("state = %q, stale picker must not advance the flow", state)
	}
	if v := fsmStore.GetData(user.ID)["editing_field"]; v != nil {
		t.Fatalf("editing_field = %v, want unset", v)
	}
	got, err := services.GetOwnedEvent(ctx, db, user, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Month != second.Month || got.Day != second.Day || got.Year != second.Year {
		t.Fatalf("second event changed: %+v -> %+v", second, got)
	}
}

func TestEditFlow_StaleClearButtonCannotWipeOtherEvent(t *testing.T) {
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
	mk := func(name string) *models.Event {
		ev, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: name, Month: 3, Day: 15, Year: intPtr(1990)}, 1000)
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}
	evA, evB := mk("A"), mk("B")

	// The clear button rendered under event A's year prompt.
	kb := keyboards.FieldPromptKeyboard("en", "year", evA.ID, true)
	var staleData string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if strings.HasPrefix(btn.CallbackData, "ev:clear:") {
				staleData = btn.CallbackData
			}
		}
	}
	if staleData == "" {
		t.Fatal("no clear button rendered")
	}

	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.EditEventEnteringValue)
	fsmStore.UpdateData(user.ID, map[string]any{"event_id": evB.ID, "editing_field": "year"})
	bot := testEditBot(&fakeEditClient{})

	for _, data := range []string{staleData, "ev:clear:year"} {
		if err := cbFieldClear(bot, editYearCtx(t, db, user.ID, fsmStore, data, "", user)); err != nil {
			t.Fatalf("cbFieldClear(%q): %v", data, err)
		}
		got, err := services.GetOwnedEvent(ctx, db, user, evB.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Year == nil || *got.Year != 1990 {
			t.Fatalf("stale %q wiped event B's year: %v", data, got.Year)
		}
		if state, _ := fsmStore.GetState(user.ID); state != fsm.EditEventEnteringValue {
			t.Fatalf("state changed to %q", state)
		}
	}
}

func intPtr(v int) *int { return &v }

func TestEditFlow_TextReplyForOldPromptRefusedOnOtherEvent(t *testing.T) {
	if err := i18n.LoadLocales(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 4103, nil, "U", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	evA, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "A", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	evB, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "B", Month: 5, Day: 6}, 1001)
	if err != nil {
		t.Fatal(err)
	}
	fsmStore := fsm.NewStore(time.Hour, 200)
	fsmStore.SetState(user.ID, fsm.EditEventChoosingField)
	fsmStore.UpdateData(user.ID, map[string]any{"event_id": evA.ID})
	client := &fakeEditClient{}
	bot := testEditBot(client)

	// A's year prompt opens, then the flow switches to B without leaving the state.
	if err := cbFieldPick(bot, editYearCtx(t, db, user.ID, fsmStore, "ev:field:"+keyboards.FieldPickValue("year", evA.ID), "", user)); err != nil {
		t.Fatal(err)
	}
	fsmStore.UpdateData(user.ID, map[string]any{"event_id": evB.ID})

	if err := msgFieldValue(bot, editYearCtx(t, db, user.ID, fsmStore, "", "1990", user)); err != nil {
		t.Fatalf("msgFieldValue: %v", err)
	}
	for _, ev := range []*models.Event{evA, evB} {
		got, err := services.GetOwnedEvent(ctx, db, user, ev.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Year != nil {
			t.Fatalf("event %d got year %d from a stale prompt", ev.ID, *got.Year)
		}
	}
	if state, _ := fsmStore.GetState(user.ID); state != fsm.EditEventEnteringValue {
		t.Fatalf("state = %q, want unchanged", state)
	}
	if !client.sentContains(i18n.T("edit.stale_prompt", user.Language, nil)) {
		t.Fatal("user did not get the refusal")
	}
}

// replyCtx is a text reply that answers the message with id replyTo.
func replyCtx(t *testing.T, db repo.DBTX, userID int64, fsmStore *fsm.Store, text string, replyTo int64, user *models.User) *ext.Context {
	t.Helper()
	g := editYearCtx(t, db, userID, fsmStore, "", text, user)
	g.EffectiveMessage.ReplyToMessage = &gotgbot.Message{MessageId: replyTo, Chat: gotgbot.Chat{Id: 555}}
	return g
}

func TestEditFlow_ReplyToFirstPromptDoesNotLandOnSecondEvent(t *testing.T) {
	if err := i18n.LoadLocales(); err != nil {
		t.Fatal(err)
	}
	for _, expire := range []bool{false, true} {
		name := "same flow"
		if expire {
			name = "after flow expiry"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			user, err := services.GetOrCreateUser(ctx, db, 4201, nil, "U", nil, false)
			if err != nil {
				t.Fatal(err)
			}
			evA, _ := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "A", Month: 3, Day: 15}, 1000)
			evB, _ := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "B", Month: 5, Day: 6}, 1001)
			fsmStore := fsm.NewStore(time.Hour, 200)
			client := &fakeEditClient{}
			bot := testEditBot(client)

			open := func(ev *models.Event, msgID int64) {
				fsmStore.SetState(user.ID, fsm.EditEventChoosingField)
				fsmStore.UpdateData(user.ID, map[string]any{"event_id": ev.ID})
				g := editYearCtx(t, db, user.ID, fsmStore, "ev:field:"+keyboards.FieldPickValue("year", ev.ID), "", user)
				g.CallbackQuery.Message = gotgbot.Message{MessageId: msgID, Chat: gotgbot.Chat{Id: 555}}
				if err := cbFieldPick(bot, g); err != nil {
					t.Fatal(err)
				}
			}
			open(evA, 101)
			if expire {
				fsmStore.Clear(user.ID)
			}
			open(evB, 202)

			if err := msgFieldValue(bot, replyCtx(t, db, user.ID, fsmStore, "1990", 101, user)); err != nil {
				t.Fatal(err)
			}
			for _, ev := range []*models.Event{evA, evB} {
				got, err := services.GetOwnedEvent(ctx, db, user, ev.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.Year != nil {
					t.Fatalf("event %d got year %d from a reply to another prompt", ev.ID, *got.Year)
				}
			}
			if !client.sentContains(i18n.T("edit.stale_prompt", user.Language, nil)) {
				t.Fatal("user did not get the refusal")
			}
			// A reply to the current prompt still works.
			if err := msgFieldValue(bot, replyCtx(t, db, user.ID, fsmStore, "1990", 202, user)); err != nil {
				t.Fatal(err)
			}
			got, _ := services.GetOwnedEvent(ctx, db, user, evB.ID)
			if got.Year == nil || *got.Year != 1990 {
				t.Fatalf("reply to the current prompt not applied: %v", got.Year)
			}
		})
	}
}
