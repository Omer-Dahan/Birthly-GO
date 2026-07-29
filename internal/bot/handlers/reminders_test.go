package handlers

import (
	"context"
	"path/filepath"
	"testing"

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

func TestParsePackedRuleTime(t *testing.T) {
	hour, mm, ruleID, ok := parsePackedRuleTime("18-00:42")
	if !ok || hour != 18 || mm != "00" || ruleID != 42 {
		t.Errorf("parsePackedRuleTime(18-00:42) = (%d,%q,%d,%v), want (18,00,42,true)", hour, mm, ruleID, ok)
	}

	_, _, _, ok = parsePackedRuleTime("no-colon-here")
	if ok {
		t.Error("parsePackedRuleTime with no colon should return ok=false")
	}
	_, _, _, ok = parsePackedRuleTime("1800:42")
	if ok {
		t.Error("parsePackedRuleTime with no dash in the time part should return ok=false")
	}
	_, _, _, ok = parsePackedRuleTime("18-aa:42")
	if ok {
		t.Error("parsePackedRuleTime with non-digit minutes should return ok=false")
	}
	_, _, _, ok = parsePackedRuleTime("18-00:abc")
	if ok {
		t.Error("parsePackedRuleTime with non-digit rule id should return ok=false")
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
