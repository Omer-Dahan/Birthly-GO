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
	"path/filepath"
	"strings"
	"time"

	"birthly/internal/backfill"
	"birthly/internal/store"
)

const backupRetention = 7

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("repair-hebrew-birth-year", flag.ContinueOnError)
	dbPath := fs.String("db-path", "data/birthly.db", "path to the SQLite database")
	apply := fs.Bool("apply", false, "write the repairs (default is dry-run)")
	backupDir := fs.String("backup-dir", "data/backups", "directory for the pre-apply backup snapshot")
	confirm := fs.Int("confirm-count", -1, "with --apply: the number of events the dry-run reported (must match the plan exactly)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Friendly early message; OpenExisting below is the real guard (it cannot
	// create a file, so a path that vanishes after this check still fails).
	if st, err := os.Stat(*dbPath); err != nil {
		return fmt.Errorf("database %s does not exist or is unreadable (refusing to create one): %w", *dbPath, err)
	} else if st.IsDir() {
		return fmt.Errorf("database path %s is a directory", *dbPath)
	}

	if *apply {
		if err := checkBackupDir(*dbPath, *backupDir); err != nil {
			return err
		}
	}

	db, err := store.OpenExisting(*dbPath)
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

	if *confirm != len(rep.Fixes) {
		return fmt.Errorf("--apply needs --confirm-count %d (the number of events in the plan above); got %d, nothing written", len(rep.Fixes), *confirm)
	}

	backupPath, err := backfill.Backup(ctx, db, *backupDir, time.Now(), backupRetention, "birthly_hebyear", *dbPath)
	if err != nil {
		return fmt.Errorf("backup before apply (aborting, nothing written): %w", err)
	}
	fmt.Printf("\nbackup written to %s\n", backupPath)
	res, err := backfill.ApplyHebrewYearRepair(ctx, db, rep.Fixes)
	if err != nil {
		return fmt.Errorf("applying repair (transaction rolled back, nothing written): %w", err)
	}
	fmt.Printf("applied %d event(s), skipped %d no longer provable %v\n", res.Applied, len(res.Skipped), res.Skipped)
	return nil
}

// checkBackupDir refuses a backup directory that is, or sits inside, the
// directory holding the database, so the input can never be pruned or
// overwritten by backup rotation.
func checkBackupDir(dbPath, backupDir string) error {
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(absDB); err == nil {
		absDB = real
	}
	absBk, err := filepath.Abs(backupDir)
	if err != nil {
		return err
	}
	// Resolve the deepest existing ancestor so a not-yet-created dir is judged by where it will land.
	rest := ""
	probe := absBk
	for {
		if real, err := filepath.EvalSymlinks(probe); err == nil {
			absBk = filepath.Join(real, rest)
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		rest = filepath.Join(filepath.Base(probe), rest)
		probe = parent
	}
	rel, err := filepath.Rel(absBk, absDB)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("backup dir %s contains the database %s; choose a different --backup-dir", backupDir, dbPath)
	}
	return nil
}
