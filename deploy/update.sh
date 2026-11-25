#!/usr/bin/env bash
# Rebuild, copy the new binary in, restart. Run from the repo working copy.
set -euo pipefail

APP_DIR=/opt/birthly-go

# -s -w strip debug symbols/DWARF — ~30% smaller on disk (24.7MB -> 17.5MB
# measured locally), no effect on behavior; only loses `go tool` debugging
# of a crash dump, which journald's panic log plus the source repo cover.
# Stamp the binary with the version, commit and build date taken from git, so
# `/version` and the bot_starting log line say exactly what is running.
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse HEAD 2>/dev/null || echo dev)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PKG=birthly/internal/version
LDFLAGS="-s -w -X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.Date=${BUILD_DATE}"

GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o birthly ./cmd/birthly
sudo cp birthly "$APP_DIR/birthly"
sudo chown birthly-go:birthly-go "$APP_DIR/birthly"
sudo chmod 755 "$APP_DIR/birthly"
sudo systemctl restart birthly-go
