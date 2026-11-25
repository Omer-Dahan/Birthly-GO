package main

import (
	"os"
	"path/filepath"
	"testing"
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
