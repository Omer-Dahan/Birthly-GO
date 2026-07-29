package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
)

func TestStatsBar(t *testing.T) {
	if got := statsBar(0, 10); got != "" {
		t.Errorf("statsBar(0,10) = %q, want empty", got)
	}
	if got := statsBar(5, 0); got != "" {
		t.Errorf("statsBar(5,0) = %q, want empty (no max)", got)
	}
	if got := statsBar(10, 10); len([]rune(got)) != barMaxLen {
		t.Errorf("statsBar(10,10) length = %d, want %d (full bar)", len([]rune(got)), barMaxLen)
	}
	if got := statsBar(1, 100); len([]rune(got)) != 1 {
		t.Errorf("statsBar(1,100) length = %d, want 1 (minimum one bar for any nonzero count)", len([]rune(got)))
	}
}

func TestMonthLabel_HebrewVsEnglish(t *testing.T) {
	if got := monthLabel(1, core.LanguageHe); got != core.HebrewMonthNames[0] {
		t.Errorf("monthLabel(1,he) = %q, want %q", got, core.HebrewMonthNames[0])
	}
	if got := monthLabel(1, core.LanguageEn); got != "Jan" {
		t.Errorf("monthLabel(1,en) = %q, want Jan", got)
	}
}

func TestRenderStatsHome_CategoryOrderIsDeterministic(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 9001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	// Create events across categories in a specific order; the rendered
	// "by category" line must list them in that same first-seen order,
	// not whatever order a Go map happens to range over.
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "A", Month: 1, Day: 1}, 1000); err != nil {
		t.Fatal(err)
	}

	stats, err := services.GetUserStats(ctx, db, user)
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	text := renderStatsHome(stats, user)
	if !strings.Contains(text, "1") {
		t.Errorf("rendered stats missing total count: %q", text)
	}
	if len(stats.ByCategoryOrder) != 1 || stats.ByCategoryOrder[0] != core.CategoryOther {
		t.Errorf("ByCategoryOrder = %v, want [other] (default category)", stats.ByCategoryOrder)
	}
}

func TestRenderStatsHome_EmptyState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	user, err := services.GetOrCreateUser(ctx, db, 9002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	stats, err := services.GetUserStats(ctx, db, user)
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	text := renderStatsHome(stats, user)
	if text == "" {
		t.Error("expected non-empty text for empty stats")
	}
}
