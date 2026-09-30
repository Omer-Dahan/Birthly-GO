// Command diag-reminders is a read-only report on why a reminder was (or was
// not) sent: for each notifications_log row in a date window it shows the
// event, the rule that fired (offset, send time, global or per-event), which
// calendar track of a dual-date event it belongs to, the real days-before
// gap between the send and the occurrence, and how late the send was
// relative to its scheduled time. It also lists the user's current rules and
// a per-day status count so a downtime window shows up as failed/skipped
// rows.
//
// It opens the database with mode=ro plus PRAGMA query_only and never runs
// migrations, so it is safe to run against the live file while the bot is
// running. Pure Go (modernc.org/sqlite): the sqlite3 CLI is not needed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db-path", "data/birthly.db", "path to the SQLite database")
	userID := flag.Int64("user-id", 0, "Telegram user id to report on (0 = every user with a log row in the window)")
	since := flag.String("since", time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"), "first scheduled_at day (UTC, YYYY-MM-DD) to include")
	statusDays := flag.Int("status-days", 21, "days of per-day status counts to show")
	backupDir := flag.String("backup-dir", "data/backups", "backup directory, checked for dual-dates backfill snapshots")
	flag.Parse()

	sinceDay, err := time.Parse("2006-01-02", *since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: --since: %v\n", err)
		os.Exit(2)
	}

	db, err := openReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	opts := options{UserID: *userID, Since: sinceDay, StatusDays: *statusDays, BackupDir: *backupDir, Now: time.Now().UTC()}
	if err := report(context.Background(), db, opts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func openReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("database %s: %w", path, err)
	}
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA query_only=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("applying %q: %w", p, err)
		}
	}
	return db, nil
}

type options struct {
	UserID     int64
	Since      time.Time
	StatusDays int
	BackupDir  string
	Now        time.Time
}

type userRow struct {
	ID                   int64
	FirstName            string
	Username             sql.NullString
	Timezone             string
	DefaultNotifyTime    string
	NotificationsEnabled bool
	ShowHebrewDate       bool
	CreatedAt            time.Time
	loc                  *time.Location
}

func report(ctx context.Context, db *sql.DB, o options, w io.Writer) error {
	fmt.Fprintf(w, "== diag-reminders (read-only), now %s UTC\n", o.Now.Format("2006-01-02 15:04:05"))
	if err := printMeta(ctx, db, w); err != nil {
		return err
	}
	printBackfillSnapshots(o.BackupDir, w)

	userIDs := []int64{o.UserID}
	if o.UserID == 0 {
		ids, err := usersWithLogsSince(ctx, db, o.Since)
		if err != nil {
			return err
		}
		userIDs = ids
		if len(userIDs) == 0 {
			fmt.Fprintf(w, "\nno notifications_log rows scheduled since %s\n", o.Since.Format("2006-01-02"))
		}
	}

	for _, id := range userIDs {
		u, err := loadUser(ctx, db, id)
		if err != nil {
			return err
		}
		if u == nil {
			fmt.Fprintf(w, "\nuser %d: not found\n", id)
			continue
		}
		printUser(u, w)
		if err := printRules(ctx, db, u, w); err != nil {
			return err
		}
		if err := printLogs(ctx, db, u, o.Since, w); err != nil {
			return err
		}
		if err := printDualDateEvents(ctx, db, u, w); err != nil {
			return err
		}
	}

	return printStatusByDay(ctx, db, o, w)
}

