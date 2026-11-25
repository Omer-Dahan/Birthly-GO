# Hebrew birth year repair runbook

One-time, manual operation. Never run on boot, never from cron or `update.sh`.

Background: before commit dc4492d a Gregorian birth year typed in the Hebrew
flow was converted as `+3760` for every month. For births from Rosh Hashana
to 31 December the correct Hebrew year is `+3761`, so the stored year is one
too small and the age shown is one too high.

Tool: `cmd/repair-hebrew-birth-year` (logic in `internal/backfill/hebyear.go`).
It only ever changes `events.year` (+1) and `updated_at`. Month, day, the
secondary date and next-occurrence columns do not depend on the year.

## The provability signal

A row is fixed only when ALL hold: not deleted, birthday, `calendar_type =
'hebrew'`, a stored year, and a Gregorian secondary date whose month/day

- equals the Gregorian date of the stored Hebrew month/day in year **+1**,
- differs from its date in the stored year, and
- year+1 falls in Gregorian year (stored year - 3760), the year the old rule assumed.

That means the owner confirmed or typed a true Gregorian date that contradicts
the stored year. The same text is printed by the dry run.

Classes reported (counts): `provable`, `already-correct` (secondary agrees with
the stored year), `ambiguous` (everything else, never touched).

## What cannot be proven

- Hebrew events with no secondary date: no Gregorian date is stored.
- Rows whose secondary date was auto-derived from the wrong year by the old
  code: it agrees with the wrong year, so it looks identical to a correct row
  (counted as `already-correct`). These need the owner to re-enter the year via
  the edit flow.

## Steps (as the service user, bot stopped)

```bash
sudo systemctl stop birthly-go
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o repair-hebrew-birth-year ./cmd/repair-hebrew-birth-year
scp repair-hebrew-birth-year <server>:/opt/birthly-go/
cd /opt/birthly-go
sudo -u birthly-go ./repair-hebrew-birth-year --db-path data/birthly.db          # dry run
sudo -u birthly-go ./repair-hebrew-birth-year --db-path data/birthly.db --apply  # write
sudo systemctl start birthly-go
```

`--apply` first writes `data/backups/birthly_hebyear_<timestamp>.db`
(aborts if that fails), then updates all rows in one transaction.

## Idempotency and rollback

After apply the fixed rows read `already-correct`, so a re-run finds nothing.
Rollback is the same restore procedure as `DUAL_DATES_BACKFILL.md` using the
`birthly_hebyear_*` snapshot.
