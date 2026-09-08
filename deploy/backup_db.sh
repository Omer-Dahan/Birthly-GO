#!/usr/bin/env bash
# Point-in-time snapshot of the live SQLite DB via VACUUM INTO. Safe to run
# against a database that's being written concurrently under WAL mode
# (unlike a raw file copy, which can grab a torn/inconsistent read).
# Keeps the newest N snapshots in the backup dir, deletes the rest.
#
# Delegates to the backup-db Go binary (internal/backfill.Backup) instead of
# the sqlite3 CLI, since the server only runs the pure-Go modernc.org/sqlite
# driver and sqlite3 isn't guaranteed to be installed.
set -euo pipefail

DB_PATH="${1:-${DB_PATH:-data/birthly.db}}"
BACKUP_DIR="${2:-${BACKUP_DIR:-data/backups}}"
RETENTION="${BACKUP_RETENTION:-7}"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  echo "Usage: $0 [db_path] [backup_dir]" >&2
  echo "  positional args override the DB_PATH / BACKUP_DIR env vars" >&2
  echo "  BACKUP_RETENTION (default 7): number of snapshots to keep" >&2
  exit 0
fi

if [[ ! -f "$DB_PATH" ]]; then
  echo "error: database not found at $DB_PATH" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_BIN="$SCRIPT_DIR/../backup-db"

if [[ ! -x "$BACKUP_BIN" ]]; then
  echo "error: backup-db binary not found at $BACKUP_BIN" >&2
  echo "build it locally and copy it over, same as the backfill tool:" >&2
  echo "  GOOS=linux GOARCH=amd64 go build -ldflags=\"-s -w\" -o backup-db ./cmd/backup-db" >&2
  echo "  scp backup-db <server>:/opt/birthly-go/backup-db" >&2
  exit 1
fi

DEST="$("$BACKUP_BIN" --db-path "$DB_PATH" --backup-dir "$BACKUP_DIR" --retention "$RETENTION")"

echo "$DEST"
