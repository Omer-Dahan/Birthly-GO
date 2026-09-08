package backfill

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

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

// makeUser creates a user with ShowHebrewDate set as requested, returning its id.
func makeUser(t *testing.T, ctx context.Context, db *sql.DB, telegramID int64, showHebrewDate bool) int64 {
	t.Helper()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, telegramID, nil, "Test", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET show_hebrew_date = ? WHERE id = ?`, showHebrewDate, user.ID); err != nil {
		t.Fatalf("setting show_hebrew_date: %v", err)
	}
	return user.ID
}

// makeEvent inserts a birthday event directly (bypassing the service layer,
// which doesn't offer gregorian-primary + hebrew-secondary events) and
// returns its id.
func makeEvent(t *testing.T, ctx context.Context, db *sql.DB, userID int64, calendarType string, year *int, month, day int, isActive bool, deleted bool) int64 {
	t.Helper()
	events := repo.NewEventRepo(db, userID)
	occ, err := core.NextOccurrence(calendarType, month, day, time.Now().UTC(), core.AdarPolicyAdarII, core.Feb29PolicyFeb28)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	e, err := events.Create(ctx, &models.Event{
		EventType: core.EventTypeBirthday, FirstName: "Yael", Category: core.CategoryOther,
		CalendarType: calendarType, Year: year, Month: month, Day: day,
		IsActive: isActive, NextOccurrence: &occ,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	if deleted {
		if err := events.SoftDelete(ctx, e.ID); err != nil {
			t.Fatalf("SoftDelete: %v", err)
		}
	}
	return e.ID
}

func TestPlan_ConvertsGregorianBirthdayToHebrewWithSecondary(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	userID := makeUser(t, ctx, db, 1001, true)
	year := 2002
	eventID := makeEvent(t, ctx, db, userID, core.CalendarTypeGregorian, &year, 11, 4, true, false)

	changes, err := Plan(ctx, db)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("Plan returned %d changes, want 1: %+v", len(changes), changes)
	}
	c := changes[0]
	if c.EventID != eventID {
		t.Errorf("EventID = %d, want %d", c.EventID, eventID)
	}
	// 2002-11-04 is 29 Cheshvan 5763 on the Hebrew calendar.
	if c.AfterYear != 5763 || c.AfterMonth != 8 || c.AfterDay != 29 {
		t.Errorf("hebrew date = %d/%d/%d, want 5763/8/29", c.AfterYear, c.AfterMonth, c.AfterDay)
	}
	if c.SecondaryMonth != 11 || c.SecondaryDay != 4 {
		t.Errorf("secondary date = %d/%d, want 11/4", c.SecondaryMonth, c.SecondaryDay)
	}
	if c.NextOccurrence.IsZero() || c.SecondaryNextOccurrence.IsZero() {
		t.Errorf("next occurrences not computed: %+v", c)
	}

	if err := Apply(ctx, db, changes); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	events := repo.NewEventRepo(db, userID)
	got, err := events.GetOwned(ctx, eventID)
	if err != nil || got == nil {
		t.Fatalf("GetOwned after apply: got=%v err=%v", got, err)
	}
	if got.CalendarType != core.CalendarTypeHebrew {
		t.Errorf("CalendarType = %q, want hebrew", got.CalendarType)
	}
	if got.Year == nil || *got.Year != 5763 || got.Month != 8 || got.Day != 29 {
		t.Errorf("primary date = %v/%d/%d, want 5763/8/29", got.Year, got.Month, got.Day)
	}
	if got.SecondaryCalendarType == nil || *got.SecondaryCalendarType != core.CalendarTypeGregorian {
		t.Errorf("SecondaryCalendarType = %v, want gregorian", got.SecondaryCalendarType)
	}
	if got.SecondaryMonth == nil || *got.SecondaryMonth != 11 || got.SecondaryDay == nil || *got.SecondaryDay != 4 {
		t.Errorf("secondary date = %v/%v, want 11/4", got.SecondaryMonth, got.SecondaryDay)
	}
	if got.NextOccurrence == nil || got.SecondaryNextOccurrence == nil {
		t.Errorf("next occurrences not persisted: %+v", got)
	}

	// AgeAt must read the new Year as a Hebrew year, not the original
	// Gregorian birth year 2002, that's the whole point of converting Year,
	// not just Month/Day.
	age, ok := core.AgeAt(got.CalendarType, got.Year, *got.NextOccurrence)
	if !ok {
		t.Fatalf("AgeAt: not ok")
	}
	if age < 0 || age > 130 {
		t.Errorf("AgeAt = %d, implausible for a 2002 birth (Year must be the Hebrew birth year)", age)
	}
}

func TestPlan_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	userID := makeUser(t, ctx, db, 1002, true)
	year := 2002
	makeEvent(t, ctx, db, userID, core.CalendarTypeGregorian, &year, 11, 4, true, false)

	changes, err := Plan(ctx, db)
	if err != nil || len(changes) != 1 {
		t.Fatalf("first Plan: changes=%d err=%v", len(changes), err)
	}
	if err := Apply(ctx, db, changes); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	changesAfter, err := Plan(ctx, db)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if len(changesAfter) != 0 {
		t.Fatalf("second Plan returned %d changes, want 0 (idempotent)", len(changesAfter))
	}

	// Re-applying an empty change set must be a harmless no-op.
	if err := Apply(ctx, db, changesAfter); err != nil {
		t.Fatalf("second Apply (no-op): %v", err)
	}
}

func TestPlan_ScopedToEligibleEventsOnly(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	year := 2002

	// User without ShowHebrewDate: event must be left alone.
	userNoPref := makeUser(t, ctx, db, 2001, false)
	makeEvent(t, ctx, db, userNoPref, core.CalendarTypeGregorian, &year, 11, 4, true, false)

	// User with ShowHebrewDate, but an event with no birth year: left alone.
	userWithPref := makeUser(t, ctx, db, 2002, true)
	makeEvent(t, ctx, db, userWithPref, core.CalendarTypeGregorian, nil, 6, 15, true, false)

	// Already a hebrew-primary event: left alone.
	makeEvent(t, ctx, db, userWithPref, core.CalendarTypeHebrew, &year, 8, 29, true, false)

	// Soft-deleted gregorian event with a year: left alone.
	makeEvent(t, ctx, db, userWithPref, core.CalendarTypeGregorian, &year, 3, 1, true, true)

	// Muted (is_active=0) gregorian event with a year: left alone.
	makeEvent(t, ctx, db, userWithPref, core.CalendarTypeGregorian, &year, 4, 1, false, false)

	// The one eligible event.
	eligibleID := makeEvent(t, ctx, db, userWithPref, core.CalendarTypeGregorian, &year, 11, 4, true, false)

	changes, err := Plan(ctx, db)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("Plan returned %d changes, want 1: %+v", len(changes), changes)
	}
	if changes[0].EventID != eligibleID {
		t.Errorf("EventID = %d, want %d", changes[0].EventID, eligibleID)
	}
}

func TestPlan_DryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	userID := makeUser(t, ctx, db, 3001, true)
	year := 2002
	eventID := makeEvent(t, ctx, db, userID, core.CalendarTypeGregorian, &year, 11, 4, true, false)

	before, err := repo.NewEventRepo(db, userID).GetOwned(ctx, eventID)
	if err != nil {
		t.Fatalf("GetOwned before: %v", err)
	}

	if _, err := Plan(ctx, db); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	after, err := repo.NewEventRepo(db, userID).GetOwned(ctx, eventID)
	if err != nil {
		t.Fatalf("GetOwned after: %v", err)
	}
	if after.CalendarType != before.CalendarType || after.Month != before.Month || after.Day != before.Day {
		t.Errorf("Plan mutated the event: before=%+v after=%+v", before, after)
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Errorf("Plan touched updated_at: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}
