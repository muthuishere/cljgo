#!/usr/bin/env bash
# Spike s28 probe: capture the rendered error for each case across the three
# contexts (REPL / cljgo run / compiled binary). Run from the repo root so the
# dev binary's walk-up finds this module for the generated go.mod replace.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
cd "$ROOT"

BIN=/tmp/cljgo-s28
echo "==> building fresh cljgo from source"
go build -o "$BIN" ./cmd/cljgo || { echo "build failed"; exit 1; }

hr() { printf '%s\n' "----------------------------------------------------------------"; }

run_case() {
  local name="$1" file="$HERE/$2" compile="${3:-yes}"
  hr; echo "### CASE: $name   ($2)"; hr
  echo "--- source ---"; cat "$file"
  echo "--- cljgo run (stderr) ---"
  "$BIN" run "$file" 2>&1 1>/dev/null
  if [ "$compile" = "yes" ]; then
    echo "--- compiled binary (stderr) ---"
    local out; out="$(mktemp -d)"
    if "$BIN" build -o "$out/prog" "$file" >"$out/build.log" 2>&1; then
      "$out/prog" 2>&1 1>/dev/null
    else
      echo "[build-time error — not a runtime panic; build output:]"
      cat "$out/build.log"
    fi
    rm -rf "$out"
  fi
  echo
}

repl_case() {
  local name="$1"; shift
  hr; echo "### REPL CASE: $name"; hr
  printf '%b' "$1" | "$BIN" repl 2>&1
  echo
}

run_case "arity (named + located + expects)" arity.clj
run_case "unresolved symbol + did-you-mean"  unresolved.clj
run_case "type/cast runtime error"            typecast.clj
run_case "divide-by-zero (location-less; parity proof)" divzero.clj
run_case "reader error (positioned, compile-time)" reader.clj

# P0 boundary parity: `cljgo build` evaluates top-level forms, so the runtime
# error must live in -main for the COMPILED binary to reach main()'s recover().
hr; echo "### CASE: compiled -main runtime throw (P0 recover boundary)"; hr
cat "$HERE/main-throw.clj"
OUT="$(mktemp -d)"
if "$BIN" build -o "$OUT/prog" "$HERE/main-throw.clj" >"$OUT/build.log" 2>&1; then
  echo "--- compiled binary (stderr) — clean line, no Go stack trace ---"
  "$OUT/prog" 2>&1 1>/dev/null; echo "exit=$?"
else
  echo "[unexpected build error]"; cat "$OUT/build.log"
fi
rm -rf "$OUT"
echo

# REPL captures for the headline cases (filename is REPL, lines relative to
# the input line).
repl_case "arity"      '(def f (fn* f [x] x))\n(f 1 2 3)\n'
repl_case "unresolved" '(pritnln "hi")\n'
repl_case "divzero"    '(/ 1 0)\n'
