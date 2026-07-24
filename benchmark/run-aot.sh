#!/usr/bin/env bash
#
# AOT-vs-AOT-vs-AOT benchmark — compiled binaries only, no interpreted legs.
#
# Three legs, all native binaries built from the same programs:
#   cljgo-aot   — `cljgo build` (this repo)
#   glojure-aot — gloat -E glj      (Glojure Clojure->Go->native)
#   letgo-aot   — gloat -E lglvm    (let-go IR lowered to Go, VM runtime linked;
#                 gloat's pure `lgl` engine is not implemented yet)
#
# Sources: cljgo compiles programs/*.clj (top-level call). The gloat engines
# compile let-go's own AOT variants of the SAME programs (ns + `-main`
# wrapper, upstream benchmark/gloat/*.clj) because gloat requires an entry
# point. Same computation, same constants.
#
# NOTE — interpreted legs (cljgo run, glj, lg VM, bb, joker, clj) are
# deliberately absent here; that comparison lives in run.sh / results.md.
#
# Methodology: hyperfine, 3 warmup / 10 timed runs, wall-clock mean,
# startup included. Compile time is NOT measured, only execution
# (matching let-go's published method).
#
# Prereq: binaries already built —
#   run.sh's precompile stage        -> .build/aot_<name>   (cljgo)
#   gloat -E glj  -o <name>-glj ...  -> .build/aotcmp/<name>-glj
#   gloat -E lglvm -o <name>-lg ...  -> .build/aotcmp/<name>-lg
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
BUILD="$HERE/.build"
AOTCMP="$BUILD/aotcmp"
OUT="$BUILD/results-aot"
mkdir -p "$OUT"
WARMUP="${WARMUP:-3}"; RUNS="${RUNS:-10}"

command -v hyperfine >/dev/null || { echo "need hyperfine"; exit 1; }

BENCHES=(tak fib loop-recur persistent-map map-filter transducers reduce)

bench_cmds () {  # emit hyperfine -n/cmd pairs for one program ($1) or "startup"
  local n="$1"
  if [ "$n" = startup ]; then
    [ -x "$BUILD/aot_startup" ]    && printf '%s\0' -n cljgo-aot "$BUILD/aot_startup"
    [ -x "$AOTCMP/startup-glj" ]   && printf '%s\0' -n glojure-aot "$AOTCMP/startup-glj"
    [ -x "$AOTCMP/startup-lg" ]    && printf '%s\0' -n letgo-aot "$AOTCMP/startup-lg"
    return
  fi
  [ -x "$BUILD/aot_$n" ]    && printf '%s\0' -n cljgo-aot "$BUILD/aot_$n"
  [ -x "$AOTCMP/$n-glj" ]   && printf '%s\0' -n glojure-aot "$AOTCMP/$n-glj"
  [ -x "$AOTCMP/$n-lg" ]    && printf '%s\0' -n letgo-aot "$AOTCMP/$n-lg"
}

for n in startup "${BENCHES[@]}"; do
  echo "### $n ###"
  mapfile -d '' pairs < <(bench_cmds "$n")
  if [ "${#pairs[@]}" -lt 6 ]; then echo "  SKIP (fewer than 2 legs present)"; continue; fi
  hyperfine --warmup "$WARMUP" --runs "$RUNS" --style basic \
    --export-json "$OUT/$n.json" "${pairs[@]}" >/dev/null 2>&1 \
    && echo "  done" || echo "  ERROR (a binary may have failed)"
done

echo "### rendering results-aot.md ###"
python3 "$HERE/report.py" --aot > "$HERE/results-aot.md" && echo "wrote $HERE/results-aot.md"
