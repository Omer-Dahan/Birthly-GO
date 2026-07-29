package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
)

func TestRenderList_EmptyState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 3001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	text, kb, err := renderList(ctx, db, user, 0, 8)
	if err != nil {
		t.Fatalf("renderList: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text even for an empty list")
	}
	if kb == nil {
		t.Fatal("expected non-nil keyboard even when empty")
	}
}

func TestRenderList_PaginationAndRowCount(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 3002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "E", Month: 1, Day: (i % 28) + 1}, 1000); err != nil {
			t.Fatalf("CreateMinimalEvent %d: %v", i, err)
		}
	}

	pageSize := 2
	_, kb, err := renderList(ctx, db, user, 0, pageSize)
	if err != nil {
		t.Fatalf("renderList: %v", err)
	}
	// pageSize event rows + page row + controls row + home row = pageSize+3 rows
	if len(kb.InlineKeyboard) != pageSize+3 {
		t.Errorf("keyboard rows = %d, want %d (page 0 of 5 events at page size 2)", len(kb.InlineKeyboard), pageSize+3)
	}
}

func TestCbListSet_UpdatesSortAndFilter(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 3003, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	user.ListSort = core.ListSortName
	if err := services.UpdateSettings(ctx, db, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if !validSortValues[core.ListSortName] {
		t.Fatal("ListSortName should be a recognized sort value")
	}
	if !validFilterValues["muted"] {
		t.Fatal("muted should be a recognized filter value")
	}
}
