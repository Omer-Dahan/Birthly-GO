// Command audit-hebrew-years prints a strictly read-only report over Hebrew
// birthdays that have a year. It opens the database query-only (no writes, no
// journal mode change, no migrations) and has no apply mode. Rows the old
// Gregorian-to-Hebrew conversion could not have moved are "trustworthy"; rows
// inside its window are "undecidable" and show both candidate dates.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"

	"birthly/internal/core"
	"birthly/internal/store"
)

const (
	classTrustworthy = "trustworthy"
	classUndecidable = "undecidable"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("audit-hebrew-years", flag.ContinueOnError)
	dbPath := fs.String("db-path", "data/birthly.db", "path to the SQLite database (opened read-only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if st, err := os.Stat(*dbPath); err != nil {
		return fmt.Errorf("database %s does not exist or is unreadable: %w", *dbPath, err)
	} else if st.IsDir() {
		return fmt.Errorf("database path %s is a directory", *dbPath)
	}
	db, err := store.OpenReadOnly(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return report(context.Background(), db, out)
}

const query = `
SELECT e.id, e.user_id, e.first_name, e.last_name, e.year, e.month, e.day, u.adar_policy
FROM events e
JOIN users u ON u.id = e.user_id
WHERE e.deleted_at IS NULL
  AND e.event_type = 'birthday'
  AND e.calendar_type = 'hebrew'
  AND e.year IS NOT NULL
ORDER BY e.id`

// classify reports whether the old conversion could have moved the stored
// year. It stored one year too early for births from Rosh Hashana to 31
// December, so a row is only at risk when the next year's date lands in
// Gregorian year (stored year - 3760). It returns both candidate dates.
func classify(year, month, day int, adarPolicy string) (class string, stored, next string, err error) {
	d1, err := core.ToGregorian(year, core.ResolveMonth(year, month, adarPolicy), day)
	if err != nil {
		return "", "", "", err
	}
	d2, err := core.ToGregorian(year+1, core.ResolveMonth(year+1, month, adarPolicy), day)
	if err != nil {
		return "", "", "", err
	}
	class = classTrustworthy
	if d2.Year() == year-3760 {
		class = classUndecidable
	}
	return class, d1.Format("2006-01-02"), d2.Format("2006-01-02"), nil
}

func report(ctx context.Context, db *sql.DB, out io.Writer) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying hebrew birthdays: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var (
			id, userID       int64
			first            string
			last             sql.NullString
			year, month, day int
			adarPolicy       string
		)
		if err := rows.Scan(&id, &userID, &first, &last, &year, &month, &day, &adarPolicy); err != nil {
			return fmt.Errorf("scanning event row: %w", err)
		}
		name := first
		if last.Valid && last.String != "" {
			name += " " + last.String
		}
		class, stored, next, err := classify(year, month, day, adarPolicy)
		if err != nil {
			fmt.Fprintf(out, "event %d user %d %q: hebrew %d/%d/%d: cannot convert: %v\n", id, userID, name, day, month, year, err)
			counts["error"]++
			continue
		}
		counts[class]++
		if class == classUndecidable {
			fmt.Fprintf(out, "event %d user %d %q: hebrew %d/%d/%d; gregorian %s; %s: candidates %s (stored year) or %s (next year)\n",
				id, userID, name, day, month, year, stored, class, stored, next)
		} else {
			fmt.Fprintf(out, "event %d user %d %q: hebrew %d/%d/%d; gregorian %s; %s\n",
				id, userID, name, day, month, year, stored, class)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nsummary: %s=%d %s=%d", classTrustworthy, counts[classTrustworthy], classUndecidable, counts[classUndecidable])
	if counts["error"] > 0 {
		fmt.Fprintf(out, " unconvertible=%d", counts["error"])
	}
	fmt.Fprintln(out)
	return nil
}
