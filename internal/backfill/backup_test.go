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
		path, err := Backup(ctx, db, backupDir, base.Add(time.Duration(i)*time.Minute), 7)
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
	if len(entries) != 7 {
		t.Errorf("backup dir has %d files, want 7 (retention)", len(entries))
	}
	if _, err := os.Stat(last); err != nil {
		t.Errorf("newest snapshot should survive retention pruning: %v", err)
	}

	oldest := filepath.Join(backupDir, "birthly_backfill_20260101_120000.db")
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Errorf("oldest snapshot should have been pruned, stat err = %v", err)
	}
}
