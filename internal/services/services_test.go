package services

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"birthly/internal/core"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustUser(t *testing.T, ctx context.Context, db *sql.DB, id int64) *models.User {
	t.Helper()
	name := "Dana"
	user, err := GetOrCreateUser(ctx, db, id, nil, name, nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	return user
}

func TestGetOrCreateUser_CreatesDefaultRulesAtomically(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	user := mustUser(t, ctx, db, 1001)
	rules := repo.NewReminderRuleRepo(db, user.ID)
	global, err := rules.ListGlobal(ctx)
	if err != nil {
		t.Fatalf("ListGlobal: %v", err)
	}
	if len(global) != 2 {
		t.Fatalf("expected 2 default global rules, got %d", len(global))
	}

	// second contact: not re-created, no duplicate rules
	user2, err := GetOrCreateUser(ctx, db, 1001, nil, "Dana Updated", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser second contact: %v", err)
	}
	if user2.FirstName != "Dana Updated" {
		t.Errorf("profile not refreshed: %+v", user2)
	}
	global2, _ := rules.ListGlobal(ctx)
	if len(global2) != 2 {
		t.Errorf("expected still 2 global rules after second contact, got %d", len(global2))
	}
}

func TestCreateMinimalEvent_EnforcesLimitAndComputesOccurrence(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 2002)

	year := 1990
	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{
		FirstName: "Yossi", Month: 3, Day: 15, Year: &year,
	}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}
	if event.NextOccurrence == nil {
		t.Fatal("expected NextOccurrence to be computed")
	}
	if event.CalendarType != core.CalendarTypeGregorian {
		t.Errorf("expected default calendar_type=gregorian, got %s", event.CalendarType)
	}

	// limit enforcement
	_, err = CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "X", Month: 1, Day: 1}, 1)
	if err == nil {
		t.Fatal("expected LimitErr when at max_events_per_user")
	}
	if _, ok := err.(*LimitErr); !ok {
		t.Errorf("expected *LimitErr, got %T: %v", err, err)
	}
}

func TestUpdateEvent_RecomputesOccurrenceAndIsIDOROwned(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 3003)
	other := mustUser(t, ctx, db, 3004)

	event, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "A", Month: 6, Day: 1}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	// IDOR: another user must not be able to fetch/update it.
	if _, err := GetOwnedEvent(ctx, db, other, event.ID); err == nil {
		t.Error("expected NotFoundErr for cross-user GetOwnedEvent")
	}

	event.Month = 12
	event.Day = 25
	updated, err := UpdateEvent(ctx, db, user, event)
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if updated.NextOccurrence.Month() != 12 || updated.NextOccurrence.Day() != 25 {
		t.Errorf("occurrence not recomputed after date change: %v", updated.NextOccurrence)
	}
}

func TestDeleteRestoreToggleMute(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 4004)
	event, _ := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "B", Month: 1, Day: 1}, 1000)

	if _, err := DeleteEvent(ctx, db, user, event.ID); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, err := GetOwnedEvent(ctx, db, user, event.ID); err == nil {
		t.Error("expected NotFoundErr after soft delete")
	}
	if _, err := RestoreEvent(ctx, db, user, event.ID); err != nil {
		t.Fatalf("RestoreEvent: %v", err)
	}
	restored, err := GetOwnedEvent(ctx, db, user, event.ID)
	if err != nil || restored == nil {
		t.Fatalf("event should be visible after restore: %v", err)
	}

	toggled, err := ToggleMute(ctx, db, user, event.ID)
	if err != nil {
		t.Fatalf("ToggleMute: %v", err)
	}
	if toggled.IsActive {
		t.Error("expected IsActive=false after first toggle")
	}
}

func TestRenderTemplate_AgeSentenceRemovalWhenYearUnknown(t *testing.T) {
	user := &models.User{Language: core.LanguageHe, Timezone: "Asia/Jerusalem"}
	event := &models.Event{FirstName: "Dana", CalendarType: core.CalendarTypeGregorian, Month: 1, Day: 1}
	tpl := &models.GreetingTemplate{Body: "מזל טוב {name}! את בת {age}. שיהיה יום נהדר."}

	got := RenderTemplate(tpl, event, user)
	if got != "מזל טוב Dana! שיהיה יום נהדר." {
		t.Errorf("RenderTemplate (no year) = %q", got)
	}

	year := 1990
	event.Year = &year
	got2 := RenderTemplate(tpl, event, user)
	if got2 == got {
		t.Errorf("RenderTemplate with year should include the age sentence, got %q", got2)
	}
}

func TestRenderTemplate_NicknameFallback(t *testing.T) {
	user := &models.User{Language: core.LanguageHe, Timezone: "Asia/Jerusalem"}
	event := &models.Event{FirstName: "Dana", CalendarType: core.CalendarTypeGregorian, Month: 1, Day: 1}
	tpl := &models.GreetingTemplate{Body: "{nickname} שלי!"}

	got := RenderTemplate(tpl, event, user)
	if got != "Dana שלי!" {
		t.Errorf("RenderTemplate nickname fallback = %q, want %q", got, "Dana שלי!")
	}
}

func TestGetUserStats_BasicAggregation(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 5005)

	y1, y2 := 1990, 2000
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "A", Month: 1, Day: 1, Year: &y1}, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "B", Month: 6, Day: 15, Year: &y2}, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "C", Month: 3, Day: 3}, 1000); err != nil {
		t.Fatal(err)
	}

	stats, err := GetUserStats(ctx, db, user)
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}
	if stats.WithoutYear != 1 {
		t.Errorf("WithoutYear = %d, want 1", stats.WithoutYear)
	}
	if stats.Youngest == nil || stats.Oldest == nil {
		t.Fatal("expected Youngest/Oldest to be set")
	}
	if stats.Youngest.Event.FirstName == stats.Oldest.Event.FirstName && *stats.AvgAge != float64(stats.Youngest.Age) {
		t.Errorf("Youngest and Oldest collapsed unexpectedly")
	}
}

func TestSettingsUpdateAndWipe(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 6006)

	if err := SetLanguage(ctx, db, user, core.LanguageEn); err != nil {
		t.Fatalf("SetLanguage: %v", err)
	}
	if user.Language != core.LanguageEn {
		t.Errorf("in-memory user.Language not updated")
	}
	reloaded, err := repo.NewUserRepo(db).Get(ctx, user.ID)
	if err != nil || reloaded.Language != core.LanguageEn {
		t.Errorf("persisted language mismatch: %+v, err=%v", reloaded, err)
	}

	if _, err := ValidateTimezone("Definitely/NotAZone"); err == nil {
		t.Error("expected error for invalid timezone")
	}
	if tz, err := ValidateTimezone(" Asia/Jerusalem "); err != nil || tz != "Asia/Jerusalem" {
		t.Errorf("ValidateTimezone(Asia/Jerusalem) = (%q,%v)", tz, err)
	}

	if err := WipeAccount(ctx, db, user); err != nil {
		t.Fatalf("WipeAccount: %v", err)
	}
	gone, err := repo.NewUserRepo(db).Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("Get after wipe: %v", err)
	}
	if gone != nil {
		t.Error("user row should be gone after WipeAccount")
	}
}
