package backfill

import (
	"context"
	"testing"
	"time"

	"birthly/internal/core"
	"birthly/internal/store/repo"
)

func TestHebrewYearRepair(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uid := makeUser(t, ctx, db, 2001, false)

	// Born 30.11.1983: Hebrew year 5744, old code stored 5743.
	hy, hm, hd := core.ToHebrew(time.Date(1983, 11, 30, 0, 0, 0, 0, time.UTC))
	if hy != 5744 {
		t.Fatalf("fixture: hebrew year %d", hy)
	}
	mk := func(year int, sec bool, sm, sd int) int64 {
		y := year
		id := makeEvent(t, ctx, db, uid, core.CalendarTypeHebrew, &y, hm, hd, true, false)
		if sec {
			if _, err := db.ExecContext(ctx, `UPDATE events SET secondary_calendar_type='gregorian', secondary_month=?, secondary_day=? WHERE id=?`, sm, sd, id); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	provable := mk(5743, true, 11, 30)
	correct := mk(5744, true, 11, 30)
	noSec := mk(5743, false, 0, 0)
	other := mk(5743, true, 3, 3)

	rep, err := PlanHebrewYearRepair(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Provable != 1 || rep.Consistent != 1 || rep.Ambiguous != 2 || rep.NoSecondary != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if res, err := ApplyHebrewYearRepair(ctx, db, rep.Fixes); err != nil || res.Applied != 1 || len(res.Skipped) != 0 {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	year := func(id int64) int {
		e, err := repo.NewEventRepo(db, uid).GetOwned(ctx, id)
		if err != nil || e == nil || e.Year == nil {
			t.Fatalf("GetOwned %d: %v", id, err)
		}
		return *e.Year
	}
	for id, want := range map[int64]int{provable: 5744, correct: 5744, noSec: 5743, other: 5743} {
		if got := year(id); got != want {
			t.Errorf("event %d year = %d, want %d", id, got, want)
		}
	}
	again, _ := PlanHebrewYearRepair(ctx, db)
	if len(again.Fixes) != 0 || again.Consistent != 2 {
		t.Errorf("not idempotent: %+v", again)
	}
}

func TestApplySkipsRowChangedSinceplanning(t *testing.T) {
}
