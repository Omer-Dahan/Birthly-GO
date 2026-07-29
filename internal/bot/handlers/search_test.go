package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
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

func TestSearchEvents_ByCategoryName(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 8003, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	events := repo.NewEventRepo(db, user.ID)
	familyEvent, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "family",
		CalendarType: "gregorian", Month: 3, Day: 15, IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Yossi", Category: "work",
		CalendarType: "gregorian", Month: 4, Day: 1, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := services.SearchEvents(ctx, db, user, "משפחה")
	if err != nil {
		t.Fatalf("SearchEvents by category: %v", err)
	}
	if len(got) != 1 || got[0].ID != familyEvent.ID {
		t.Errorf("SearchEvents(משפחה) = %+v, want just the family-category event", got)
	}
}

func TestSearchEvents_ByHebrewMonthName(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 8004, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	marchEvent, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Yossi", Month: 4, Day: 1}, 1000); err != nil {
		t.Fatal(err)
	}

	got, err := services.SearchEvents(ctx, db, user, "מרץ")
	if err != nil {
		t.Fatalf("SearchEvents by Hebrew month name: %v", err)
	}
	if len(got) != 1 || got[0].ID != marchEvent.ID {
		t.Errorf("SearchEvents(מרץ) = %+v, want just the March event", got)
	}
}

func TestSearchEvents_ByPhoneSubstring(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 8005, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	events := repo.NewEventRepo(db, user.ID)
	phone := "050-1234567"
	withPhone, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "gregorian", Month: 3, Day: 15, IsActive: true, Phone: &phone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Yossi", Category: "other",
		CalendarType: "gregorian", Month: 4, Day: 1, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := services.SearchEvents(ctx, db, user, "050")
	if err != nil {
		t.Fatalf("SearchEvents by phone: %v", err)
	}
	if len(got) != 1 || got[0].ID != withPhone.ID {
		t.Errorf("SearchEvents(050) = %+v, want just the event with a matching phone", got)
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
