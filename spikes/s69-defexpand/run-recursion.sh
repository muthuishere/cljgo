#!/bin/bash
# The two divergent recursion cases: run under a hard timeout and report what
# actually happened (exit code + peak RSS), rather than asserting "it hangs".
cd "$(dirname "$0")"
CLJGO="${CLJGO:-/tmp/cljgo-s69-semantics}"
mkdir -p out
for f in 04a-recursion-hang.clj 04b-recursion-mutual.clj; do
  base="$(basename "$f" .clj)"
  cat prototype.clj "$f" > "out/${base}.run.clj"
  echo "== $f (limit: 30s)"
  start=$(date +%s)
  /usr/bin/time -l timeout 30 "$CLJGO" run "out/${base}.run.clj" 2>&1 \
    | grep -Ev '^\s+[0-9]+\s+(voluntary|involuntary|page|messages|signals|swaps|block|instructions|cycles|peak memory|reclaims|faults)' \
    | head -20
  rc=${PIPESTATUS[0]}
  echo "   elapsed: $(( $(date +%s) - start ))s"
done
