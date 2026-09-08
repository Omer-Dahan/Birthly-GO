package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// backupNamePattern matches only this tool's own snapshots, so pruning never
// touches the bot's separate scheduled auto-backups sharing the same directory.
var backupNamePattern = regexp.MustCompile(`^birthly_backfill_\d{8}_\d{6}\.db$`)

// Backup creates a timestamped VACUUM INTO snapshot of db in backupDir and
// prunes older backfill snapshots beyond retention. VACUUM INTO is safe to
// run against a live database under WAL, unlike a raw file copy. Returns the
// created snapshot's path.
func Backup(ctx context.Context, db *sql.DB, backupDir string, now time.Time, retention int) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}

	stamp := now.UTC().Format("20060102_150405")
	dest := filepath.Join(backupDir, fmt.Sprintf("birthly_backfill_%s.db", stamp))
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}

	escaped := strings.ReplaceAll(absDest, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}

	pruneBackups(backupDir, retention)
	return dest, nil
}

func pruneBackups(backupDir string, retention int) {
	if retention <= 0 {
		return
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if backupNamePattern.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	// Timestamp-named (YYYYMMDD_HHMMSS), so lexicographic order is chronological.
	sort.Strings(names)
	if len(names) <= retention {
		return
	}
	for _, name := range names[:len(names)-retention] {
		os.Remove(filepath.Join(backupDir, name))
	}
}
