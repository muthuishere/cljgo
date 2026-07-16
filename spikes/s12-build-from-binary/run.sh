#!/usr/bin/env bash
# S12 — reproduce every measurement in VERDICT.md.
# Writes ONLY into ./scratch (gitignored). Needs: go 1.26+, network,
# a cljgo source tree (this repo) to build the emitting binary from.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
S="$HERE/scratch"
# GOMODCACHE contents are read-only; make them writable before wiping.
[ -d "$S" ] && chmod -R u+w "$S"
rm -rf "$S" && mkdir -p "$S"

TAG=v0.1.0
export GOPROXY=https://proxy.golang.org
export GOFLAGS=

echo "== 1. build the emitting cljgo binary from this tree =="
go -C "$REPO" build -o "$S/cljgo" ./cmd/cljgo
"$S/cljgo" version

echo "== 2. emit a generated module (today's replace-based path) =="
mkdir -p "$S/work" && cp "$HERE/fixtures/hello.clj" "$S/work/"
(cd "$S/work" && CLJGO_SRC="$REPO" "$S/cljgo" build -gen "$S/work/gen" hello.clj)
"$S/work/hello"

echo "== 3. no-replace module: require $TAG from the public proxy =="
mkdir -p "$S/noreplace"
cp "$S/work/gen/main.go" "$S/noreplace/"
sed "s/v0\.1\.0/$TAG/" "$HERE/fixtures/go.mod.noreplace" > "$S/noreplace/go.mod"

echo "-- cold-cache download (fresh GOMODCACHE) --"
export GOMODCACHE="$S/gomodcache"
time go -C "$S/noreplace" mod download github.com/muthuishere/cljgo@$TAG
du -sh "$GOMODCACHE"
du -sk "$GOMODCACHE"/cache/download/github.com/muthuishere/cljgo/@v/*.zip

echo "-- tidy + build + run --"
(cd "$S/noreplace" && go mod tidy && go build -o hello-noreplace . && ./hello-noreplace)

echo "== 4. timings: cold GOCACHE, no-replace vs replace =="
GOCACHE="$S/gocache-a" go -C "$S/noreplace" clean -cache 2>/dev/null || true
time GOCACHE="$S/gocache-a" go -C "$S/noreplace" build -o /dev/null .
time GOCACHE="$S/gocache-b" go -C "$S/work/gen" build -o /dev/null .
echo "-- warm rebuild --"
touch "$S/noreplace/main.go"
time go -C "$S/noreplace" build -o /dev/null .

echo "== 5. binary sizes (cljgo's GoBuild flags: -trimpath -ldflags '-s -w') =="
go -C "$S/noreplace" build -trimpath -ldflags="-s -w" -o hello-nr-stripped .
go -C "$S/work/gen" build -trimpath -ldflags="-s -w" -o hello-r-stripped .
ls -l "$S/noreplace/hello-nr-stripped" "$S/work/gen/hello-r-stripped"

echo "== 6. skew probes =="
echo "-- unpublished tag --"
go mod download github.com/muthuishere/cljgo@v0.9.9 2>&1 | head -2 || true
echo "-- pseudo-version for an untagged pushed commit --"
(cd "$S/noreplace" && go list -m github.com/muthuishere/cljgo@main) || true

echo "== done; see VERDICT.md for the recorded numbers =="