func printMeta(ctx context.Context, db *sql.DB, w io.Writer) error {
	var lastTick sql.NullString
	err := db.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = 'last_tick_at'`).Scan(&lastTick)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("reading app_meta: %w", err)
	}
	fmt.Fprintf(w, "last_tick_at (UTC): %s\n", orDash(lastTick))
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err == nil && version.Valid {
		fmt.Fprintf(w, "schema version: %d\n", version.Int64)
	}
	return nil
}

func printBackfillSnapshots(dir string, w io.Writer) {
	matches, _ := filepath.Glob(filepath.Join(dir, "birthly_backfill_*.db"))
	if len(matches) == 0 {
		fmt.Fprintf(w, "dual-dates backfill snapshots in %s: none found\n", dir)
		return
	}
	sort.Strings(matches)
	fmt.Fprintf(w, "dual-dates backfill snapshots in %s (backfill --apply ran):\n", dir)
	for _, m := range matches {
		fmt.Fprintf(w, "  %s\n", filepath.Base(m))
	}
}

func usersWithLogsSince(ctx context.Context, db *sql.DB, since time.Time) ([]int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT user_id FROM notifications_log WHERE scheduled_at >= ? ORDER BY user_id`,
		since.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadUser(ctx context.Context, db *sql.DB, id int64) (*userRow, error) {
	var u userRow
	err := db.QueryRowContext(ctx,
		`SELECT id, first_name, username, timezone, default_notify_time, notifications_enabled, show_hebrew_date, created_at
		 FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.FirstName, &u.Username, &u.Timezone, &u.DefaultNotifyTime, &u.NotificationsEnabled, &u.ShowHebrewDate, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading user %d: %w", id, err)
	}
	u.loc, err = time.LoadLocation(u.Timezone)
	if err != nil {
		u.loc = time.UTC
	}
	return &u, nil
}

func printUser(u *userRow, w io.Writer) {
	fmt.Fprintf(w, "\n== user %d (%s", u.ID, u.FirstName)
	if u.Username.Valid && u.Username.String != "" {
		fmt.Fprintf(w, ", @%s", u.Username.String)
	}
	fmt.Fprintf(w, ")\n")
	fmt.Fprintf(w, "timezone=%s default_notify_time=%s notifications_enabled=%t show_hebrew_date=%t created_at=%s UTC\n",
		u.Timezone, u.DefaultNotifyTime, u.NotificationsEnabled, u.ShowHebrewDate, u.CreatedAt.Format("2006-01-02 15:04"))
}

func printRules(ctx context.Context, db *sql.DB, u *userRow, w io.Writer) error {
	rows, err := db.QueryContext(ctx,
		`SELECT r.id, r.event_id, e.first_name, r.offset_days, r.offset_minutes, r.send_time, r.enabled, r.created_at
		 FROM reminder_rules r LEFT JOIN events e ON e.id = r.event_id
		 WHERE r.user_id = ? ORDER BY r.event_id IS NOT NULL, r.event_id, r.offset_days`, u.ID)
	if err != nil {
		return fmt.Errorf("listing rules: %w", err)
	}
	defer rows.Close()

	fmt.Fprintf(w, "\n-- reminder rules (all, including disabled)\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "rule\tscope\toffset_days\toffset_min\tsend_time\tenabled\tcreated_at(UTC)")
	for rows.Next() {
		var (
			id                  int64
			eventID             sql.NullInt64
			eventName           sql.NullString
			offDays, offMinutes sql.NullInt64
			sendTime            sql.NullString
			enabled             bool
			createdAt           time.Time
		)
		if err := rows.Scan(&id, &eventID, &eventName, &offDays, &offMinutes, &sendTime, &enabled, &createdAt); err != nil {
			return err
		}
		scope := "global"
		if eventID.Valid {
			scope = fmt.Sprintf("event %d (%s)", eventID.Int64, eventName.String)
		}
		st := u.DefaultNotifyTime + " (user default)"
		if sendTime.Valid && sendTime.String != "" {
			st = sendTime.String
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%t\t%s\n", id, scope, intOrDash(offDays), intOrDash(offMinutes), st, enabled, createdAt.Format("2006-01-02 15:04"))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tw.Flush()
}

func printLogs(ctx context.Context, db *sql.DB, u *userRow, since time.Time, w io.Writer) error {
	rows, err := db.QueryContext(ctx,
		`SELECT n.id, n.status, n.attempts, n.error, n.occurrence_date, n.scheduled_at, n.sent_at,
		        n.rule_id, r.offset_days, r.offset_minutes, r.send_time, r.event_id, r.enabled,
		        e.id, e.first_name, e.last_name, e.calendar_type, e.month, e.day,
		        e.next_occurrence, e.secondary_month, e.secondary_day, e.secondary_next_occurrence
		 FROM notifications_log n
		 JOIN events e ON e.id = n.event_id
		 LEFT JOIN reminder_rules r ON r.id = n.rule_id
		 WHERE n.user_id = ? AND n.scheduled_at >= ?
		 ORDER BY n.scheduled_at, n.id`,
		u.ID, since.Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("listing notifications_log: %w", err)
	}
	defer rows.Close()

	fmt.Fprintf(w, "\n-- notifications_log since %s (times in %s)\n", since.Format("2006-01-02"), u.loc)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "log\tstatus\tscheduled(local)\tsent(local)\tlate_by\tevent\toccurrence\ttrack\tdays_before\trule\tnote")
	for rows.Next() {
		var (
			logID, attempts, eventID int64
			status                   string
			errText                  sql.NullString
			occ, scheduled           time.Time
			sentAt                   sql.NullTime
			ruleID, offDays, offMin  sql.NullInt64
			ruleEventID              sql.NullInt64
			sendTime                 sql.NullString
			ruleEnabled              sql.NullBool
			firstName, calendarType  string
			lastName                 sql.NullString
			month, day               int
			nextOcc, secNextOcc      sql.NullTime
			secMonth, secDay         sql.NullInt64
		)
		if err := rows.Scan(&logID, &status, &attempts, &errText, &occ, &scheduled, &sentAt,
			&ruleID, &offDays, &offMin, &sendTime, &ruleEventID, &ruleEnabled,
			&eventID, &firstName, &lastName, &calendarType, &month, &day,
			&nextOcc, &secMonth, &secDay, &secNextOcc); err != nil {
			return err
		}

		name := firstName
		if lastName.Valid && lastName.String != "" {
			name += " " + lastName.String
		}
		eventDesc := fmt.Sprintf("%d %s [%s %02d/%02d", eventID, name, calendarType, day, month)
		if secMonth.Valid && secDay.Valid {
			eventDesc += fmt.Sprintf(" + gregorian %02d/%02d", secDay.Int64, secMonth.Int64)
		}
		eventDesc += "]"

		track := "neither (stale)"
		switch {
		case nextOcc.Valid && nextOcc.Time.Equal(occ):
			track = "primary"
		case secNextOcc.Valid && secNextOcc.Time.Equal(occ):
			track = "secondary"
		}

		schedLocal := scheduled.In(u.loc)
		schedDay := time.Date(schedLocal.Year(), schedLocal.Month(), schedLocal.Day(), 0, 0, 0, 0, time.UTC)
		daysBefore := int(occ.Sub(schedDay).Hours() / 24)

		sent, lateBy := "-", "-"
		if sentAt.Valid {
			sent = sentAt.Time.In(u.loc).Format("01-02 15:04:05")
			lateBy = sentAt.Time.Sub(scheduled).Round(time.Second).String()
		}

		ruleDesc := "deleted (rule_id NULL)"
		var notes []string
		if ruleID.Valid {
			ruleDesc = fmt.Sprintf("%d", ruleID.Int64)
			switch {
			case offDays.Valid:
				ruleDesc += fmt.Sprintf(" days=%d", offDays.Int64)
				if int64(daysBefore) != offDays.Int64 {
					notes = append(notes, fmt.Sprintf("MISMATCH: fired %d days before, rule says %d", daysBefore, offDays.Int64))
				}
			case offMin.Valid:
				ruleDesc += fmt.Sprintf(" minutes=%d", offMin.Int64)
			default:
				ruleDesc += " (rule row missing)"
			}
			if sendTime.Valid && sendTime.String != "" {
				ruleDesc += " at " + sendTime.String
			}
			if ruleEventID.Valid {
				ruleDesc += " per-event"
			} else if offDays.Valid || offMin.Valid {
				ruleDesc += " global"
			}
			if ruleEnabled.Valid && !ruleEnabled.Bool {
				notes = append(notes, "rule now disabled")
			}
		}
		if errText.Valid && errText.String != "" {
			notes = append(notes, "error: "+truncate(errText.String, 60))
		}
		if attempts > 1 {
			notes = append(notes, fmt.Sprintf("attempts=%d", attempts))
		}

		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			logID, status, schedLocal.Format("01-02 15:04"), sent, lateBy, eventDesc,
			occ.Format("2006-01-02"), track, daysBefore, ruleDesc, strings.Join(notes, "; "))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tw.Flush()
}

func printDualDateEvents(ctx context.Context, db *sql.DB, u *userRow, w io.Writer) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, first_name, calendar_type, month, day, next_occurrence, secondary_month, secondary_day, secondary_next_occurrence, updated_at
		 FROM events
		 WHERE user_id = ? AND deleted_at IS NULL AND is_active = 1 AND secondary_month IS NOT NULL
		 ORDER BY COALESCE(MIN(next_occurrence, secondary_next_occurrence), next_occurrence)`, u.ID)
	if err != nil {
		return fmt.Errorf("listing dual-date events: %w", err)
	}
	defer rows.Close()

	fmt.Fprintf(w, "\n-- active dual-date events (each fires reminders on BOTH dates)\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "event\tname\tprimary\tnext_occurrence\tsecondary\tsecondary_next\tgap_days\tupdated_at(UTC)")
	n := 0
	for rows.Next() {
		var (
			id               int64
			name, calType    string
			month, day       int
			nextOcc, secOcc  sql.NullTime
			secMonth, secDay int
			updatedAt        time.Time
		)
		if err := rows.Scan(&id, &name, &calType, &month, &day, &nextOcc, &secMonth, &secDay, &secOcc, &updatedAt); err != nil {
			return err
		}
		gap := "-"
		if nextOcc.Valid && secOcc.Valid {
			gap = fmt.Sprintf("%d", int(secOcc.Time.Sub(nextOcc.Time).Hours()/24))
		}
		fmt.Fprintf(tw, "%d\t%s\t%s %02d/%02d\t%s\tgregorian %02d/%02d\t%s\t%s\t%s\n",
			id, name, calType, day, month, dateOrDash(nextOcc), secDay, secMonth, dateOrDash(secOcc), gap, updatedAt.Format("2006-01-02 15:04"))
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		fmt.Fprintln(tw, "(none)")
	}
	return tw.Flush()
}

func printStatusByDay(ctx context.Context, db *sql.DB, o options, w io.Writer) error {
	from := o.Now.AddDate(0, 0, -o.StatusDays)
	query := `SELECT substr(scheduled_at, 1, 10) AS day, status, COUNT(*)
		 FROM notifications_log WHERE scheduled_at >= ?`
	args := []any{from.Format("2006-01-02 15:04:05")}
	if o.UserID != 0 {
		query += ` AND user_id = ?`
		args = append(args, o.UserID)
	}
	query += ` GROUP BY day, status ORDER BY day, status`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("status counts: %w", err)
	}
	defer rows.Close()

	scope := "all users"
	if o.UserID != 0 {
		scope = fmt.Sprintf("user %d", o.UserID)
	}
	fmt.Fprintf(w, "\n-- notifications_log status per scheduled day (UTC), last %d days, %s\n", o.StatusDays, scope)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "day\tstatus\tcount")
	for rows.Next() {
		var day, status string
		var count int
		if err := rows.Scan(&day, &status, &count); err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\n", day, status, count)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tw.Flush()
}

func orDash(s sql.NullString) string {
	if !s.Valid || s.String == "" {
		return "-"
	}
	return s.String
}

func intOrDash(n sql.NullInt64) string {
	if !n.Valid {
		return "-"
	}
	return fmt.Sprintf("%d", n.Int64)
}

func dateOrDash(t sql.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Format("2006-01-02")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
