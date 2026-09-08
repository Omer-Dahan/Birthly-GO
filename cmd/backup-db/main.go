// Command backup-db takes a point-in-time snapshot of the live SQLite
// database via VACUUM INTO (safe under WAL, unlike a raw file copy), using
// the same Go mechanism as the backfill tool's pre-apply backup. This avoids
// depending on the sqlite3 CLI, which isn't guaranteed to be installed on a
// server that only runs the pure-Go modernc.org/sqlite driver. Wrapped by
// deploy/backup_db.sh for manual snapshots; see deploy/DUAL_DATES_BACKFILL.md.
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

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db-path", "data/birthly.db", "path to the SQLite database")
	backupDir := flag.String("backup-dir", "data/backups", "directory for the backup snapshot")
	retention := flag.Int("retention", 7, "number of snapshots to keep")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("opening database at %s: %w", *dbPath, err)
	}
	defer db.Close()

	path, err := backfill.Backup(context.Background(), db, *backupDir, time.Now(), *retention, "birthly_manual")
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	fmt.Println(path)
	return nil
}
