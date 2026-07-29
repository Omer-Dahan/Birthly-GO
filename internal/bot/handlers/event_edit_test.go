package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
)

func TestSetEventField_AllFields(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 4001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	relation := "friend"
	setEventField(event, "relation", &relation)
	if event.Relation == nil || *event.Relation != "friend" {
		t.Errorf("relation not set: %+v", event.Relation)
	}

	setEventField(event, "relation", nil)
	if event.Relation != nil {
		t.Errorf("expected relation cleared, got %v", *event.Relation)
	}

	phone := "0501234567"
	setEventField(event, "phone", &phone)
	if event.Phone == nil || *event.Phone != phone {
		t.Errorf("phone not set: %+v", event.Phone)
	}

	category := core.CategoryWork
	setEventField(event, "category", &category)
	if event.Category != core.CategoryWork {
		t.Errorf("category not set: %v", event.Category)
	}

	// category with nil value must NOT clear the string field (it's non-nullable)
	setEventField(event, "category", nil)
	if event.Category != core.CategoryWork {
		t.Errorf("category should be unchanged on nil value, got %v", event.Category)
	}

	photoID := "AgADabc"
	setEventField(event, "photo", &photoID)
	if event.PhotoFileID == nil || *event.PhotoFileID != photoID {
		t.Errorf("photo not mapped to PhotoFileID: %+v", event.PhotoFileID)
	}
}

func TestApplyField_PersistsThroughUpdateEvent(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 4002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	notes := "Loves chocolate cake"
	setEventField(event, "notes", &notes)
	updated, err := services.UpdateEvent(ctx, db, user, event)
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if updated.Notes == nil || *updated.Notes != notes {
		t.Errorf("notes not persisted: %+v", updated.Notes)
	}

	reloaded, err := services.GetOwnedEvent(ctx, db, user, event.ID)
	if err != nil {
		t.Fatalf("GetOwnedEvent: %v", err)
	}
	if reloaded.Notes == nil || *reloaded.Notes != notes {
		t.Errorf("notes not persisted across reload: %+v", reloaded.Notes)
	}
}

func TestFieldHasValue(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 4003, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	if fieldHasValue(db, user, event.ID, "phone") {
		t.Error("expected phone to have no value on a freshly created event")
	}

	phone := "0501234567"
	setEventField(event, "phone", &phone)
	if _, err := services.UpdateEvent(ctx, db, user, event); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if !fieldHasValue(db, user, event.ID, "phone") {
		t.Error("expected phone to have a value after setting it")
	}
}
