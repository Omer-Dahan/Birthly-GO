package repo

import (
	"context"
	"database/sql"
)

// AppMetaRepo is a small key-value store for scheduler/admin bookkeeping
// (schema_version, last_auto_backup, last_tick_at, ...). Not user-scoped.
type AppMetaRepo struct {
	db DBTX
}

func NewAppMetaRepo(db DBTX) *AppMetaRepo {
	return &AppMetaRepo{db: db}
}

// Get returns the value for key, or (nil, nil) if the key doesn't exist.
func (r *AppMetaRepo) Get(ctx context.Context, key string) (*string, error) {
	var value *string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return value, err
}

// Set upserts key -> value.
func (r *AppMetaRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO app_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
