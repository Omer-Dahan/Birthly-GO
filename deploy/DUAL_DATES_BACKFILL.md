# Dual-dates backfill runbook

One-time operation: converts existing Gregorian-primary birthday events into
dual-date events (Hebrew primary + Gregorian secondary) for users who already
have the "show Hebrew date" setting on. Their birthday card will then also
carry a real Hebrew-calendar reminder anchored to their actual Hebrew birth
date, instead of just the yearly Hebrew-equivalent line shown on the card.

Only touches events that are:
- not deleted, not muted (`is_active = 1`)
- Gregorian (`calendar_type = 'gregorian'`)
- have a known birth year
- don't already have a secondary date
- owned by a user with `show_hebrew_date = 1`

Everything else (users without the setting, events with no birth year,
already-Hebrew events, trashed/muted events) is left untouched.

Tool: `cmd/backfill-dual-dates`. Backup script: `deploy/backup_db.sh`.

## 1. Deploy the new version

The binary embeds migration `00003_dual_calendar_dates.sql` (adds the
`secondary_*` columns) and runs it automatically on startup, same as any
other deploy:

```bash
git pull
./deploy/update.sh
```

Confirm the bot is up and the columns exist before continuing:

```bash
sqlite3 /opt/birthly-go/data/birthly.db "PRAGMA table_info(events)" | grep secondary
```

## 2. Stop the bot before touching the DB

The backfill tool runs a multi-row transaction directly against the SQLite
file. Do it while the bot isn't also writing to avoid `SQLITE_BUSY` retries
under load:

```bash
sudo systemctl stop birthly-go
```

## 3. Take a backup

The backfill tool creates its own timestamped backup automatically before
`--apply` (see step 5) — this manual step is for an extra, independent
snapshot before you start poking at anything:

```bash
cd /opt/birthly-go
./deploy/backup_db.sh data/birthly.db data/backups
```

Prints the created snapshot's path. Keeps the newest 7 by default
(`BACKUP_RETENTION` env var to change).

## 4. Build the tool

The server has no Go toolchain installed (`deploy/install.sh` only ever
copies a pre-built binary). Build locally, same as `deploy/update.sh` does
for the bot itself, and copy it over:

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o backfill-dual-dates ./cmd/backfill-dual-dates
scp backfill-dual-dates <server>:/opt/birthly-go/backfill-dual-dates
```

## 5. Dry run

```bash
cd /opt/birthly-go
./backfill-dual-dates --db-path data/birthly.db
```

Prints a table of every event that would change: id, user id, name, the
current Gregorian date, and the computed Hebrew primary + Gregorian secondary
date, followed by a total count. Writes nothing.

Review the table. Spot-check a few rows: the Hebrew date shown should match
the person's actual Hebrew birthday (e.g. a Nov 4, 2002 birth becomes
29 Cheshvan 5763), and the secondary date should be their original Gregorian
month/day.

## 6. Apply

```bash
./backfill-dual-dates --db-path data/birthly.db --apply
```

This will:
1. Create its own timestamped backup in `data/backups/birthly_backfill_<timestamp>.db`
   (via `VACUUM INTO`, safe under WAL) — aborts before touching anything if
   this fails.
2. Run every computed change in a single transaction — any single event
   failing rolls back the entire batch, so the DB is never left half-updated.
3. Print a log line per updated event.

## 7. Verify

```bash
sqlite3 data/birthly.db <<'SQL'
-- Should be 0: no gregorian-primary event left with a known year, no
-- secondary date, and an owner who wants the Hebrew date.
SELECT COUNT(*) FROM events e JOIN users u ON u.id = e.user_id
WHERE e.deleted_at IS NULL AND e.is_active = 1
  AND e.calendar_type = 'gregorian' AND e.year IS NOT NULL
  AND e.secondary_month IS NULL AND u.show_hebrew_date = 1;

-- Spot-check a converted event by name.
SELECT id, first_name, last_name, calendar_type, year, month, day,
       secondary_calendar_type, secondary_month, secondary_day,
       next_occurrence, secondary_next_occurrence
FROM events WHERE first_name = '<name>';
SQL
```

First query must return 0. Second query's `calendar_type` should now read
`hebrew`, with `secondary_calendar_type` = `gregorian` holding the original
birth month/day.

Also check the tool's own log output from step 6 for anything other than
successful `applied N event(s)` lines.

## 8. Restart the bot

```bash
sudo systemctl start birthly-go
sudo systemctl status birthly-go --no-pager
```

## Rollback

If something looks wrong after applying, stop the bot and restore the
pre-apply backup:

```bash
sudo systemctl stop birthly-go
cp data/backups/birthly_backfill_<timestamp>.db data/birthly.db
sudo systemctl start birthly-go
```

## Re-running

The tool is idempotent: once an event has been converted, it's no longer
`calendar_type = 'gregorian'`, so a second `--apply` run finds nothing left
to do and changes nothing. Safe to re-run the dry-run or apply at any time
to confirm the backfill is complete.
