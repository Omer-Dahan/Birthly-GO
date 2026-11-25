package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"birthly/internal/core"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// seedProvable creates a database at path holding one provably-short Hebrew year.
func seedProvable(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, _, err := repo.NewUserRepo(db).GetOrCreate(ctx, 4001, nil, "T", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_, hm, hd := core.ToHebrew(time.Date(1983, 11, 30, 0, 0, 0, 0, time.UTC))
	occ, err := core.NextOccurrence(core.CalendarTypeHebrew, hm, hd, time.Now().UTC(), core.AdarPolicyAdarII, core.Feb29PolicyFeb28)
	if err != nil {
		t.Fatal(err)
	}
	y := 5743
	e, err := repo.NewEventRepo(db, u.ID).Create(ctx, &models.Event{
		EventType: core.EventTypeBirthday, FirstName: "Yael", Category: core.CategoryOther,
		CalendarType: core.CalendarTypeHebrew, Year: &y, Month: hm, Day: hd, IsActive: true, NextOccurrence: &occ,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE events SET secondary_calendar_type='gregorian', secondary_month=11, secondary_day=30 WHERE id=?`, e.ID); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWithoutIDsFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	seedProvable(t, dbPath)
	bk := filepath.Join(t.TempDir(), "bk")
	if err := run([]string{"--db-path", dbPath, "--backup-dir", bk, "--apply"}); err == nil {
		t.Fatal("expected error without --ids")
	}
	if err := run([]string{"--db-path", dbPath, "--backup-dir", bk, "--apply", "--ids", "99"}); err == nil {
		t.Fatal("expected error naming a non-candidate id")
	}
	if _, err := os.Stat(bk); err == nil {
		t.Fatal("backup dir created before confirmation")
	}
	if err := run([]string{"--db-path", dbPath, "--backup-dir", bk, "--apply", "--ids", "1"}); err != nil {
		t.Fatalf("confirmed apply failed: %v", err)
	}
}

func TestApplySurvivesInputInsideBackupDir(t *testing.T) {
	dir := t.TempDir()
	// Named like a backup and oldest, so pruning would pick it first.
	dbPath := filepath.Join(dir, "birthly_hebyear_20000101_000000.db")
	seedProvable(t, dbPath)
	if err := run([]string{"--db-path", dbPath, "--backup-dir", dir, "--apply", "--ids", "1"}); err == nil {
		t.Fatal("expected refusal of a backup dir containing the database")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("input deleted: %v", err)
	}
}

func TestApplyInputSurvivesPruningInSeparateDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "birthly_hebyear_20000101_000000.db")
	seedProvable(t, dbPath)
	bk := t.TempDir()
	for i := 1; i <= backupRetention+2; i++ {
		name := filepath.Join(bk, "birthly_hebyear_2001010"+string(rune('0'+i%10))+"_000000.db")
		if err := os.WriteFile(name, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"--db-path", dbPath, "--backup-dir", bk, "--apply", "--ids", "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("input deleted: %v", err)
	}
}

func TestRunRefusesMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "typo.db")
	if err := run([]string{"--db-path", missing}); err == nil {
		t.Fatal("expected error for a missing database")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("files were created: %v", entries)
	}
}

func TestOpenExistingNeverCreates(t *testing.T) {
	dir := t.TempDir()
	if db, err := store.OpenExisting(filepath.Join(dir, "gone.db")); err == nil {
		db.Close()
		t.Fatal("expected error opening a missing database")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("files were created: %v", entries)
	}
}

func TestForeignDatabaseInBackupDirSurvives(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	seedProvable(t, dbPath)
	bk := t.TempDir()
	foreign := filepath.Join(bk, "birthly_hebyear_20000101_000000.db")
	if err := os.WriteFile(foreign, []byte("someone else's database"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= backupRetention+2; i++ {
		name := filepath.Join(bk, "birthly_hebyear_2001010"+string(rune('0'+i%10))+"_000000.db")
		if err := os.WriteFile(name, []byte("foreign"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"--db-path", dbPath, "--backup-dir", bk, "--apply", "--ids", "1"}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(foreign); err != nil || string(b) != "someone else's database" {
		t.Fatalf("foreign database was touched: %v", err)
	}
}
