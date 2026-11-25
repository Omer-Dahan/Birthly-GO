package main

import (
	"os"
	"path/filepath"
	"testing"

	"birthly/internal/store"
)

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
