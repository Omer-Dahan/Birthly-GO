#!/usr/bin/env bash
# Idempotent installer for the Go rewrite of Birthly. Targets a clean Ubuntu
# 22.04/24.04 host, side-by-side with (not replacing) an existing Python
# install during shadow-run — see deploy/SHADOW_RUN.md.
set -euo pipefail

APP_USER=birthly-go
APP_DIR=/opt/birthly-go

id -u "$APP_USER" &>/dev/null || sudo useradd -r -s /bin/false -d "$APP_DIR" "$APP_USER"
sudo mkdir -p "$APP_DIR" "$APP_DIR/data/backups" "$APP_DIR/data/logs"
sudo chown -R "$APP_USER:$APP_USER" "$APP_DIR"
sudo chmod 700 "$APP_DIR/data"

# Build locally (or in CI) and copy the static binary + .env here — there is
# no venv/pip step: the Go binary is self-contained (migrations and locale
# files are compiled in via go:embed).
if [ ! -f "$APP_DIR/birthly" ]; then
  echo "⚠️  Copy the built 'birthly' binary to $APP_DIR/birthly, then run this script again."
  echo "    e.g.: GOOS=linux GOARCH=amd64 go build -ldflags=\"-s -w\" -o birthly ./cmd/birthly"
  exit 1
fi
sudo chown "$APP_USER:$APP_USER" "$APP_DIR/birthly"
sudo chmod 755 "$APP_DIR/birthly"

if [ ! -f "$APP_DIR/.env" ]; then
  sudo -u "$APP_USER" cp .env.example "$APP_DIR/.env"
  echo "⚠️  ערוך את $APP_DIR/.env והכנס BOT_TOKEN (טוקן נפרד מהבוט של Python בזמן shadow-run), ואז הרץ שוב"
  exit 1
fi
sudo chmod 600 "$APP_DIR/.env"

# The binary runs its embedded migrations against DB_PATH on startup — no
# separate migrate step (unlike Python's `alembic upgrade head`).

sudo cp deploy/birthly-go.service /etc/systemd/system/
sudo cp deploy/logrotate.birthly-go /etc/logrotate.d/birthly-go
sudo systemctl daemon-reload
sudo systemctl enable --now birthly-go
sudo systemctl status birthly-go --no-pager
