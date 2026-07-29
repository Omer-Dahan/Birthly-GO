// Package repo contains the data-access repositories, each scoped to a user ID.
package repo

import (
	"context"
	"database/sql"
	"time"
)

// DBTX is satisfied by both *sql.DB and *sql.Tx, so every repository can run
// either directly against the database or inside a caller-managed
// transaction (needed where Python relied on a single session.commit() to
// make a multi-statement sequence atomic — e.g. user_service.get_or_create_user
// creating a user row plus two default reminder rules in one commit).
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// dateLayout/datetimeLayout match exactly what SQLAlchemy writes for its
// Date and DateTime column types (see app/db/session.py and SPEC.md's known
// pitfall: SQLite has no real date type, so the Go driver must read and
// write these same text formats, not Unix timestamps).
//
// modernc.org/sqlite auto-converts DATE/DATETIME columns to time.Time on
// read regardless of which of these two layouts (or one with a fractional
// seconds suffix) produced the stored text, always yielding a UTC-located
// time.Time. Writing must NOT rely on that same magic in reverse: binding a
// time.Time arg directly stores Go's verbose "2006-01-02 15:04:05 -0700 MST"
// form, which SQLAlchemy/Python cannot parse. Every write goes through
// formatDate/formatDateTime instead, so both sides ever store is the plain
// SQLAlchemy text form, in UTC.
const (
	dateLayout     = "2006-01-02"
	datetimeLayout = "2006-01-02 15:04:05"
)

func formatDate(t time.Time) string {
	return t.UTC().Format(dateLayout)
}

func formatDateTime(t time.Time) string {
	return t.UTC().Format(datetimeLayout)
}

// formatDatePtr/formatDateTimePtr return a driver-ready value for a nullable
// date/datetime arg: nil (SQL NULL) if ptr is nil, else the formatted text.
func formatDatePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatDate(*t)
}

func formatDateTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatDateTime(*t)
}
