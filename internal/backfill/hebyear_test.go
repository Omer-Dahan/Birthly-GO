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

func TestManualDateRowReportedNotWrittenEvenWhenNamed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uid := makeUser(t, ctx, db, 2002, false)
	_, hm, hd := core.ToHebrew(time.Date(1983, 11, 30, 0, 0, 0, 0, time.UTC))
	y := 5744 // correct year
	id := makeEvent(t, ctx, db, uid, core.CalendarTypeHebrew, &y, hm, hd, true, false)
	// The date matches the next Hebrew year but was entered by hand later.
	if _, err := db.ExecContext(ctx, `UPDATE events SET secondary_calendar_type='gregorian', secondary_month=11, secondary_day=30,
		created_at='2020-01-01 00:00:00', updated_at='2021-01-01 00:00:00' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	// Make the manual date match year+1 rather than the stored year.
	d2, err := core.ToGregorian(y+1, core.ResolveMonth(y+1, hm, core.AdarPolicyAdarII), hd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE events SET secondary_month=?, secondary_day=? WHERE id=?`, int(d2.Month()), d2.Day(), id); err != nil {
		t.Fatal(err)
	}
	rep, err := PlanHebrewYearRepair(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Fixes) != 1 || rep.Fixes[0].Writable || rep.Fixes[0].Reason == "" || rep.Provable != 0 {
		t.Fatalf("expected one reported, non-writable candidate: %+v", rep)
	}
	res, err := ApplyHebrewYearRepair(ctx, db, rep.Fixes)
	if err != nil || res.Applied != 0 || len(res.Skipped) != 1 {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	var got int
	if err := db.QueryRowContext(ctx, `SELECT year FROM events WHERE id=?`, id).Scan(&got); err != nil || got != y {
		t.Fatalf("year = %d (%v), want %d", got, err, y)
	}
}

func TestRepairSurvivesSchedulerTouchButNotYearChange(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	uid := makeUser(t, ctx, db, 2003, false)
	_, hm, hd := core.ToHebrew(time.Date(1983, 11, 30, 0, 0, 0, 0, time.UTC))
	mk := func() int64 {
		y := 5743
		id := makeEvent(t, ctx, db, uid, core.CalendarTypeHebrew, &y, hm, hd, true, false)
		if _, err := db.ExecContext(ctx, `UPDATE events SET secondary_calendar_type='gregorian', secondary_month=11, secondary_day=30 WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	touched, edited := mk(), mk()
	rep, err := PlanHebrewYearRepair(ctx, db)
	if err != nil || len(rep.Fixes) != 2 || rep.Provable != 2 {
		t.Fatalf("plan = %+v, %v", rep, err)
	}
	// Scheduler advances an occurrence: only updated_at moves.
	if _, err := db.ExecContext(ctx, `UPDATE events SET updated_at='2099-01-01 00:00:00' WHERE id=?`, touched); err != nil {
		t.Fatal(err)
	}
	// A real edit: the year changes.
	if _, err := db.ExecContext(ctx, `UPDATE events SET year=5700, updated_at='2099-01-01 00:00:00' WHERE id=?`, edited); err != nil {
		t.Fatal(err)
	}
	res, err := ApplyHebrewYearRepair(ctx, db, rep.Fixes)
	if err != nil || res.Applied != 1 || len(res.Skipped) != 1 || res.Skipped[0] != edited {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	var y1, y2 int
	db.QueryRowContext(ctx, `SELECT year FROM events WHERE id=?`, touched).Scan(&y1)
	db.QueryRowContext(ctx, `SELECT year FROM events WHERE id=?`, edited).Scan(&y2)
	if y1 != 5744 || y2 != 5700 {
		t.Fatalf("years = %d, %d", y1, y2)
	}
}
