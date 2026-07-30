#!/bin/bash
# s69 driver: cljgo's `load-file` is unusable here (see VERDICT "incidental
# findings"), so each demo is run by concatenating prototype.clj in front of
# it into out/ and running the combined script.
set -euo pipefail
cd "$(dirname "$0")"
CLJGO="${CLJGO:-/tmp/cljgo-s69-semantics}"
mkdir -p out
for f in "$@"; do
  base="$(basename "$f" .clj)"
  cat prototype.clj "$f" > "out/${base}.run.clj"
  echo "================================================================"
  echo "== $f"
  echo "================================================================"
  "$CLJGO" run "out/${base}.run.clj" || echo "!! exit $?"
done
