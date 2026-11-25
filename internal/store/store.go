// Package store manages the SQLite connection, PRAGMAs, and embedded migrations.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"birthly/migrations"
)

// Open opens the SQLite database at dbPath, applies the same PRAGMAs as
// app/db/session.py's _apply_pragmas, and runs any pending goose migrations.
// The busy_timeout and single-writer connection limit exist because SQLite
// allows only one writer at a time; WAL mode lets readers proceed concurrently.
// OpenExisting opens a database that must already exist: the SQLite open mode
// is read-write without create, so a missing file is an error even if it
// vanishes right after a caller's own check. No directory is created and no
// migration is run.
func OpenExisting(dbPath string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Opaque: (&url.URL{Path: dbPath}).EscapedPath(), RawQuery: "mode=rw"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("applying %q: %w", p, err)
		}
	}
	// Force a real connection so a missing file fails here, not later.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return db, nil
}

func Open(dbPath string) (*sql.DB, error) {
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("applying %q: %w", p, err)
		}
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}
