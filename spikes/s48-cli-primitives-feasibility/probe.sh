#!/usr/bin/env bash
# s48 — feasibility probe: is every bri.cli primitive's backing pure-Go and
# cross-compilable (the sacred CGO_ENABLED=0 + `cljgo dist` constraint)?
# We WRAP thinly or reimplement; this proves the ceiling is pure-Go so the
# own-vs-wrap choice is never forced by cgo.
set -euo pipefail
d=$(mktemp -d); cd "$d"; go mod init s48probe >/dev/null 2>&1
cat > main.go <<'EOF'
package main
import (
	_ "github.com/kardianos/service"  // system service: systemd/launchd/winsvc
	_ "github.com/robfig/cron/v3"     // scheduler
	_ "github.com/zalando/go-keyring" // secrets/keystore (s39: cgo-free)
	_ "github.com/pb33f/libopenapi"   // openapi-as-builder
)
func main() {}
EOF
GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1
CGO_ENABLED=0 go build -o /tmp/s48all . && echo "host build CGO_ENABLED=0: OK ($(du -h /tmp/s48all|cut -f1))"
echo "cgo in closure: $(CGO_ENABLED=0 go list -deps . | grep -cE 'runtime/cgo|mattn/go-sqlite3')"
for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  CGO_ENABLED=0 GOOS=${t%/*} GOARCH=${t#*/} go build -o /dev/null . 2>/dev/null && echo "  $t OK" || echo "  $t FAIL"
done
