#!/usr/bin/env bash
# Reproduces every S17 measurement. Writes only into $SCRATCH (a temp dir
# outside the repo) and a throwaway GOMODCACHE — never into the repo tree.
# Run from anywhere; paths below are resolved relative to this script.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRATCH="$(mktemp -d /tmp/s17-scratch.XXXXXX)"
echo "scratch: $SCRATCH"

# --- build the prototype (standalone, no cljgo dependency) -----------------
( cd "$HERE/prototype" && go build -o "$SCRATCH/prototype" . )
echo "prototype built"

# --- a throwaway module cache: proves nothing is served from a warm --------
# machine-wide cache seeded by earlier cljgo dev work.
COLD_CACHE="$SCRATCH/gomodcache-cold"
mkdir -p "$COLD_CACHE"
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
export GOFLAGS=

run_case() {
  local label="$1" cache="$2" dir="$3"; shift 3
  echo
  echo "=== $label (dir=$dir, cache=$cache) ==="
  GOMODCACHE="$cache" GOPATH="$SCRATCH/gopath-$(basename "$cache")" \
    /usr/bin/time -p "$SCRATCH/prototype" "$dir" "$@" 2>&1 | tail -20
}

# ---------------------------------------------------------------------------
# Case 1: stdlib only, generated module dir with a bare release-pin require
# (ADR 0028 shape) and NO go.sum yet — the exact state right after
# SynthGoMod writes go.mod for a release binary, before `go mod tidy`.
# ---------------------------------------------------------------------------
GEN1="$SCRATCH/gen-stdlib"
mkdir -p "$GEN1"
cat > "$GEN1/go.mod" <<'EOF'
module cljgo.gen/main

go 1.26

require github.com/muthuishere/cljgo v0.1.0
EOF
cat > "$GEN1/main.go" <<'EOF'
package main

func main() {}
EOF

echo "############################################################"
echo "# Case 1: stdlib, NO go.sum yet (fresh release-pin go.mod)"
echo "############################################################"
run_case "stdlib, no go.sum, cold cache" "$COLD_CACHE" "$GEN1" \
  strings TrimSpace net/http Get || echo "CASE 1 (no go.sum) FAILED as shown above"

echo
echo "-- now: go mod tidy (materializes go.sum + fetches the cljgo require) --"
GOMODCACHE="$COLD_CACHE" GOPATH="$SCRATCH/gopath-tidy1" bash -c \
  "cd '$GEN1' && time go mod tidy" 2>&1 | tail -20

echo
run_case "stdlib, AFTER go mod tidy, cold cache reused" "$COLD_CACHE" "$GEN1" \
  strings TrimSpace net/http Get

run_case "stdlib, AFTER go mod tidy, warm (2nd run same cache)" "$COLD_CACHE" "$GEN1" \
  strings TrimSpace net/http Get

# ---------------------------------------------------------------------------
# Case 2: third-party module via a build.cljgo `go-require` — generated
# go.mod additionally requires github.com/google/uuid, then `go mod tidy`
# fetches it (this is what HostFactsDir would need to happen BEFORE fact
# loading, i.e. an ordering change vs today's EmitMain-before-SynthGoMod).
# ---------------------------------------------------------------------------
GEN2="$SCRATCH/gen-thirdparty"
mkdir -p "$GEN2"
cat > "$GEN2/go.mod" <<'EOF'
module cljgo.gen/main

go 1.26

require (
	github.com/muthuishere/cljgo v0.1.0
	github.com/google/uuid v1.6.0
)
EOF
cat > "$GEN2/main.go" <<'EOF'
package main

import "github.com/google/uuid"

func main() {
	_ = uuid.New()
}
EOF

echo
echo "############################################################"
echo "# Case 2: third-party module (github.com/google/uuid)"
echo "############################################################"
echo "-- go mod tidy on a FRESH cold cache (simulates first-ever build) --"
COLD_CACHE2="$SCRATCH/gomodcache-cold2"
mkdir -p "$COLD_CACHE2"
GOMODCACHE="$COLD_CACHE2" GOPATH="$SCRATCH/gopath-tidy2" bash -c \
  "cd '$GEN2' && time go mod tidy" 2>&1 | tail -20

echo
run_case "third-party, cold cache reused" "$COLD_CACHE2" "$GEN2" \
  github.com/google/uuid New

run_case "third-party, warm (2nd run)" "$COLD_CACHE2" "$GEN2" \
  github.com/google/uuid New

echo
echo "scratch dir left at: $SCRATCH (rm -rf when done inspecting)"
