#!/usr/bin/env bash
# S44 harness — pure-Go (CGO_ENABLED=0) crypto + composition benchmarks
# plus the JWT cross-verification correctness tests. Throwaway spike
# module; touches nothing outside spikes/s44.
set -euo pipefail
cd "$(dirname "$0")/probe"
export CGO_ENABLED=0

echo "#####################################################"
echo "# correctness — JWT cross-verify (hand-rolled <-> golang-jwt) #"
echo "#####################################################"
go test ./...

echo
echo "#####################################################"
echo "# benchmarks — crypto, JWT, guard composition, pw   #"
echo "#####################################################"
go test -run XXX -bench . -benchtime=200ms

echo
echo "== done =="
