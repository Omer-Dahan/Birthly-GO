package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"birthly/internal/core"
)

// Classes reported by PlanHebrewYearRepair.
const (
	ClassProvable    = "provable"
	ClassConsistent  = "already-correct"
	ClassAmbiguous   = "ambiguous"
	HebrewYearSignal = "secondary Gregorian month/day equals the Gregorian date of the stored Hebrew " +
		"day in year+1 and differs from its date in the stored year, with year+1 landing in the Gregorian " +
		"year (stored year - 3760) that the old conversion assumed"
)

// YearFix is one event whose stored Hebrew birth year is provably one too
// small; Apply sets the year to AfterYear and touches nothing else (the
// month/day, the secondary date and both next-occurrence columns do not
// depend on the year).
type YearFix struct {
	EventID    int64
	UserID     int64
	Name       string
	BeforeYear int
	AfterYear  int
	HebMonth   int
	HebDay     int
	SecMonth   int
	SecDay     int
	// Reason says why the row is flagged.
	Reason string
	// Writable is false when the stored data cannot prove the year is wrong
	// (the row was edited after creation, so its Gregorian date may have been
	// entered by hand). Such a row is reported and never written, even if named.
	Writable bool
}

const (
	reasonProvable = "secondary Gregorian date matches the next Hebrew year, row untouched since creation"
	reasonEdited   = "secondary date matches the next Hebrew year but the row was edited after creation, so the date may be manual; not written"
)

// YearRepairReport is the per-class outcome of a scan.
type YearRepairReport struct {
	Fixes    []YearFix
	Provable int
	// Review counts flagged rows that are reported but never written.
	Review     int
	Consistent int
	Ambiguous  int
	// NoSecondary is the part of Ambiguous that has no stored Gregorian
	// date at all, so nothing can prove or disprove its year.
	NoSecondary int
}

const hebYearQuery = `
SELECT e.id, e.user_id, e.first_name, e.last_name, e.year, e.month, e.day,
       e.secondary_calendar_type, e.secondary_month, e.secondary_day, u.adar_policy,
       (CAST(e.created_at AS TEXT) = CAST(e.updated_at AS TEXT))
FROM events e
JOIN users u ON u.id = e.user_id
WHERE e.deleted_at IS NULL
  AND e.event_type = 'birthday'
  AND e.calendar_type = 'hebrew'
  AND e.year IS NOT NULL
ORDER BY e.id
`

// classifyHebrewYear decides, from stored data only, whether the stored
// Hebrew year is provably one too small.
func classifyHebrewYear(year, hMonth, hDay int, secType sql.NullString, secMonth, secDay sql.NullInt64, adarPolicy string) string {
	if !secType.Valid || secType.String != core.CalendarTypeGregorian || !secMonth.Valid || !secDay.Valid {
		return ClassAmbiguous
	}
	sm, sd := int(secMonth.Int64), int(secDay.Int64)
	d1, err1 := core.ToGregorian(year, core.ResolveMonth(year, hMonth, adarPolicy), hDay)
	d2, err2 := core.ToGregorian(year+1, core.ResolveMonth(year+1, hMonth, adarPolicy), hDay)
	if err1 != nil || err2 != nil {
		return ClassAmbiguous
	}
	if int(d1.Month()) == sm && d1.Day() == sd {
		return ClassConsistent
	}
	if int(d2.Month()) == sm && d2.Day() == sd && d2.Year() == year-3760 {
		return ClassProvable
	}
	return ClassAmbiguous
}

