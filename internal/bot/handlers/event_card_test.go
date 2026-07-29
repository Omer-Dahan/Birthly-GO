package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"birthly/internal/services"
	"birthly/internal/store"
)

func TestRenderCard_OwnedVsCrossUser(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	owner, err := services.GetOrCreateUser(ctx, db, 2001, nil, "Owner", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser owner: %v", err)
	}
	other, err := services.GetOrCreateUser(ctx, db, 2002, nil, "Other", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser other: %v", err)
	}

	event, err := services.CreateMinimalEvent(ctx, db, owner, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	text, kb, err := RenderCard(ctx, db, owner, event.ID)
	if err != nil {
		t.Fatalf("RenderCard (owner): %v", err)
	}
	if !strings.Contains(text, "Dana") {
		t.Errorf("card text missing name: %q", text)
	}
	if kb == nil || len(kb.InlineKeyboard) == 0 {
		t.Error("expected a non-empty keyboard")
	}

	_, _, err = RenderCard(ctx, db, other, event.ID)
	if !isNotFound(err) {
		t.Errorf("RenderCard for a non-owner should return NotFoundErr, got %v", err)
	}
}

func TestEventCardFlow_DeleteRestore(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2003, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	if _, err := services.DeleteEvent(ctx, db, user, event.ID); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, _, err := RenderCard(ctx, db, user, event.ID); !isNotFound(err) {
		t.Errorf("expected NotFoundErr for a soft-deleted event's card, got %v", err)
	}

	if _, err := services.RestoreEvent(ctx, db, user, event.ID); err != nil {
		t.Fatalf("RestoreEvent: %v", err)
	}
	if _, _, err := RenderCard(ctx, db, user, event.ID); err != nil {
		t.Errorf("expected card to render again after restore, got %v", err)
	}
}

func TestShareText_WithAndWithoutAge(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 2004, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	year := 1990
	withYear, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15, Year: &year}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got := shareText(user, withYear)
	if !strings.Contains(got, "(") {
		t.Errorf("shareText with known year should include age in parens: %q", got)
	}

	withoutYear, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Yossi", Month: 4, Day: 1}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got2 := shareText(user, withoutYear)
	if strings.Contains(got2, "(") {
		t.Errorf("shareText without a year should omit the age parens: %q", got2)
	}
}
