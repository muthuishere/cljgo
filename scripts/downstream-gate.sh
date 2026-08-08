#!/usr/bin/env bash
# Run the downstream consumer gate LOCALLY against this cljgo source tree.
#
#   scripts/downstream-gate.sh              both consumers
#   scripts/downstream-gate.sh toolnexus    just one
#
# This is the same work `.github/workflows/downstream.yml` does, and it exists
# so the gate is runnable BEFORE a push — the CI copy runs on push-to-main and
# nightly, which is too late to be useful while you are still editing.
#
# It uses local clones when it finds them (fast, and it is what the owner
# actually has) and clones the public repos into a temp dir otherwise. Override
# either with CLJGO_TOOLNEXUS_DIR / CLJGO_KOINE_DIR.
#
# REQUIREMENTS. go (to build cljgo), git, network (Clojars/Maven), and for the
# koine leg also java + the `clojure` CLI + python3 + timeout(1). The koine leg
# is the one that needs a JVM, because its whole point is comparing cljgo
# against real JVM Clojure.
#
# THE TRAP, hit on 2026-08-08 and worth stating loudly: a cljgo built from
# source reports version 0.1.0-dev, and cljgo REFUSES to generate a go.mod for a
# dev binary unless CLJGO_SRC points at its runtime tree. Without it the
# toolnexus gate dies at `cljgo build` with "cannot locate the
# github.com/muthuishere/cljgo source tree" — which reads like a consumer
# dependency problem and is not one. This script sets it for you.
#
# MEASURED, warm Maven cache, owner's laptop, 2026-08-08, current source:
#   toolnexus  32.5s   395 tests / 1614 assertions, AOT and interpreted, compared
#   koine      84.4s   14 checks x {JVM Clojure 1.12.5, cljgo}
set -uo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
WHICH=${1:-all}
BIN_DIR=$(mktemp -d)
trap 'rm -rf "$BIN_DIR"' EXIT

# Same pins as .github/workflows/downstream.yml — keep the two in step, or a
# local green stops being evidence about what CI will do. A LOCAL clone found
# on disk is used as-is (that is the point of working locally); only a fresh
# clone gets pinned.
TOOLNEXUS_REPO=https://github.com/muthuishere/toolnexus.git
TOOLNEXUS_REF=e8e27b848b8a57aa1b3504ae76d98354b9fe5923
KOINE_REPO=https://github.com/muthuishere/koine.git
KOINE_REF=969883073b15c8e0fdd3100855cee6d7363b65d9

log() { printf '%s\n' "$*" >&2; }

log "== building cljgo from $REPO_ROOT"
go build -o "$BIN_DIR/cljgo" "$REPO_ROOT/cmd/cljgo" || {
  log "FATAL: cljgo build failed — fix that before blaming a consumer"
  exit 2
}
export PATH="$BIN_DIR:$PATH"
export CLJGO_SRC="$REPO_ROOT"   # see THE TRAP above
log "   cljgo: $(command -v cljgo)  CLJGO_SRC=$CLJGO_SRC"

# Resolve a consumer tree: explicit override, then a sibling clone, then a
# fresh clone of the public repo.
resolve () {   # $1 = name, $2 = override, $3 = local guess, $4 = url, $5 = pinned sha
  local name=$1 override=$2 guess=$3 url=$4 ref=$5
  if [ -n "$override" ]; then printf '%s' "$override"; return; fi
  if [ -d "$guess" ]; then printf '%s' "$guess"; return; fi
  local dst="$BIN_DIR/$name"
  log "   no local $name at $guess — cloning $url @ $ref"
  git clone --filter=blob:none "$url" "$dst" >&2 || return 1
  git -C "$dst" checkout --detach "$ref" >&2 || return 1
  printf '%s' "$dst"
}

FAILED=0

run_toolnexus () {
  local dir
  dir=$(resolve toolnexus "${CLJGO_TOOLNEXUS_DIR:-}" \
        "$REPO_ROOT/../../nexus-workspace/toolnexus" "$TOOLNEXUS_REPO" "$TOOLNEXUS_REF") || {
    log "FAIL toolnexus — could not obtain the tree"; FAILED=$((FAILED+1)); return; }
  log ""
  log "== toolnexus ($dir) — AOT vs interpreted, cross-compared"
  # Its own gate; run it unmodified and trust its exit code. Do NOT set
  # TN_EXAMPLES — the script hard-sets it, having been burned by a stale one.
  ( cd "$dir/clojure" && ./cljgo-gate.sh )
  local rc=$?
  [ $rc -eq 0 ] || { log "FAIL toolnexus (exit $rc)"; FAILED=$((FAILED+1)); }
}

run_koine () {
  local dir
  dir=$(resolve koine "${CLJGO_KOINE_DIR:-}" \
        "$REPO_ROOT/../koine" "$KOINE_REPO" "$KOINE_REF") || {
    log "FAIL koine — could not obtain the tree"; FAILED=$((FAILED+1)); return; }
  if ! command -v clojure >/dev/null; then
    log "FAIL koine — no \`clojure\` CLI. This leg IS the JVM oracle; skipping it"
    log "     would leave only cljgo-vs-cljgo, which cannot see a bug cljgo gets"
    log "     identically wrong everywhere. Install it, do not skip it."
    FAILED=$((FAILED+1)); return
  fi
  log ""
  log "== koine ($dir) — JVM Clojure vs cljgo"

  # UPSTREAM GAP (found 2026-08-08, koine @ 9698830). koine's COMMITTED
  # run-conformance.sh starts the SSE server but never writes
  # /tmp/koine-stream-base, and stream_check does
  # `(slurp "/tmp/koine-stream-base")` to learn the base URL — so from a CLEAN
  # CLONE stream_check fails on BOTH hosts with "slurp: no such file or
  # directory". It passes in the owner's koine working tree only because that
  # tree carries an UNCOMMITTED one-line fix adding the printf. Do the same
  # thing here, explicitly, so a clean clone and a laptop agree. Drop this once
  # koine commits it.
  rm -f /tmp/koine-stream-base
  ( cd "$dir/src" && python3 ../test/sse_server.py >/tmp/koine-sse.log 2>&1 & )
  sleep 3
  printf 'http://127.0.0.1:8791' > /tmp/koine-stream-base

  ( cd "$dir" && ./run-conformance.sh )
  local rc=$?
  pkill -f "$dir/../test/sse_server.py" 2>/dev/null
  pkill -f "test/sse_server.py" 2>/dev/null
  [ $rc -eq 0 ] || { log "FAIL koine (exit $rc)"; FAILED=$((FAILED+1)); }
}

case "$WHICH" in
  all)       run_toolnexus; run_koine ;;
  toolnexus) run_toolnexus ;;
  koine)     run_koine ;;
  *) log "usage: $0 [all|toolnexus|koine]"; exit 2 ;;
esac

log ""
if [ "$FAILED" -eq 0 ]; then
  log "DOWNSTREAM GREEN"
  exit 0
fi
log "DOWNSTREAM RED — $FAILED consumer leg(s) failed"
exit 1
