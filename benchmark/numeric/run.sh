#!/usr/bin/env bash
#
# ADR 0067 second-table batch — A/B harness.
#
# The kernels here are the ones the FIRST ADR 0067 table (+ - * inc dec and
# the comparisons) could not type: a single `quot` / `rem` / `even?` /
# `bit-xor` / `max` in a recur carrier demoted the carrier to `any`, which
# demoted every other carrier in the loop, which cost the whole region its
# typed emission. They are therefore the honest measurement surface for the
# batch; the shipped corpus (benchmark/programs) is the regression surface
# and must not move.
#
# Measures BOTH legs — interpreted (`cljgo run`) and AOT (`cljgo build`) —
# wall-clock totals including startup, hyperfine 3 warmup / 10 runs, the
# owner's bar (never boot-subtracted).
#
# Usage:
#   bash benchmark/numeric/run.sh                    # this tree
#   CLJGO_BIN=/path/to/cljgo bash benchmark/numeric/run.sh   # a prebuilt one
#   RUNS=3 WARMUP=1 bash benchmark/numeric/run.sh    # quick smoke
#
# Both legs also assert the two harnesses AGREE on the printed value — a
# REPL-vs-binary divergence here is a release blocker, not a slow row.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
BUILD="${BUILD:-$HERE/.build}"
WARMUP="${WARMUP:-3}"; RUNS="${RUNS:-10}"
mkdir -p "$BUILD"

command -v hyperfine >/dev/null || { echo "need hyperfine"; exit 1; }

CLJGO="${CLJGO_BIN:-}"
if [ -z "$CLJGO" ]; then
  echo "### building cljgo from $ROOT ###"
  ( cd "$ROOT" && go build -trimpath -ldflags="-s -w" -o "$BUILD/cljgo" ./cmd/cljgo ) || exit 1
  CLJGO="$BUILD/cljgo"
fi
export CLJGO_SRC="${CLJGO_SRC:-$ROOT}"

printf '%-12s | %10s | %10s | %10s\n' kernel "interp ms" "aot ms" "aot bytes"
printf -- '-------------|------------|------------|-----------\n'

for f in "$HERE"/*.clj; do
  name="$(basename "$f" .clj)"
  bin="$BUILD/$name"
  "$CLJGO" build -o "$bin" "$f" >/dev/null || { echo "$name: build failed"; continue; }

  want="$("$bin")"
  got="$("$CLJGO" run "$f")"
  if [ "$want" != "$got" ]; then
    echo "$name: DIVERGENCE interpreted=$got compiled=$want (release blocker)"; continue
  fi

  ims=$(hyperfine -N --warmup "$WARMUP" --runs "$RUNS" --export-json "$BUILD/$name.i.json" \
        "$CLJGO run $f" >/dev/null 2>&1 && \
        python3 -c "import json,sys;print('%.1f'%(json.load(open('$BUILD/$name.i.json'))['results'][0]['mean']*1000))")
  ams=$(hyperfine -N --warmup "$WARMUP" --runs "$RUNS" --export-json "$BUILD/$name.a.json" \
        "$bin" >/dev/null 2>&1 && \
        python3 -c "import json,sys;print('%.1f'%(json.load(open('$BUILD/$name.a.json'))['results'][0]['mean']*1000))")
  size=$(wc -c < "$bin" | tr -d ' ')
  printf '%-12s | %10s | %10s | %10s\n' "$name" "$ims" "$ams" "$size"
done
