#!/usr/bin/env bash
# S20 driver — proves the four exit criteria (see README.md).
# Criteria 1-3: prototype/main.go against the real pkg/eval/pkg/reader.
# Criterion 4: prototype/workers.clj through interpreted cljgo.
set -euo pipefail
cd "$(dirname "$0")"
ROOT=../..

echo "== criteria 1-3: live handlers, routes-as-data, config =="
(cd prototype && go build -o s20-proto . && ./s20-proto && rm -f s20-proto)

echo
echo "== criterion 4: goroutine workers with a persistence seam =="
(cd "$ROOT" && go build -o /tmp/cljgo-s20 ./cmd/cljgo)
(cd prototype && /tmp/cljgo-s20 run workers.clj)
rm -f /tmp/cljgo-s20
echo
echo "all criteria demonstrated"
