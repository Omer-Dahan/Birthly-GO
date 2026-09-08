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

## A note on permissions

`/opt/birthly-go/data` is `chmod 700`, owned by the `birthly-go` service
user. Every command below that touches the database or backup directory runs
as `sudo -u birthly-go ...`. Running them as your own login (or root) either
fails with "permission denied" reading the DB, or, worse, succeeds but
leaves new files (backups, the DB itself if a raw copy is ever used) owned
by the wrong user, which then blocks the bot from reading or writing them on
its next start.

## 1. Deploy the new version

The binary embeds migration `00003_dual_calendar_dates.sql` (adds the
`secondary_*` columns) and runs it automatically on startup, same as any
other deploy:

```bash
git pull
./deploy/update.sh
```

## 2. Stop the bot before touching the DB

The backfill tool runs a multi-row transaction directly against the SQLite
file. Do it while the bot isn't also writing to avoid `SQLITE_BUSY` retries
under load:

```bash
sudo systemctl stop birthly-go
```

## 3. Build the tools

The server has no Go toolchain installed (`deploy/install.sh` only ever
copies pre-built binaries). Build both tools locally, same as
`deploy/update.sh` does for the bot itself, and copy them over:

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o backfill-dual-dates ./cmd/backfill-dual-dates
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o backup-db ./cmd/backup-db
scp backfill-dual-dates backup-db <server>:/opt/birthly-go/
```

Both tools are pure Go (`modernc.org/sqlite`), so neither the server nor
this step needs the `sqlite3` CLI installed.

Confirm the deployed migration actually added the columns the tools expect
by running the dry run in step 5: it queries `secondary_month` directly and
fails loudly with a SQL error if the column is missing.

## 4. Take a backup

The backfill tool creates its own timestamped backup automatically before
`--apply` (see step 6), this manual step is for an extra, independent
snapshot before you start poking at anything. Runs as the `birthly-go` user
(see "A note on permissions" above):

```bash
cd /opt/birthly-go
sudo -u birthly-go ./deploy/backup_db.sh data/birthly.db data/backups
```

Prints the created snapshot's path
(`data/backups/birthly_manual_<timestamp>.db`). Keeps the newest 7 by
default (`BACKUP_RETENTION` env var to change). Manual snapshots and the
tool's own pre-apply snapshots (step 6) use different filename prefixes, so
each has its own independent retention count and neither prunes the other.

## 5. Dry run

```bash
cd /opt/birthly-go
sudo -u birthly-go ./backfill-dual-dates --db-path data/birthly.db
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
sudo -u birthly-go ./backfill-dual-dates --db-path data/birthly.db --apply
```

This will:
1. Create its own timestamped backup in `data/backups/birthly_backfill_<timestamp>.db`
   (via `VACUUM INTO`, safe under WAL), aborts before touching anything if
   this fails.
2. Run every computed change in a single transaction, any single event
   failing rolls back the entire batch, so the DB is never left half-updated.
3. Print a log line per updated event.

## 7. Verify

Re-run the dry run from step 5. It re-evaluates the same eligibility query
used for the backfill, so a clean run now proves the batch is complete
without needing any separate DB tooling:

```bash
sudo -u birthly-go ./backfill-dual-dates --db-path data/birthly.db
```

Must now print `no eligible events found`. If it lists any events, something
was missed or failed partway; check the apply log from step 6 for anything
other than successful `applied N event(s)` lines before re-running.

To spot-check a specific converted event's columns, you need the `sqlite3`
CLI (`apt-get install sqlite3` if it's not already on the server, it is not
required by any other step in this runbook):

```bash
sudo -u birthly-go sqlite3 data/birthly.db <<'SQL'
SELECT id, first_name, last_name, calendar_type, year, month, day,
       secondary_calendar_type, secondary_month, secondary_day,
       next_occurrence, secondary_next_occurrence
FROM events WHERE first_name = '<name>';
SQL
```

`calendar_type` should now read `hebrew`, with `secondary_calendar_type` =
`gregorian` holding the original birth month/day.

## 8. Restart the bot

```bash
sudo systemctl start birthly-go
sudo systemctl status birthly-go --no-pager
```

## Rollback

If something looks wrong after applying, stop the bot, remove the WAL/SHM
files so SQLite doesn't replay stale write-ahead pages over the restored
snapshot, and restore the pre-apply backup:

```bash
sudo systemctl stop birthly-go
sudo -u birthly-go rm -f data/birthly.db-wal data/birthly.db-shm
sudo -u birthly-go cp data/backups/birthly_backfill_<timestamp>.db data/birthly.db
sudo systemctl start birthly-go
```

## Re-running

The tool is idempotent: once an event has been converted, it's no longer
`calendar_type = 'gregorian'`, so a second `--apply` run finds nothing left
to do and changes nothing. Safe to re-run the dry-run or apply at any time
to confirm the backfill is complete.
