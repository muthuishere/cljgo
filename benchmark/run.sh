#!/usr/bin/env bash
#
# cljgo benchmark harness — the reproducible cross-implementation suite.
#
# Runs BOTH cljgo legs (interpreted `cljgo run` and AOT `cljgo build`) against
# the closest comparables, on let-go's own 7 programs (vendored under
# programs/, derived from github.com/nooga/let-go, unmodified). This is the
# push-button version of the ad-hoc spike S22/S24 comparisons.
#
# Methodology (matches let-go's published method): hyperfine, 3 warmup /
# 10 timed runs, wall-clock mean ± σ, totals INCLUDE each runtime's startup.
# Owner's honesty bar: wall-clock totals, never boot-subtracted (design/00 §1).
#
# Requires: go, hyperfine. Auto-detects: bb (babashka), joker, clj (Clojure
# JVM), and let-go (via $LETGO, or built from ../references/let-go if present).
#
# Usage:  bash benchmark/run.sh          # full suite -> results.md
#         WARMUP=1 RUNS=3 bash benchmark/run.sh   # quick smoke
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
PROGRAMS="$HERE/programs"
BUILD="$HERE/.build"          # gitignored; holds compiled binaries
OUT="$HERE/.build/results"
mkdir -p "$BUILD" "$OUT"
WARMUP="${WARMUP:-3}"; RUNS="${RUNS:-10}"

command -v hyperfine >/dev/null || { echo "need hyperfine"; exit 1; }

echo "### building cljgo @HEAD ###"
go build -trimpath -ldflags="-s -w" -o "$BUILD/cljgo" "$ROOT/cmd/cljgo" || exit 1
CLJGO="$BUILD/cljgo"

# let-go: env override, else build from a reference clone if one is found.
# references/ is gitignored external study material whose location is not fixed
# (repo may be a git worktree), so probe a few candidates; pass $LETGO to skip.
LETGO="${LETGO:-}"
if [ -z "$LETGO" ]; then
  for c in "$ROOT/../references/let-go" "$ROOT/../../references/let-go" "$ROOT/references/let-go"; do
    if [ -f "$c/lg.go" ]; then
      echo "### building let-go from $c ###"
      ( cd "$c" && go build -trimpath -ldflags="-s -w" -o "$BUILD/letgo" . ) && LETGO="$BUILD/letgo"
      break
    fi
  done
fi
[ -z "$LETGO" ] && echo "### let-go not found (set \$LETGO to include it) ###"

# glojure: env override, else build glj from a reference clone (same probing).
GLJ="${GLJ:-}"
if [ -z "$GLJ" ]; then
  for c in "$ROOT/../references/glojure" "$ROOT/../../references/glojure" "$ROOT/references/glojure"; do
    if [ -f "$c/cmd/glj/main.go" ]; then
      echo "### building glojure from $c ###"
      ( cd "$c" && go build -trimpath -ldflags="-s -w" -o "$BUILD/glj" ./cmd/glj ) && GLJ="$BUILD/glj"
      break
    fi
  done
fi
[ -z "$GLJ" ] && echo "### glojure not found (set \$GLJ to include it) ###"
BB=$(command -v bb || true); JOKER=$(command -v joker || true); CLJ=$(command -v clj || true)

BENCHES=(tak fib loop-recur persistent-map map-filter transducers reduce)
JOKER_SKIP="fib tak transducers"    # joker: ~13x slower on tree-recursion; no transducers

echo "### precompiling AOT binaries (cljgo build) ###"
: > "$BUILD/empty.clj"
"$CLJGO" build -o "$BUILD/aot_startup" "$BUILD/empty.clj" >/dev/null 2>&1
for b in "${BENCHES[@]}"; do
  "$CLJGO" build -o "$BUILD/aot_$b" "$PROGRAMS/$b.clj" >/dev/null 2>&1 && echo "  cljgo-aot $b ok" || echo "  cljgo-aot $b FAIL"
done

bench_cmds () {  # emit hyperfine -n/cmd pairs for one program name ($1) or "startup"
  local n="$1" f
  if [ "$n" = startup ]; then
    printf '%s\0' -n cljgo-run "$CLJGO run $BUILD/empty.clj"
    [ -x "$BUILD/aot_startup" ] && printf '%s\0' -n cljgo-aot "$BUILD/aot_startup"
    [ -n "$LETGO" ] && printf '%s\0' -n let-go "$LETGO -e nil"
    [ -n "$GLJ" ]   && printf '%s\0' -n glojure "$GLJ -e nil"
    [ -n "$BB" ]    && printf '%s\0' -n babashka "bb -e nil"
    [ -n "$JOKER" ] && printf '%s\0' -n joker "joker -e nil"
    [ -n "$CLJ" ]   && printf '%s\0' -n clojure-jvm "clj -M -e nil"
    return
  fi
  f="$PROGRAMS/$n.clj"
  printf '%s\0' -n cljgo-run "$CLJGO run $f"
  [ -x "$BUILD/aot_$n" ] && printf '%s\0' -n cljgo-aot "$BUILD/aot_$n"
  [ -n "$LETGO" ] && printf '%s\0' -n let-go "$LETGO $f"
  [ -n "$GLJ" ]   && printf '%s\0' -n glojure "$GLJ $f"
  [ -n "$BB" ]    && printf '%s\0' -n babashka "bb $f"
  if [ -n "$JOKER" ] && ! echo " $JOKER_SKIP " | grep -q " $n "; then printf '%s\0' -n joker "joker $f"; fi
  [ -n "$CLJ" ]   && printf '%s\0' -n clojure-jvm "clj -M -e '(load-file \"$f\")'"
}

for n in startup "${BENCHES[@]}"; do
  echo "### $n ###"
  mapfile -d '' pairs < <(bench_cmds "$n")
  hyperfine --warmup "$WARMUP" --runs "$RUNS" --style basic \
    --export-json "$OUT/$n.json" "${pairs[@]}" >/dev/null 2>&1 \
    && echo "  done" || echo "  ERROR (a runtime may have failed on this program)"
done

echo "### rendering results.md ###"
python3 "$HERE/report.py" > "$HERE/results.md" && echo "wrote $HERE/results.md"
