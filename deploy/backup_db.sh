#!/usr/bin/env bash
# Point-in-time snapshot of the live SQLite DB via VACUUM INTO. Safe to run
# against a database that's being written concurrently under WAL mode
# (unlike a raw file copy, which can grab a torn/inconsistent read).
# Keeps the newest N snapshots in the backup dir, deletes the rest.
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

mkdir -p "$BACKUP_DIR"

STAMP="$(date -u +%Y%m%d_%H%M%S)"
DEST="$BACKUP_DIR/birthly_manual_${STAMP}.db"

sqlite3 "$DB_PATH" "VACUUM INTO '${DEST}'"

# Retention: keep the newest $RETENTION snapshots produced by this script.
mapfile -t snapshots < <(find "$BACKUP_DIR" -maxdepth 1 -name 'birthly_manual_*.db' | sort)
count=${#snapshots[@]}
if (( count > RETENTION )); then
  for ((i = 0; i < count - RETENTION; i++)); do
    rm -f "${snapshots[$i]}"
  done
fi

echo "$DEST"
