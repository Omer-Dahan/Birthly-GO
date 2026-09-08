// Package backfill converts existing Gregorian-primary birthday events into
// dual-date events (Hebrew primary + Gregorian secondary) for users who have
// ShowHebrewDate enabled. The Hebrew date is anchored to the real birth date
// (year/month/day), the same way services.hebrewEquivalentDate computes the
// card's Hebrew-equivalent line — not to this year's Gregorian occurrence.
package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store/models"
)

// dateLayout matches repo's SQLite Date column format (SPEC: SQLite has no
// real date type, so writes must use this exact text form, not Go's verbose
// time.Time string, or Python/SQLAlchemy reads on the shared DB break).
const dateLayout = "2006-01-02"
const datetimeLayout = "2006-01-02 15:04:05"

// Change describes one event's computed before/after state. Plan produces
// these without writing; Apply persists them.
type Change struct {
	EventID int64
	UserID  int64
	Name    string

	BeforeYear  int
	BeforeMonth int
	BeforeDay   int

	// AfterYear/Month/Day are the event's new primary (Hebrew) date: the
	// Hebrew year/month/day the person was actually born on.
	AfterYear  int
	AfterMonth int
	AfterDay   int

	// SecondaryMonth/Day are the original Gregorian birth month/day,
	// preserved as the event's new secondary date.
	SecondaryMonth int
	SecondaryDay   int

	NextOccurrence          time.Time
	SecondaryNextOccurrence time.Time
}

type eligibleRow struct {
	EventID     int64
	UserID      int64
	FirstName   string
	LastName    sql.NullString
	Year        int
	Month       int
	Day         int
	Timezone    string
	AdarPolicy  string
	Feb29Policy string
}

// eligibleQuery selects active, non-deleted, Gregorian-primary events with a
// known birth year, belonging to users who opted into ShowHebrewDate, that
// don't already carry a secondary date. Re-running the tool after a
// successful Apply naturally returns nothing here, since those events are
// no longer calendar_type='gregorian' — that's the idempotency guarantee.
const eligibleQuery = `
SELECT e.id, e.user_id, e.first_name, e.last_name, e.year, e.month, e.day,
       u.timezone, u.adar_policy, u.feb29_policy
FROM events e
JOIN users u ON u.id = e.user_id
WHERE e.deleted_at IS NULL
  AND e.is_active = 1
  AND e.calendar_type = 'gregorian'
  AND e.year IS NOT NULL
  AND e.secondary_month IS NULL
  AND e.secondary_day IS NULL
  AND e.secondary_calendar_type IS NULL
  AND u.show_hebrew_date = 1
ORDER BY e.id
`

// Plan reads every event eligible for the dual-date backfill and computes
// its converted state. It does not write anything.
func Plan(ctx context.Context, db *sql.DB) ([]Change, error) {
	rows, err := db.QueryContext(ctx, eligibleQuery)
	if err != nil {
		return nil, fmt.Errorf("querying eligible events: %w", err)
	}
	defer rows.Close()

	var out []Change
	for rows.Next() {
		var r eligibleRow
		if err := rows.Scan(&r.EventID, &r.UserID, &r.FirstName, &r.LastName, &r.Year, &r.Month, &r.Day,
			&r.Timezone, &r.AdarPolicy, &r.Feb29Policy); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		change, err := computeChange(r)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", r.EventID, err)
		}
		out = append(out, change)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func computeChange(r eligibleRow) (Change, error) {
	birth := time.Date(r.Year, time.Month(r.Month), r.Day, 0, 0, 0, 0, time.UTC)
	hebYear, hebMonth, hebDay := core.ToHebrew(birth)

	// UserToday only reads user.Timezone; building a bare User with just
	// that field avoids needing a full user fetch through the services layer.
	today := services.UserToday(&models.User{Timezone: r.Timezone})

	nextOcc, err := core.NextOccurrence(core.CalendarTypeHebrew, hebMonth, hebDay, today, r.AdarPolicy, r.Feb29Policy)
	if err != nil {
		return Change{}, fmt.Errorf("computing hebrew next occurrence: %w", err)
	}
	secOcc, err := core.NextOccurrence(core.CalendarTypeGregorian, r.Month, r.Day, today, r.AdarPolicy, r.Feb29Policy)
	if err != nil {
		return Change{}, fmt.Errorf("computing gregorian secondary occurrence: %w", err)
	}

	name := r.FirstName
	if r.LastName.Valid && r.LastName.String != "" {
		name += " " + r.LastName.String
	}

	return Change{
		EventID: r.EventID, UserID: r.UserID, Name: name,
		BeforeYear: r.Year, BeforeMonth: r.Month, BeforeDay: r.Day,
		AfterYear: hebYear, AfterMonth: hebMonth, AfterDay: hebDay,
		SecondaryMonth: r.Month, SecondaryDay: r.Day,
		NextOccurrence:          nextOcc,
		SecondaryNextOccurrence: secOcc,
	}, nil
}

// Apply writes every change to the database inside a single transaction,
// rolling back entirely if any event fails.
func Apply(ctx context.Context, db *sql.DB, changes []Change) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(datetimeLayout)
	for _, c := range changes {
		_, err := tx.ExecContext(ctx, `
			UPDATE events
			SET calendar_type = ?, year = ?, month = ?, day = ?,
			    secondary_calendar_type = ?, secondary_month = ?, secondary_day = ?,
			    next_occurrence = ?, secondary_next_occurrence = ?,
			    updated_at = ?
			WHERE id = ?`,
			core.CalendarTypeHebrew, c.AfterYear, c.AfterMonth, c.AfterDay,
			core.CalendarTypeGregorian, c.SecondaryMonth, c.SecondaryDay,
			c.NextOccurrence.Format(dateLayout), c.SecondaryNextOccurrence.Format(dateLayout),
			now, c.EventID,
		)
		if err != nil {
			return fmt.Errorf("event %d: %w", c.EventID, err)
		}
	}
	return tx.Commit()
}
