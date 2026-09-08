// Command backfill-dual-dates converts existing Gregorian-primary birthday
// events into dual-date events (Hebrew primary + Gregorian secondary) for
// users who have ShowHebrewDate enabled, anchoring the Hebrew date to the
// real birth date. Defaults to a dry-run; pass --apply to write. See
// deploy/DUAL_DATES_BACKFILL.md for the operator runbook.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"birthly/internal/backfill"
	"birthly/internal/store"
)

// backupRetention is how many of this tool's own snapshots to keep in
// backup-dir; matches the bot's default BACKUP_RETENTION_DAYS-driven count.
const backupRetention = 7

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db-path", "data/birthly.db", "path to the SQLite database")
	apply := flag.Bool("apply", false, "write the computed changes (default is dry-run)")
	backupDir := flag.String("backup-dir", "data/backups", "directory for the pre-apply backup snapshot")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("opening database at %s: %w", *dbPath, err)
	}
	defer db.Close()

	ctx := context.Background()
	changes, err := backfill.Plan(ctx, db)
	if err != nil {
		return fmt.Errorf("planning backfill: %w", err)
	}

	printPlan(changes)

	if len(changes) == 0 {
		fmt.Println("\nnothing to do")
		return nil
	}

	if !*apply {
		fmt.Printf("\n%d event(s) would change. Re-run with --apply to write them.\n", len(changes))
		return nil
	}

	backupPath, err := backfill.Backup(ctx, db, *backupDir, time.Now(), backupRetention, "birthly_backfill")
	if err != nil {
		return fmt.Errorf("backup before apply (aborting, nothing written): %w", err)
	}
	fmt.Printf("\nbackup written to %s\n", backupPath)

	if err := backfill.Apply(ctx, db, changes); err != nil {
		return fmt.Errorf("applying backfill (transaction rolled back, nothing written): %w", err)
	}

	fmt.Printf("applied %d event(s):\n", len(changes))
	for _, c := range changes {
		fmt.Printf("  event %d (user %d, %s): hebrew %d/%d/%d, secondary gregorian %d/%d, next=%s secondary_next=%s\n",
			c.EventID, c.UserID, c.Name, c.AfterYear, c.AfterMonth, c.AfterDay,
			c.SecondaryMonth, c.SecondaryDay,
			c.NextOccurrence.Format("2006-01-02"), c.SecondaryNextOccurrence.Format("2006-01-02"))
	}
	return nil
}

func printPlan(changes []backfill.Change) {
	if len(changes) == 0 {
		fmt.Println("no eligible events found")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "EVENT\tUSER\tNAME\tBEFORE (gregorian)\tAFTER (hebrew + secondary gregorian)")
	for _, c := range changes {
		before := fmt.Sprintf("%04d-%02d-%02d", c.BeforeYear, c.BeforeMonth, c.BeforeDay)
		after := fmt.Sprintf("hebrew %d/%d/%d + secondary %d/%d",
			c.AfterYear, c.AfterMonth, c.AfterDay, c.SecondaryMonth, c.SecondaryDay)
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\n", c.EventID, c.UserID, c.Name, before, after)
	}
	w.Flush()
}
