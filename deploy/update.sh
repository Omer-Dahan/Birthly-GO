#!/usr/bin/env bash
# Rebuild, copy the new binary in, restart. Run from the repo working copy.
set -euo pipefail

APP_DIR=/opt/birthly-go

GOOS=linux GOARCH=amd64 go build -o birthly ./cmd/birthly
sudo cp birthly "$APP_DIR/birthly"
sudo chown birthly-go:birthly-go "$APP_DIR/birthly"
sudo chmod 755 "$APP_DIR/birthly"
sudo systemctl restart birthly-go
