package repo

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"birthly/internal/store"
	"birthly/internal/store/models"
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

func TestUserAndEventLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	users := NewUserRepo(db)
	username := "danacohen"
	user, created, err := users.GetOrCreate(ctx, 1001, &username, "Dana", nil, false)
	if err != nil || !created {
		t.Fatalf("GetOrCreate: user=%v created=%v err=%v", user, created, err)
	}
	if user.Language != "he" || user.AdarPolicy != "adar_ii" {
		t.Errorf("unexpected defaults: %+v", user)
	}
	if user.Timezone != "Asia/Jerusalem" {
		t.Errorf("Timezone default = %q, want Asia/Jerusalem", user.Timezone)
	}
	if !user.NotificationsEnabled {
		t.Errorf("NotificationsEnabled default = false, want true")
	}

	// second contact: not created, profile refreshed
	user2, created2, err := users.GetOrCreate(ctx, 1001, &username, "Dana Updated", nil, false)
	if err != nil || created2 {
		t.Fatalf("GetOrCreate second contact: created=%v err=%v", created2, err)
	}
	if user2.FirstName != "Dana Updated" {
		t.Errorf("profile not refreshed: %+v", user2)
	}

	events := NewEventRepo(db, user.ID)
	year := 1990
	e := &models.Event{
		EventType: "birthday", FirstName: "Yossi", Category: "family",
		CalendarType: "gregorian", Year: &year, Month: 3, Day: 15, IsActive: true,
	}
	created3, err := events.Create(ctx, e)
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	if created3.ID == 0 || created3.UserID != user.ID {
		t.Errorf("unexpected created event: %+v", created3)
	}

	// IDOR protection: another user's repo must not see this event.
	otherRepo := NewEventRepo(db, 9999)
	notOwned, err := otherRepo.GetOwned(ctx, created3.ID)
	if err != nil {
		t.Fatalf("GetOwned cross-user: %v", err)
	}
	if notOwned != nil {
		t.Errorf("cross-user GetOwned leaked a row: %+v", notOwned)
	}

	count, err := events.CountNotDeleted(ctx)
	if err != nil || count != 1 {
		t.Errorf("CountNotDeleted = %d, %v; want 1", count, err)
	}

	if err := events.SoftDelete(ctx, created3.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	count2, _ := events.CountNotDeleted(ctx)
	if count2 != 0 {
		t.Errorf("after soft delete CountNotDeleted = %d; want 0", count2)
	}
	list, total, err := events.ListPage(ctx, models.SortUpcoming, models.ListViewTrash, 0, 10)
	if err != nil {
		t.Fatalf("ListPage trash: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("ListPage trash = %d items, total %d; want 1, 1", len(list), total)
	}

	if err := events.Restore(ctx, created3.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	count3, _ := events.CountNotDeleted(ctx)
	if count3 != 1 {
		t.Errorf("after restore CountNotDeleted = %d; want 1", count3)
	}
}

func TestReminderRuleUnusedOffsetConstraint(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "x"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 2002, &uname, "X", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	rules := NewReminderRuleRepo(db, user.ID)
	offsetDays := 1
	rule, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetDays, Enabled: true})
	if err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	toggled, err := rules.Toggle(ctx, rule.ID)
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if toggled.Enabled {
		t.Errorf("expected rule disabled after toggle, got enabled")
	}

	// exactly_one_offset CHECK: both offset_days and offset_minutes set must fail.
	offsetMinutes := 30
	_, err = db.ExecContext(ctx,
		`INSERT INTO reminder_rules (user_id, offset_days, offset_minutes) VALUES (?, ?, ?)`,
		user.ID, offsetDays, offsetMinutes,
	)
	if err == nil {
		t.Errorf("expected CHECK constraint violation for both offsets set, got no error")
	}

	// exactly_one_offset CHECK: neither offset set must also fail — the
	// constraint is a strict XOR, not just "at most one".
	_, err = db.ExecContext(ctx,
		`INSERT INTO reminder_rules (user_id, offset_days, offset_minutes) VALUES (?, NULL, NULL)`,
		user.ID,
	)
	if err == nil {
		t.Errorf("expected CHECK constraint violation for neither offset set, got no error")
	}
}

func TestUserDeleteCascadesEventsAndRules(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "cascade"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 5005, &uname, "Cascade", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	events := NewEventRepo(db, user.ID)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "E", Category: "other",
		CalendarType: "gregorian", Month: 1, Day: 1, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := NewReminderRuleRepo(db, user.ID)
	offsetDays := 1
	if _, err := rules.Create(ctx, &models.ReminderRule{EventID: &event.ID, OffsetDays: &offsetDays, Enabled: true}); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	if err := users.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete user: %v", err)
	}

	var eventCount, ruleCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE user_id = ?`, user.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminder_rules WHERE user_id = ?`, user.ID).Scan(&ruleCount); err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if eventCount != 0 {
		t.Errorf("events remaining after user delete = %d, want 0 (ON DELETE CASCADE)", eventCount)
	}
	if ruleCount != 0 {
		t.Errorf("reminder_rules remaining after user delete = %d, want 0 (ON DELETE CASCADE)", ruleCount)
	}
}

