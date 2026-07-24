#!/usr/bin/env bash
# s46 measurement: build both candidates CGO_ENABLED=0, prove no cgo,
# cross-compile the ADR 0077 matrix, and report binary sizes + dep counts.
set -euo pipefail
cd "$(dirname "$0")"

report() {
  local name=$1 dir=$2
  ( cd "$dir" && go mod tidy >/dev/null 2>&1 || true )
  echo "=== $name ==="
  # 1. pure-Go / CGO_ENABLED=0 host build
  ( cd "$dir" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /tmp/s46_$name . )
  echo "  host build (CGO_ENABLED=0): OK  size=$(du -h /tmp/s46_$name | cut -f1)"
  # 2. cgo in the dep closure?
  local cgo
  cgo=$(cd "$dir" && CGO_ENABLED=0 go list -deps . 2>/dev/null | grep -cE 'runtime/cgo|mattn/go-sqlite3' || true)
  echo "  cgo packages in closure: $cgo"
  # 3. dependency-module count (maintenance surface)
  local mods
  mods=$(cd "$dir" && go list -m all 2>/dev/null | wc -l | tr -d ' ')
  echo "  modules in build (go list -m all): $mods"
  # 4. cross-compile the ADR 0077 matrix
  echo "  cross-compile:"
  for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
    os=${t%/*}; arch=${t#*/}
    if ( cd "$dir" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o /tmp/s46_${name}_${os}-${arch} . 2>/dev/null ); then
      echo "    $t: OK  $(du -h /tmp/s46_${name}_${os}-${arch} | cut -f1)"
    else
      echo "    $t: FAIL"
    fi
  done
  echo "  LOC (main.go): $(wc -l < "$dir/main.go")"
}

report charm charm
echo
report bespoke bespoke
