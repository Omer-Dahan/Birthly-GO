package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/bot/callbacks"
	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

func TestRenderRules_GlobalVsPerEvent(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 5001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	// GetOrCreateUser already seeds 2 default global rules.

	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}
	offset := 3
	if _, err := repo.NewReminderRuleRepo(db, user.ID).Create(ctx, &models.ReminderRule{EventID: &event.ID, OffsetDays: &offset, Enabled: true}); err != nil {
		t.Fatalf("create per-event rule: %v", err)
	}

	globalRules, err := repo.NewReminderRuleRepo(db, user.ID).ListGlobal(ctx)
	if err != nil {
		t.Fatalf("ListGlobal: %v", err)
	}
	if len(globalRules) != 2 {
		t.Errorf("global rules = %d, want 2 (from GetOrCreateUser defaults)", len(globalRules))
	}

	eventRules, err := repo.NewReminderRuleRepo(db, user.ID).ListForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("ListForEvent: %v", err)
	}
	if len(eventRules) != 1 {
		t.Errorf("event rules = %d, want 1", len(eventRules))
	}
}

func TestReminderLimits(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 5002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	ruleRepo := repo.NewReminderRuleRepo(db, user.ID)

	count, err := ruleRepo.CountGlobal(ctx)
	if err != nil {
		t.Fatalf("CountGlobal: %v", err)
	}
	if count != 2 {
		t.Fatalf("initial global count = %d, want 2", count)
	}

	// Fill up to the global limit.
	for count < core.MaxReminderRulesGlobal {
		offset := count
		if _, err := ruleRepo.Create(ctx, &models.ReminderRule{OffsetDays: &offset, Enabled: true}); err != nil {
			t.Fatalf("Create rule: %v", err)
		}
		count++
	}
	final, err := ruleRepo.CountGlobal(ctx)
	if err != nil {
		t.Fatalf("CountGlobal: %v", err)
	}
	if final != core.MaxReminderRulesGlobal {
		t.Errorf("final global count = %d, want %d", final, core.MaxReminderRulesGlobal)
	}
}

func TestFmtHHMM(t *testing.T) {
	cases := []struct {
		hour int
		mm   string
		want string
	}{
		{9, "00", "09:00"},
		{18, "30", "18:30"},
		{0, "05", "00:05"},
		{23, "59", "23:59"},
	}
	for _, c := range cases {
		if got := fmtHHMM(c.hour, c.mm); got != c.want {
			t.Errorf("fmtHHMM(%d,%q) = %q, want %q", c.hour, c.mm, got, c.want)
		}
	}
}

// TestReminderTimeCallback_RoundTrip pins down a regression: the "time"
// action used to pack "HH-MM:ruleID" into Reminder.Value, which collided
// with callback_data's own colon field separator and made DecodeReminder
// fail for every time-picker button (worse yet with EventID also set, i.e.
// picking a time for a per-event reminder rule). RuleID is now its own
// field precisely so Value never needs a second colon.
func TestReminderTimeCallback_RoundTrip(t *testing.T) {
	eventID := int64(7)
	ruleID := int64(42)
	v := "18-00"
	data := callbacks.Reminder{Action: "time", Value: &v, EventID: &eventID, RuleID: &ruleID}.Encode()

	rc, err := callbacks.DecodeReminder(data)
	if err != nil {
		t.Fatalf("DecodeReminder(%q): %v", data, err)
	}
	if rc.Value == nil || *rc.Value != "18-00" {
		t.Errorf("Value = %v, want 18-00", rc.Value)
	}
	if rc.EventID == nil || *rc.EventID != 7 {
		t.Errorf("EventID = %v, want 7", rc.EventID)
	}
	if rc.RuleID == nil || *rc.RuleID != 42 {
		t.Errorf("RuleID = %v, want 42", rc.RuleID)
	}
}

func TestIsAllDigits(t *testing.T) {
	cases := map[string]bool{"123": true, "": false, "12a": false, "0": true}
	for in, want := range cases {
		if got := isAllDigits(in); got != want {
			t.Errorf("isAllDigits(%q) = %v, want %v", in, got, want)
		}
	}
}
