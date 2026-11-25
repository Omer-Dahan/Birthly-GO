package backfill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackup_CreatesSnapshotAndPrunes(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	backupDir := t.TempDir()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var last string
	for i := 0; i < 9; i++ {
		path, err := Backup(ctx, db, backupDir, base.Add(time.Duration(i)*time.Minute), 7, "birthly_backfill")
		if err != nil {
			t.Fatalf("Backup #%d: %v", i, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Backup #%d: snapshot not on disk: %v", i, err)
		}
		last = path
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if n := countDB(entries); n != 7 {
		t.Errorf("backup dir has %d snapshots, want 7 (retention)", n)
	}
	if _, err := os.Stat(last); err != nil {
		t.Errorf("newest snapshot should survive retention pruning: %v", err)
	}

	oldest := filepath.Join(backupDir, "birthly_backfill_20260101_120000.db")
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Errorf("oldest snapshot should have been pruned, stat err = %v", err)
	}
}

func TestBackup_DistinctPrefixesHaveIndependentRetention(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	backupDir := t.TempDir()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := Backup(ctx, db, backupDir, base.Add(time.Duration(i)*time.Minute), 1, "birthly_backfill"); err != nil {
			t.Fatalf("Backup (birthly_backfill) #%d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := Backup(ctx, db, backupDir, base.Add(time.Duration(i)*time.Minute), 1, "birthly_manual"); err != nil {
			t.Fatalf("Backup (birthly_manual) #%d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	// Retention of 1 for each prefix, applied independently, one survivor each.
	if n := countDB(entries); n != 2 {
		t.Errorf("backup dir has %d snapshots, want 2 (one survivor per prefix)", n)
	}
}

func TestBackup_NeverPrunesProtectedFile(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	dir := t.TempDir()
	// A file that matches the backup pattern and sorts oldest.
	input := filepath.Join(dir, "birthly_hebyear_20000101_000000.db")
	if err := os.WriteFile(input, []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		if _, err := Backup(ctx, db, dir, base.Add(time.Duration(i)*time.Minute), 2, "birthly_hebyear", input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatalf("protected input was pruned: %v", err)
	}
}

// countDB counts snapshot files, ignoring the ownership markers beside them.
func countDB(entries []os.DirEntry) int {
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".db" {
			n++
		}
	}
	return n
}

func TestBackup_MarkerSymlinkNotFollowed(t *testing.T) {
	db := openTestDB(t)
	backupDir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	marker := filepath.Join(backupDir, "birthly_backfill_20260101_120000.db"+ownerMarkerSuffix)
	if err := os.Symlink(victim, marker); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := Backup(context.Background(), db, backupDir, now, 7, "birthly_backfill"); err == nil {
		t.Fatal("Backup should refuse a symlink marker")
	}
	got, err := os.ReadFile(victim)
	if err != nil || string(got) != "precious" {
		t.Fatalf("symlink target modified: %q, %v", got, err)
	}
}

func TestBackup_PruneIgnoresForeignDBAndStaleMarker(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	backupDir := t.TempDir()

	foreign := filepath.Join(backupDir, "birthly_backfill_20200101_000000.db")
	if err := os.WriteFile(foreign, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := foreign + ownerMarkerSuffix
	if err := os.WriteFile(marker, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if _, err := Backup(ctx, db, backupDir, base.Add(time.Duration(i)*time.Minute), 2, "birthly_backfill"); err != nil {
			t.Fatalf("Backup #%d: %v", i, err)
		}
	}
	for _, p := range []string{foreign, marker} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should survive prune: %v", filepath.Base(p), err)
		}
	}
	entries, _ := os.ReadDir(backupDir)
	if n := countDB(entries); n != 3 { // 2 retained + foreign
		t.Errorf("got %d db files, want 3", n)
	}
}
