#!/usr/bin/env bash
# Run the S13 numeric probes against both the real Clojure CLI (oracle) and
# cljgo, from the repo root, and write raw outputs for diff_probes.py.
set -euo pipefail
cd "$(dirname "$0")/../.."   # repo root

DIR=spikes/s13-numeric-divergences
mkdir -p "$DIR/out"

for f in probes probes_abs; do
  echo "== oracle: $f.clj =="
  clojure -M "$DIR/$f.clj" > "$DIR/out/${f}.oracle.txt" 2>&1 || true
  echo "== cljgo:  $f.clj =="
  go run ./cmd/cljgo run "$DIR/$f.clj" > "$DIR/out/${f}.cljgo.txt" 2>&1 || true
done

echo "Done. Raw outputs in $DIR/out/. Run diff_probes.py to compare."
