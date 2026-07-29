package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
)

func TestSearchEvents_ByFreeTextAndCategory(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 8001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	lastName := "Cohen"
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", LastName: &lastName, Month: 3, Day: 15}, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Yossi", Month: 4, Day: 1}, 1000); err != nil {
		t.Fatal(err)
	}

	byName, err := services.SearchEvents(ctx, db, user, "dana")
	if err != nil {
		t.Fatalf("SearchEvents by name: %v", err)
	}
	if len(byName) != 1 || byName[0].FirstName != "Dana" {
		t.Errorf("SearchEvents(dana) = %+v, want just Dana", byName)
	}

	noMatch, err := services.SearchEvents(ctx, db, user, "nonexistent")
	if err != nil {
		t.Fatalf("SearchEvents no match: %v", err)
	}
	if len(noMatch) != 0 {
		t.Errorf("SearchEvents(nonexistent) = %+v, want empty", noMatch)
	}
}

func TestSearchResultsKeyboard_HasEventRowsAndFooter(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 8002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	kb := searchResultsKeyboard(user, []*models.Event{event})
	// 1 event row + 1 footer row (again/home).
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("keyboard rows = %d, want 2", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 1 {
		t.Errorf("event row buttons = %d, want 1", len(kb.InlineKeyboard[0]))
	}
	if len(kb.InlineKeyboard[1]) != 2 {
		t.Errorf("footer row buttons = %d, want 2 (again, home)", len(kb.InlineKeyboard[1]))
	}
}
