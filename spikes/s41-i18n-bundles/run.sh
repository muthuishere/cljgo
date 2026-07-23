#!/usr/bin/env bash
# S41 driver — pure-Go, no services. Builds a CGO_ENABLED=0 static
# binary (proves the single-binary/embed claim) then runs the probe,
# which prints PASS/FAIL per exit criterion. Throwaway (ADR 0027).
set -euo pipefail
cd "$(dirname "$0")"

echo "== building static (CGO_ENABLED=0) binary =="
CGO_ENABLED=0 go build -o /tmp/s41probe .
echo "built /tmp/s41probe (locales embedded)"
echo
/tmp/s41probe