func TestMinimalEventGetsDocumentedDefaults(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "minimal"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 5006, &uname, "Minimal", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	// Insert with only user_id + first_name + month + day set — every other
	// column omitted entirely, so the schema's own DEFAULT clauses apply
	// (unlike EventRepo.Create, which always supplies every column
	// explicitly and so never exercises these DB-level defaults). Matches
	// SPEC ch.11.1's "simplicity contract" at the DB-model level, the way
	// Python's test_db_models.py checks the SQLAlchemy model defaults.
	res, err := db.ExecContext(ctx,
		`INSERT INTO events (user_id, first_name, month, day) VALUES (?, ?, ?, ?)`,
		user.ID, "Dana", 3, 15,
	)
	if err != nil {
		t.Fatalf("insert minimal event: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	events := NewEventRepo(db, user.ID)
	got, err := events.GetOwned(ctx, id)
	if err != nil {
		t.Fatalf("GetOwned: %v", err)
	}
	if got.IsActive != true {
		t.Errorf("IsActive default = %v, want true", got.IsActive)
	}
	if got.Year != nil {
		t.Errorf("Year = %v, want nil", got.Year)
	}
	if got.EventType != "birthday" {
		t.Errorf("EventType = %q, want birthday", got.EventType)
	}
	if got.CalendarType != "gregorian" {
		t.Errorf("CalendarType = %q, want gregorian", got.CalendarType)
	}
	if got.Category != "other" {
		t.Errorf("Category = %q, want other", got.Category)
	}
	if got.LastName != nil || got.Nickname != nil || got.Gender != nil || got.Relation != nil ||
		got.Phone != nil || got.Notes != nil || got.PhotoFileID != nil || got.EventTime != nil {
		t.Errorf("expected every optional field to be nil on a minimal event, got %+v", got)
	}
}

func TestNotificationCreatePendingIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "n"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 3003, &uname, "N", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	events := NewEventRepo(db, user.ID)
	ev, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "E", Category: "other",
		CalendarType: "gregorian", Month: 1, Day: 1, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	// rule_id must be a concrete row for the UNIQUE(event_id, rule_id,
	// occurrence_date) constraint to fire at all: SQLite (like standard SQL)
	// never treats two NULLs as equal for uniqueness, so a NULL rule_id here
	// would silently defeat the idempotency check — same behavior as the
	// Python UniqueConstraint, not a Go-specific gap.
	rules := NewReminderRuleRepo(db, user.ID)
	offsetDays := 0
	rule, err := rules.Create(ctx, &models.ReminderRule{EventID: &ev.ID, OffsetDays: &offsetDays, Enabled: true})
	if err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	notifs := NewNotificationRepo(db)
	occ := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sched := time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC)

	first, err := notifs.CreatePending(ctx, user.ID, ev.ID, &rule.ID, occ, sched)
	if err != nil {
		t.Fatalf("CreatePending first: %v", err)
	}
	if first == nil {
		t.Fatalf("expected first CreatePending to succeed")
	}

	dup, err := notifs.CreatePending(ctx, user.ID, ev.ID, &rule.ID, occ, sched)
	if err != nil {
		t.Fatalf("CreatePending duplicate: unexpected error %v (want nil,nil on UNIQUE violation)", err)
	}
	if dup != nil {
		t.Errorf("expected duplicate CreatePending to return nil, got %+v", dup)
	}

	if err := notifs.MarkSent(ctx, first.ID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
}

func TestUpcomingBetween(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "u"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 4004, &uname, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	events := NewEventRepo(db, user.ID)

	mkEvent := func(name string, occ time.Time) *models.Event {
		e := &models.Event{
			EventType: "birthday", FirstName: name, Category: "other",
			CalendarType: "gregorian", Month: int(occ.Month()), Day: occ.Day(),
			IsActive: true, NextOccurrence: &occ,
		}
		created, err := events.Create(ctx, e)
		if err != nil {
			t.Fatalf("Create event %s: %v", name, err)
		}
		return created
	}

	// Out of range (before window), on the lower boundary, in the middle,
	// on the upper boundary, and out of range (after window) — in
	// deliberately non-sorted insertion order, to prove UpcomingBetween
	// both filters and sorts by occurrence date.
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	mkEvent("TooLate", end.AddDate(0, 0, 1))
	upperBoundary := mkEvent("UpperBoundary", end)
	mkEvent("TooEarly", start.AddDate(0, 0, -1))
	middle := mkEvent("Middle", start.AddDate(0, 0, 3))
	lowerBoundary := mkEvent("LowerBoundary", start)

	got, err := events.UpcomingBetween(ctx, start, end)
	if err != nil {
		t.Fatalf("UpcomingBetween: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("UpcomingBetween returned %d events, want 3 (both boundaries inclusive, middle included)", len(got))
	}
	wantOrder := []int64{lowerBoundary.ID, middle.ID, upperBoundary.ID}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Errorf("UpcomingBetween[%d].ID = %d (%s), want %d", i, got[i].ID, got[i].FirstName, want)
		}
	}
}

func TestUpcomingBetween_EmptyWhenNoneInRange(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uname := "u2"
	users := NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 4005, &uname, "U2", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	events := NewEventRepo(db, user.ID)

	got, err := events.UpcomingBetween(ctx, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("UpcomingBetween: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("UpcomingBetween with no events = %d results, want 0", len(got))
	}
}
