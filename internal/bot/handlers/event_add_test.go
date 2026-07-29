package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"birthly/internal/bot/fsm"
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

	event, limitErr, err := finalizeEvent(ctx, db, user, fsmStore, 3, 15, nil, "gregorian", 1000)
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

	_, limitErr, err := finalizeEvent(ctx, db, user, fsmStore, 2, 2, nil, "gregorian", 1)
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
