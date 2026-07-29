# Stage 12 — shadow run, walkthrough, cutover

This is the runbook for the final stage of the Python→Go rewrite (see the
migration plan). It covers everything from here to a production cutover.

**None of this has been executed yet.** Everything below requires actions
this assistant cannot take on its own: running a live bot against real
Telegram traffic, holding a VPS session open for a week, or stopping the
production service. The build and static verification (steps 0) are done;
steps 1–4 are for you to run.

---

## Step 0 — build, already verified

```
go build -o bin/birthly ./cmd/birthly   # local sanity build
go test ./...                           # full suite green as of this commit
go vet ./...

# actual VPS build (matches deploy/install.sh and deploy/update.sh):
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o birthly ./cmd/birthly
```

`-s -w` strips debug symbols — 24.7MB → 17.5MB measured locally (Windows
amd64; the Linux VPS build will differ slightly but the ratio holds), no
behavior change. That's on-disk size, not RSS — actual memory footprint is
what Step 2's RAM monitoring measures.

`deploy/birthly-go.service`, `deploy/logrotate.birthly-go`, `deploy/install.sh`,
`deploy/update.sh`, and `.env.example` are ready in this repo.

## Step 1 — install side-by-side with the live Python bot

On the VPS, **do not touch the existing `birthly` systemd service or its
`/opt/birthly` data directory.** The Go install lives entirely alongside it:

```
scp bin/birthly your-vps:/tmp/
ssh your-vps
cd /path/to/Birthly-GO-checkout
cp /tmp/birthly .
./deploy/install.sh
```

`install.sh` creates a separate `birthly-go` OS user and `/opt/birthly-go`
directory tree. Before it will start the service, edit `/opt/birthly-go/.env`:

- `BOT_TOKEN` — a **separate bot token** (a second BotFather bot), never the
  production token. This is what makes shadow-run safe: real users only ever
  talk to the Python bot.
- `DB_PATH` — point it at a **copy** of the production DB, e.g.
  `data/birthly_shadow.db`. Copy it with `sqlite3 prod.db ".backup shadow.db"`
  (safe against a live WAL-mode writer) rather than `cp`, then `chmod 600`
  it under the `birthly-go` user.
- `ADMIN_IDS` — your own Telegram id, so `/admin`, `/dbstats`, etc. are
  reachable on the shadow bot for the manual walkthrough in Step 3.

## Step 2 — shadow run (one week)

With both `birthly` (Python, production) and `birthly-go` (Go, shadow)
running — Python serving real users, Go serving nobody but you on its own
token — both schedulers tick against their respective copies of
`notifications_log`. After ~1 week, compare:

```
sqlite3 /opt/birthly/data/birthly.db \
  "SELECT event_id, rule_id, occurrence_date, scheduled_at FROM notifications_log ORDER BY 1,2,3" \
  > /tmp/py_notifs.txt

sqlite3 /opt/birthly-go/data/birthly_shadow.db \
  "SELECT event_id, rule_id, occurrence_date, scheduled_at FROM notifications_log ORDER BY 1,2,3" \
  > /tmp/go_notifs.txt

diff /tmp/py_notifs.txt /tmp/go_notifs.txt
```

(Or `sqldiff prod.db shadow.db` if the `sqldiff` CLI is installed — same
idea, more detail.) **Target: zero differences** in
`(event_id, rule_id, occurrence_date, scheduled_at)`. Any diff here means a
scheduling-logic mismatch and must be root-caused before cutover — this is
the single hardest-to-unit-test surface in the whole rewrite (timezone
arithmetic × grace windows × per-event rule overrides, running against real
production data volume and shape).

While shadow-run is going, watch RSS:

```
watch -n60 'systemctl show birthly-go --property=MemoryCurrent'
# or: cat /proc/$(systemctl show birthly-go -p MainPID --value)/status | grep VmRSS
```

Target: stable **under 40MB RSS** after 48h. A steady climb points at a leak
in the FSM store, a rate-limit bucket map, or the outbound send-rate
limiter never evicting stale entries.

## Step 3 — manual walkthrough (both languages)

Talk to the shadow bot (its own token) yourself, in Hebrew and then again
after switching to English via the Settings screen's language toggle
(there's no `/lang` slash command — S12 below), through every screen.
Checklist (SPEC.md screen numbers):

- [ ] S1 · Home (`/start` for an existing user)
- [ ] S2–S5 · Add event: name → Gregorian date → (side path) Hebrew date →
      saved-result screen
- [ ] S6 · "More details" (relation / phone / notes / category / photo /
      clear-a-field)
- [ ] S7 · List, pagination
- [ ] S8 · Event card
- [ ] S9 · Delete → undo
- [ ] S10 · Search (name, category name, Hebrew month name, phone)
- [ ] S11 · Reminders (global + per-event rules, toggle, delete, change time)
- [ ] S12 · Settings (language, timezone, date/time format, notifications
      toggle, wipe account — cancel out of wipe, don't actually confirm it)
- [ ] S13 · Stats
- [ ] S14 · Greetings (pick a tone, "pick another", AI-handoff prompt,
      personal template create/use/delete, memorial event blocks the flow)
- [ ] S15 · Backup (export JSON/CSV/XLSX, import a JSON export back in)
- [ ] S16 · Reminder push itself — wait for or force one via `/forcebackup`-
      adjacent testing, or temporarily set an event's date to trigger a
      same-day reminder
- [ ] S18 · Help
- [ ] S19 · Admin (`/admin`, `/dbstats`, `/broadcast` to yourself only,
      `/logs`, `/userinfo`, `/block` + `/unblock` on a throwaway test
      account, `/forcebackup`)

S17 (daily digest) is intentionally a stub in both the Go port and — per
its own comment — Python (`app/scheduler/jobs.py` marks it "Full
implementation: M6"); skip it.

## Step 4 — cutover (requires your explicit go-ahead, not automated here)

Only after Step 2 shows zero `notifications_log` diffs and Step 3's
checklist is clean:

```
systemctl stop birthly                          # Python
sqlite3 /opt/birthly/data/birthly.db "PRAGMA wal_checkpoint(TRUNCATE);"
# point /opt/birthly-go/.env's DB_PATH at the real /opt/birthly/data/birthly.db
# (same file, same schema — alembic_version table is untouched by the Go
# migration runner, which uses its own goose_db_version bookkeeping table)
systemctl start birthly-go
```

**Rollback** if anything looks wrong: `systemctl stop birthly-go && systemctl
start birthly` — the schema is identical and untouched, so this is safe to
do at any point, even hours or days after cutover.

This step is destructive to the live service and irreversible in the
"real users notice" sense even though the technical rollback is safe — do
not run it without confirming with the user in the moment, per this
project's own standing rule.
