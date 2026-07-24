#!/usr/bin/env bash
# s49 — can a shared cross-OS "native systems" layer (services, SIMD, GPU,
# Win32/Cocoa) be built WITHOUT cgo, preserving CGO_ENABLED=0 + `cljgo dist`?
set -euo pipefail
d=$(mktemp -d); cd "$d"; go mod init s49probe >/dev/null 2>&1
cat > main.go <<'EOF'
package main
import (
	_ "github.com/ebitengine/purego"   // runtime FFI to native .so/.dylib/.dll — NO cgo (ADR 0044)
	_ "github.com/klauspost/cpuid/v2"  // SIMD feature detect via Go assembly — NO cgo
	_ "golang.org/x/sys/unix"          // OS syscalls / service mgmt — NO cgo
)
func main() {}
EOF
GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1
CGO_ENABLED=0 go build -o /tmp/s49 . && echo "host CGO_ENABLED=0: OK ($(du -h /tmp/s49|cut -f1))"
echo "cgo in closure: $(CGO_ENABLED=0 go list -deps . | grep -cE 'runtime/cgo')"
for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  CGO_ENABLED=0 GOOS=${t%/*} GOARCH=${t#*/} go build -o /dev/null . 2>/dev/null && echo "  $t OK" || echo "  $t FAIL"
done
