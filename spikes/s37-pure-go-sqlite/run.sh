#!/usr/bin/env bash
# S37 harness — builds everything STATIC (CGO_ENABLED=0), proves no libsqlite
# linkage, measures size delta, runs bench/adapter/wal. Prints per-criterion
# evidence. Throwaway spike module; touches nothing outside spikes/s37.
set -euo pipefail
cd "$(dirname "$0")/probe"

export CGO_ENABLED=0
BIN="$(pwd)/s37probe"

echo "########################################################"
echo "# criterion 1 — STATIC BUILD (CGO_ENABLED=0) + linkage #"
echo "########################################################"
go build -o "$BIN" .
echo "built: $BIN"
echo "--- file ---"
file "$BIN"
echo "--- otool -L (macOS dynamic deps) ---"
if command -v otool >/dev/null; then
  otool -L "$BIN"
  # Only inspect the DEPENDENCY lines (tab-indented). The first line is the
  # binary's own path, which contains "sqlite" (…/s37-pure-go-sqlite/…) and
  # would false-positive a naive grep.
  if otool -L "$BIN" | grep '^	' | grep -qi 'libsqlite'; then
    echo ">>> FAIL: dynamic libsqlite linkage found"
  else
    echo ">>> PASS: no dynamic libsqlite linkage — deps are only the macOS system"
    echo "         libs (libSystem/libresolv) every CGO_ENABLED=0 Go binary links."
  fi
else
  echo "(otool not present — Linux? try: ldd $BIN)"
  ldd "$BIN" 2>&1 || true
fi

echo
echo "########################################################"
echo "# criterion 2 — BINARY-SIZE DELTA (battery cost)        #"
echo "########################################################"
( cd sizetest/without && go mod init cljgospike/s37/size-without >/dev/null 2>&1 || true
  go build -o without.bin . )
# 'with' needs the sqlite dep -> build it inside the probe module via a temp main
WITHOUT_BIN="$(pwd)/sizetest/without/without.bin"
WITH_BIN="$(pwd)/sizetest/with.bin"
go build -o "$WITH_BIN" ./sizetest/with
sz() { stat -f%z "$1" 2>/dev/null || stat -c%s "$1"; }
W=$(sz "$WITHOUT_BIN"); H=$(sz "$WITH_BIN")
awk -v w="$W" -v h="$H" 'BEGIN{
  printf "without sqlite : %.2f MB\n", w/1e6;
  printf "with    sqlite : %.2f MB\n", h/1e6;
  printf ">>> battery adds: %.2f MB\n", (h-w)/1e6;
}'

echo
echo "########################################################"
echo "# criterion 3/4/5 — bench, adapter, WAL/concurrency     #"
echo "########################################################"
"$BIN" all

echo
echo "== done =="
