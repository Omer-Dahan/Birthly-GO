package main

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// seedDualDate builds the scenario from the "reminder ~8 days early" report:
// a hebrew-primary birthday whose gregorian secondary date is 8 days later,
// with the stock day-of global rule. The primary track's day-of reminder
// has been sent; the user thinks of the event by its gregorian date.
func seedDualDate(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 42, nil, "Omer", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	primary := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	secondary := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	gregorian := "gregorian"
	secMonth, secDay := 10, 8
	event, err := repo.NewEventRepo(db, 42).Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "hebrew", Month: 7, Day: 19,
		NextOccurrence:        &primary,
		SecondaryCalendarType: &gregorian, SecondaryMonth: &secMonth, SecondaryDay: &secDay,
		SecondaryNextOccurrence: &secondary,
		IsActive:                true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	dayOf := 0
	rule, err := repo.NewReminderRuleRepo(db, 42).Create(ctx, &models.ReminderRule{OffsetDays: &dayOf, Enabled: true})
	if err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	notifs := repo.NewNotificationRepo(db)
	log, err := notifs.CreatePending(ctx, 42, event.ID, &rule.ID, primary, time.Date(2026, 9, 30, 6, 0, 0, 0, time.UTC))
	if err != nil || log == nil {
		t.Fatalf("CreatePending: %v", err)
	}
	if err := notifs.MarkSent(ctx, log.ID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
}

func TestReportAttributesSendToTrackAndRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "birthly.db")
	seedDualDate(t, path)

	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("openReadOnly: %v", err)
	}
	defer db.Close()

	var out bytes.Buffer
	opts := options{
		UserID: 42, Since: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), StatusDays: 7,
		BackupDir: t.TempDir(), Now: time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC),
	}
	if err := report(context.Background(), db, opts, &out); err != nil {
		t.Fatalf("report: %v", err)
	}
	got := out.String()

	for _, want := range []string{
		"hebrew 19/07 + gregorian 08/10",
		"2026-09-30  primary",
		"days=0",
		"global",
		"09-30 09:00", // scheduled 06:00 UTC shown in the user's zone
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q\n%s", want, got)
		}
	}
	if !regexp.MustCompile(`gregorian 08/10\s+2026-10-08\s+8\s`).MatchString(got) {
		t.Errorf("dual-date section should show the 8-day gap between tracks\n%s", got)
	}
	if strings.Contains(got, "MISMATCH") {
		t.Errorf("unexpected MISMATCH for a day-of rule that fired on the occurrence day\n%s", got)
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "birthly.db")
	seedDualDate(t, path)

	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("openReadOnly: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`UPDATE users SET first_name = 'x'`); err == nil {
		t.Fatal("write succeeded on a read-only connection")
	}
}