// PlanHebrewYearRepair scans Hebrew-primary birthdays and classifies each.
// It does not write anything.
func PlanHebrewYearRepair(ctx context.Context, db *sql.DB) (YearRepairReport, error) {
	var rep YearRepairReport
	rows, err := db.QueryContext(ctx, hebYearQuery)
	if err != nil {
		return rep, fmt.Errorf("querying hebrew birthdays: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id, userID       int64
			first            string
			last             sql.NullString
			year, month, day int
			secType          sql.NullString
			secMonth, secDay sql.NullInt64
			adarPolicy       string
			untouched        bool
		)
		if err := rows.Scan(&id, &userID, &first, &last, &year, &month, &day, &secType, &secMonth, &secDay, &adarPolicy, &untouched); err != nil {
			return rep, fmt.Errorf("scanning event row: %w", err)
		}
		switch classifyHebrewYear(year, month, day, secType, secMonth, secDay, adarPolicy) {
		case ClassProvable:
			name := first
			if last.Valid && last.String != "" {
				name += " " + last.String
			}
			rep.Fixes = append(rep.Fixes, YearFix{
				EventID: id, UserID: userID, Name: name, BeforeYear: year, AfterYear: year + 1,
				HebMonth: month, HebDay: day, SecMonth: int(secMonth.Int64), SecDay: int(secDay.Int64),
				Reason: reasonEdited, Writable: untouched,
			})
			if untouched {
				rep.Fixes[len(rep.Fixes)-1].Reason = reasonProvable
				rep.Provable++
			} else {
				rep.Review++
			}
		case ClassConsistent:
			rep.Consistent++
		default:
			rep.Ambiguous++
			if !secMonth.Valid || !secDay.Valid {
				rep.NoSecondary++
			}
		}
	}
	return rep, rows.Err()
}

// ApplyResult reports what ApplyHebrewYearRepair did.
type ApplyResult struct {
	Applied int
	// Skipped lists events that were no longer provable (changed, deleted or
	// gone) when re-checked inside the transaction.
	Skipped []int64
}

// ApplyHebrewYearRepair writes the given fixes (the owner-named, previously
// reported candidates) in a single transaction. Every row is re-read inside
// that transaction and must still be exactly as reported (year, Hebrew and
// Gregorian dates) and still provable; updated_at is not consulted, since the
// scheduler also bumps it; anything else,
// including a non-writable candidate, is skipped, never written.
func ApplyHebrewYearRepair(ctx context.Context, db *sql.DB, fixes []YearFix) (ApplyResult, error) {
	var res ApplyResult
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(datetimeLayout)
	for _, f := range fixes {
		if !f.Writable {
			res.Skipped = append(res.Skipped, f.EventID)
			continue
		}
		var (
			year, month, day int
			secType          sql.NullString
			secMonth, secDay sql.NullInt64
			adarPolicy       string
		)
		err := tx.QueryRowContext(ctx, `
SELECT e.year, e.month, e.day, e.secondary_calendar_type, e.secondary_month, e.secondary_day, u.adar_policy
FROM events e JOIN users u ON u.id = e.user_id
WHERE e.id = ? AND e.deleted_at IS NULL AND e.event_type = 'birthday'
  AND e.calendar_type = 'hebrew' AND e.year IS NOT NULL`, f.EventID).
			Scan(&year, &month, &day, &secType, &secMonth, &secDay, &adarPolicy)
		if err == sql.ErrNoRows {
			res.Skipped = append(res.Skipped, f.EventID)
			continue
		}
		if err != nil {
			return res, fmt.Errorf("event %d: %w", f.EventID, err)
		}
		asReported := year == f.BeforeYear && month == f.HebMonth && day == f.HebDay &&
			secMonth.Valid && secDay.Valid && int(secMonth.Int64) == f.SecMonth && int(secDay.Int64) == f.SecDay
		if !asReported || classifyHebrewYear(year, month, day, secType, secMonth, secDay, adarPolicy) != ClassProvable {
			res.Skipped = append(res.Skipped, f.EventID)
			continue
		}
		r, err := tx.ExecContext(ctx, `UPDATE events SET year = ?, updated_at = ? WHERE id = ? AND year = ? AND calendar_type = 'hebrew'`,
			year+1, now, f.EventID, year)
		if err != nil {
			return res, fmt.Errorf("event %d: %w", f.EventID, err)
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return res, fmt.Errorf("event %d changed while repairing", f.EventID)
		}
		res.Applied++
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, err
	}
	return res, nil
}
