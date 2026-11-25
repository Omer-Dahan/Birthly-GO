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
	"syscall"
	"time"
)

// Backup creates a timestamped VACUUM INTO snapshot of db in backupDir,
// named "<prefix>_<timestamp>.db", and prunes older snapshots sharing that
// same prefix beyond retention. Callers use distinct prefixes (e.g.
// "birthly_backfill" for this tool's own pre-apply safety backups vs.
// "birthly_manual" for operator-triggered snapshots) so the two retention
// pools never prune each other's backups. VACUUM INTO is safe to run against
// a live database under WAL, unlike a raw file copy. Returns the created
// snapshot's path. Files named in protect (e.g. the live database) are never
// pruned, even if they match the backup name pattern.
func Backup(ctx context.Context, db *sql.DB, backupDir string, now time.Time, retention int, prefix string, protect ...string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}

	stamp := now.UTC().Format("20060102_150405")
	dest := filepath.Join(backupDir, fmt.Sprintf("%s_%s.db", prefix, stamp))
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}

	// Refuse a bad marker path before creating any snapshot.
	if err := checkMarkerPath(backupDir, dest+ownerMarkerSuffix); err != nil {
		return "", err
	}

	escaped := strings.ReplaceAll(absDest, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}

	// Mark the snapshot as this tool's own; pruning only ever touches marked files.
	if err := writeMarker(backupDir, dest+ownerMarkerSuffix); err != nil {
		return "", fmt.Errorf("writing backup marker: %w", err)
	}

	pruneBackups(backupDir, retention, prefix, protect)
	return dest, nil
}

// ownerMarkerSuffix names the sidecar file that marks a snapshot as created
// by Backup. A database file without its marker is never pruned.
const ownerMarkerSuffix = ".birthly-backup"

// checkMarkerPath refuses a marker that would land outside backupDir or that
// already exists as anything other than a regular file (e.g. a symlink).
func checkMarkerPath(backupDir, marker string) error {
	if filepath.Clean(filepath.Dir(marker)) != filepath.Clean(backupDir) {
		return fmt.Errorf("backup marker %q is outside backup dir", marker)
	}
	fi, err := os.Lstat(marker)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("backup marker %q is not a regular file", marker)
	}
	return nil
}

// writeMarker writes the marker without ever following a symlink.
func writeMarker(backupDir, marker string) error {
	if err := checkMarkerPath(backupDir, marker); err != nil {
		return err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("created by birthly backup\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func pruneBackups(backupDir string, retention int, prefix string, protect []string) {
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
		if !pattern.MatchString(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		// Only snapshots carrying our marker (a regular file) are ours.
		if mi, err := os.Lstat(filepath.Join(backupDir, e.Name()+ownerMarkerSuffix)); err != nil || !mi.Mode().IsRegular() {
			continue
		}
		names = append(names, e.Name())
	}
	// Timestamp-named (YYYYMMDD_HHMMSS), so lexicographic order is chronological.
	sort.Strings(names)
	if len(names) <= retention {
		return
	}
	var guarded []os.FileInfo
	for _, p := range protect {
		if fi, err := os.Stat(p); err == nil {
			guarded = append(guarded, fi)
		}
	}
	for _, name := range names[:len(names)-retention] {
		path := filepath.Join(backupDir, name)
		if fi, err := os.Stat(path); err == nil {
			skip := false
			for _, g := range guarded {
				if os.SameFile(fi, g) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		os.Remove(path)
		os.Remove(path + ownerMarkerSuffix)
	}
}
