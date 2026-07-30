#!/bin/bash
# Regenerate evidence.txt (every number in VERDICT.md comes from here).
cd "$(dirname "$0")"
{
  echo "s69/semantics evidence — captured $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "cljgo: $(git rev-parse --short HEAD) (branch $(git rev-parse --abbrev-ref HEAD))"
  echo "host:  $(uname -sm)"
  echo
  ./run.sh 01-once-only.clj 02-hygiene.clj 03-with-redefs.clj 04-recursion.clj \
           05-variadic-multiarity.clj 06-fn-fallback.clj 07-cljx-port.clj \
           08-cost-of-once-only.clj
  echo
  echo "================================================================"
  echo "== divergent recursion cases (hard 30s timeout)"
  echo "================================================================"
  ./run-recursion.sh
} > evidence.txt 2>&1
echo "wrote evidence.txt ($(wc -l < evidence.txt) lines)"
