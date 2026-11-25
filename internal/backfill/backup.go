package backfill

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
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

	// Track the snapshot in memory; pruning only ever deletes files this run created.
	createdMu.Lock()
	createdThisRun[absDest] = struct{}{}
	createdMu.Unlock()

	// The marker is documentation only; it never authorizes a delete.
	ours, err := writeMarker(backupDir, dest+ownerMarkerSuffix)
	if err != nil {
		return "", fmt.Errorf("writing backup marker: %w", err)
	}
	if !ours {
		slog.Warn("backup marker path holds a file not written by this tool; left untouched, backups are not pruned",
			"marker", dest+ownerMarkerSuffix)
		return dest, nil
	}

	pruneBackups(backupDir, retention, prefix, protect)
	return dest, nil
}

var (
	createdMu      sync.Mutex
	createdThisRun = map[string]struct{}{}
)

// ownerMarkerSuffix names the documentation sidecar written next to each
// snapshot. It is never consulted when deciding what to delete.
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

const markerBody = "created by birthly backup\n"

// writeMarker writes the marker atomically (temp file in the same directory,
// then rename) and never deletes the target path. It reports whether the
// marker is ours: false means a foreign file already sits at the path and was
// left untouched.
func writeMarker(backupDir, marker string) (bool, error) {
	if err := checkMarkerPath(backupDir, marker); err != nil {
		return false, err
	}
	if existing, err := os.ReadFile(marker); err == nil {
		if string(existing) != markerBody {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	tmp, err := os.CreateTemp(backupDir, ".birthly-marker-*")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(markerBody); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return false, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return false, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return false, err
	}
	if err := os.Rename(tmpName, marker); err != nil {
		os.Remove(tmpName)
		return false, err
	}
	return true, nil
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
		// Only files this run created (tracked in memory) are ours.
		abs, err := filepath.Abs(filepath.Join(backupDir, e.Name()))
		if err != nil {
			continue
		}
		createdMu.Lock()
		_, ours := createdThisRun[abs]
		createdMu.Unlock()
		if !ours {
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
		if os.Remove(path) == nil {
			if abs, err := filepath.Abs(path); err == nil {
				createdMu.Lock()
				delete(createdThisRun, abs)
				createdMu.Unlock()
			}
			os.Remove(path + ownerMarkerSuffix)
		}
	}
}
