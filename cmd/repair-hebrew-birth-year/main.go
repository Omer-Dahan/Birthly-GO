// Command repair-hebrew-birth-year fixes Hebrew birth years stored one year
// too early by the old Gregorian-to-Hebrew conversion (births from Rosh
// Hashana to 31 December). Dry-run by default; --apply writes in one
// transaction after a backup. See deploy/HEBREW_BIRTH_YEAR_REPAIR.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"birthly/internal/backfill"
	"birthly/internal/store"
)

const backupRetention = 7

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db-path", "data/birthly.db", "path to the SQLite database")
	apply := flag.Bool("apply", false, "write the repairs (default is dry-run)")
	backupDir := flag.String("backup-dir", "data/backups", "directory for the pre-apply backup snapshot")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("opening database at %s: %w", *dbPath, err)
	}
	defer db.Close()

	ctx := context.Background()
	rep, err := backfill.PlanHebrewYearRepair(ctx, db)
	if err != nil {
		return err
	}

	fmt.Printf("provability signal: %s\n\n", backfill.HebrewYearSignal)
	for _, f := range rep.Fixes {
		fmt.Printf("  event %d (user %d, %s): hebrew %d/%d, year %d -> %d (secondary gregorian %d/%d)\n",
			f.EventID, f.UserID, f.Name, f.HebMonth, f.HebDay, f.BeforeYear, f.AfterYear, f.SecMonth, f.SecDay)
	}
	fmt.Printf("\nprovable (would fix):  %d\n", rep.Provable)
	fmt.Printf("already-correct:       %d (secondary date agrees with the stored year)\n", rep.Consistent)
	fmt.Printf("ambiguous (untouched): %d (of which %d have no secondary date)\n", rep.Ambiguous, rep.NoSecondary)
	fmt.Println("note: rows whose secondary date was auto-derived from the wrong year, and rows without a secondary date, cannot be proven from stored data and are not changed")

	if len(rep.Fixes) == 0 {
		fmt.Println("\nnothing to do")
		return nil
	}
	if !*apply {
		fmt.Printf("\n%d event(s) would change. Re-run with --apply to write them.\n", len(rep.Fixes))
		return nil
	}

	backupPath, err := backfill.Backup(ctx, db, *backupDir, time.Now(), backupRetention, "birthly_hebyear")
	if err != nil {
		return fmt.Errorf("backup before apply (aborting, nothing written): %w", err)
	}
	fmt.Printf("\nbackup written to %s\n", backupPath)
	if err := backfill.ApplyHebrewYearRepair(ctx, db, rep.Fixes); err != nil {
		return fmt.Errorf("applying repair (transaction rolled back, nothing written): %w", err)
	}
	fmt.Printf("applied %d event(s)\n", len(rep.Fixes))
	return nil
}
