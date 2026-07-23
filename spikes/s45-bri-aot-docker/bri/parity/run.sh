#!/usr/bin/env bash
# parity/run.sh — the DUAL-MODE parity gate for the bri hello app (ADR 0071
# dec 6, spike s45 exit criterion 2). Drives app.parity through bri.http's
# in-process client BOTH interpreted (`cljgo run`) AND compiled (`cljgo
# build`) and asserts byte-identical responses for: a text route, a JSON
# route, and a JWT-guarded route with no token (401) and with one (200).
#
# A REPL<->binary divergence is a release blocker (CLAUDE.md); any diff here
# exits non-zero. Deterministic: only the canonical `show` lines (those
# containing ":status") are compared — the api-defaults request log carries
# per-request timing and lives on the same stream, so it is filtered out.
set -euo pipefail

app="$(cd "$(dirname "$0")/.." && pwd)"       # .../spikes/s45-bri-aot-docker/bri
repo="$(cd "$app/../../.." && pwd)"           # cljgo repo root
cd "$app"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# One host-built cljgo for both legs (fast; avoids per-invocation `go run`).
cljgo="$tmp/cljgo"
( cd "$repo" && go build -o "$cljgo" ./cmd/cljgo )

echo "== interpreted (cljgo run) =="
"$cljgo" run src/run_parity.cljg 2>/dev/null | grep ':status' > "$tmp/interp.txt"
cat "$tmp/interp.txt"

echo "== compiled (cljgo build) =="
"$cljgo" build -o "$tmp/parity-bin" src/app/parity.cljg
"$tmp/parity-bin" 2>/dev/null | grep ':status' > "$tmp/compiled.txt"
cat "$tmp/compiled.txt"

echo "== diff =="
if diff -u "$tmp/interp.txt" "$tmp/compiled.txt"; then
  echo "PARITY OK — interpreted and compiled bri are byte-identical"
else
  echo "PARITY FAIL — interpreted vs compiled diverged (release blocker)" >&2
  exit 1
fi
