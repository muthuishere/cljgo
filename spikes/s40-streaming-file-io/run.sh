#!/usr/bin/env bash
# S40 driver — builds and runs the streaming-I/O probe. Self-contained; the
# probe generates its own ~200MB test data in $TMPDIR and deletes it on exit
# (os.RemoveAll of a MkdirTemp dir), so nothing large is left behind.
# Throwaway spike code (ADR 0027).
set -euo pipefail
cd "$(dirname "$0")/probe"

echo "== S40 gates =="
gofmt -l .
go vet ./...
go build . && rm -f s40
echo "== S40 probe (generates + deletes ~200MB) =="
go run .
