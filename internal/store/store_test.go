package store

import (
	"path/filepath"
	"testing"
)

func TestOpen_RunsMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "birthly_test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	tables := []string{
		"app_meta", "users", "audit_logs", "backups", "events",
		"reminder_rules", "notifications_log", "greeting_templates",
	}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM greeting_templates WHERE user_id IS NULL").Scan(&count); err != nil {
		t.Fatalf("counting seed templates: %v", err)
	}
	if count != 185 {
		t.Errorf("expected 185 seeded system templates, got %d", count)
	}

	var schemaVersion string
	if err := db.QueryRow("SELECT value FROM app_meta WHERE key='schema_version'").Scan(&schemaVersion); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if schemaVersion != "1" {
		t.Errorf("expected schema_version=1, got %q", schemaVersion)
	}

	// Re-opening (re-running migrations) against the same DB must stay idempotent.
	db.Close()
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()
	var count2 int
	if err := db2.QueryRow("SELECT COUNT(*) FROM greeting_templates WHERE user_id IS NULL").Scan(&count2); err != nil {
		t.Fatalf("counting seed templates after reopen: %v", err)
	}
	if count2 != 185 {
		t.Errorf("expected 185 seeded system templates after reopen, got %d", count2)
	}
}
