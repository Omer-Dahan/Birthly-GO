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

// Backup creates a timestamped VACUUM INTO snapshot of db in backupDir,
// named "<prefix>_<timestamp>.db", and prunes older snapshots sharing that
// same prefix beyond retention. Callers use distinct prefixes (e.g.
// "birthly_backfill" for this tool's own pre-apply safety backups vs.
// "birthly_manual" for operator-triggered snapshots) so the two retention
// pools never prune each other's backups. VACUUM INTO is safe to run against
// a live database under WAL, unlike a raw file copy. Returns the created
// snapshot's path.
func Backup(ctx context.Context, db *sql.DB, backupDir string, now time.Time, retention int, prefix string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}

	stamp := now.UTC().Format("20060102_150405")
	dest := filepath.Join(backupDir, fmt.Sprintf("%s_%s.db", prefix, stamp))
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}

	escaped := strings.ReplaceAll(absDest, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}

	pruneBackups(backupDir, retention, prefix)
	return dest, nil
}

func pruneBackups(backupDir string, retention int, prefix string) {
	if retention <= 0 {
		return
	}
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `_\d{8}_\d{6}\.db$`)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if pattern.MatchString(e.Name()) {
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
