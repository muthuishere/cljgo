#!/usr/bin/env bash
# Run the S16 BigDecimal probes against both the real Clojure CLI (oracle)
# and cljgo (baseline), from the repo root, then run the candidate-(a)
# prototype harness against the frozen oracle output.
set -euo pipefail
cd "$(dirname "$0")/../.."   # repo root

DIR=spikes/s16-bigdecimal-scaled
mkdir -p "$DIR/out"

for f in probes probes_wp; do
  echo "== oracle: $f.clj =="
  clojure -M "$DIR/$f.clj" > "$DIR/out/${f}.oracle.txt" 2>&1 || true
  echo "== cljgo:  $f.clj =="
  go run ./cmd/cljgo run "$DIR/$f.clj" > "$DIR/out/${f}.cljgo.txt" 2>&1 || true
done

echo "== prototype harness =="
(cd "$DIR/proto" && go run . \
  -oracle ../out/probes.oracle.txt \
  -oracle-wp ../out/probes_wp.oracle.txt) | tee "$DIR/out/proto_report.txt"
