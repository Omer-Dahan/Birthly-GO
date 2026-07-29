#!/usr/bin/env bash
# Rebuild, copy the new binary in, restart. Run from the repo working copy.
set -euo pipefail

APP_DIR=/opt/birthly-go

# -s -w strip debug symbols/DWARF — ~30% smaller on disk (24.7MB -> 17.5MB
# measured locally), no effect on behavior; only loses `go tool` debugging
# of a crash dump, which journald's panic log plus the source repo cover.
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o birthly ./cmd/birthly
sudo cp birthly "$APP_DIR/birthly"
sudo chown birthly-go:birthly-go "$APP_DIR/birthly"
sudo chmod 755 "$APP_DIR/birthly"
sudo systemctl restart birthly-go
