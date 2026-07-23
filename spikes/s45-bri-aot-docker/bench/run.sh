#!/usr/bin/env bash
# Serial web-framework benchmark for spike s45 (ADR 0071). ONE container at a
# time — contention skews numbers, so nothing runs in parallel. For each
# runtime we build its image, cold-start it, warm it, load-test both routes
# with oha, sample peak RSS via `docker stats`, and record image size. Output
# is a markdown table at bench/results.md.
#
#   bash run.sh                 # all runtimes present under ../compare (+ ../bri if built)
#   DURATION=30s CONC=100 bash run.sh
#
# The cljgo entrant is the FLAGSHIP bri.http (../bri), not a raw net/http
# server — that is the whole point of the spike.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
DURATION="${DURATION:-20s}"      # oha load duration per route
CONC="${CONC:-50}"               # concurrent connections
HOSTPORT="${HOSTPORT:-8080}"     # host port mapped to the container's 8080
WARM="${WARM:-3s}"               # warm-up before measuring (JVM JIT etc.)
OUT="$HERE/results.md"

# Entries auto-discover: the flagship bri first (../bri, once the AOT agent
# lands it), then every ../compare/<name> dir that has a Dockerfile. So new
# language servers are picked up with no edit here.
ENTRIES=()
[ -f "$ROOT/bri/Dockerfile" ] && ENTRIES+=("bri:$ROOT/bri")
for d in "$ROOT"/compare/*/; do
  [ -f "$d/Dockerfile" ] || continue
  ENTRIES+=("$(basename "$d"):${d%/}")
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }; }
need docker; need oha; need curl

# oha JSON → "reqps p50ms p99ms"
oha_json() {
  local url="$1"
  oha -z "$DURATION" -c "$CONC" --no-tui --output-format json "$url" 2>/dev/null | \
    python3 -c '
import sys,json
d=json.load(sys.stdin)
s=d["summary"]; pl=d.get("latencyPercentiles",{})
def ms(x): return round((x or 0)*1000,2)
print(f'"'"'{round(s["requestsPerSec"],0):.0f} {ms(pl.get("p50"))} {ms(pl.get("p99"))}'"'"')'
}

# max RSS (MiB) sampled from docker stats while a load runs in the background
peak_rss_during() {
  local cname="$1" url="$2" max=0
  oha -z "$DURATION" -c "$CONC" --no-tui "$url" >/dev/null 2>&1 &
  local lpid=$!
  while kill -0 "$lpid" 2>/dev/null; do
    local mem
    mem=$(docker stats --no-stream --format '{{.MemUsage}}' "$cname" 2>/dev/null | awk '{print $1}')
    # normalize KiB/MiB/GiB → MiB
    local n unit val
    n=$(echo "$mem" | sed -E 's/[A-Za-z]+//g'); unit=$(echo "$mem" | sed -E 's/[0-9.]+//g')
    case "$unit" in
      GiB) val=$(echo "$n*1024" | bc -l 2>/dev/null);;
      KiB) val=$(echo "$n/1024" | bc -l 2>/dev/null);;
      *)   val="$n";;
    esac
    awk -v a="$val" -v b="$max" 'BEGIN{exit !(a>b)}' 2>/dev/null && max="$val"
    sleep 0.5
  done
  printf '%.1f' "${max:-0}"
}

wait_ready() { # returns cold-start ms (docker run → first 200)
  local t0 t1
  t0=$(python3 -c 'import time;print(time.time())')
  for _ in $(seq 1 300); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$HOSTPORT/" 2>/dev/null; then
      t1=$(python3 -c 'import time;print(time.time())')
      python3 -c "print(round(($t1-$t0)*1000))"; return 0
    fi
    sleep 0.1
  done
  echo "TIMEOUT"; return 1
}

echo "# s45 web benchmark — $(date '+%Y-%m-%d %H:%M')" > "$OUT"
{
  echo
  echo "oha: duration=$DURATION concurrency=$CONC warm=$WARM · one container at a time · Docker $(docker version -f '{{.Server.Version}}' 2>/dev/null)"
  echo
  echo "| runtime | image | cold-start | / req/s | / p99 ms | /api req/s | /api p99 ms | peak RSS |"
  echo "|---|--:|--:|--:|--:|--:|--:|--:|"
} >> "$OUT"

for e in "${ENTRIES[@]}"; do
  name="${e%%:*}"; dir="${e#*:}"
  [ -d "$dir" ] || { echo ">> skip $name (no $dir yet)"; continue; }
  echo ">> $name — building"
  img="s45-$name"
  # bri's Dockerfile compiles cljgo+app, so its build context is the REPO
  # ROOT (four levels up from bench/), not the app dir.
  if [ "$name" = "bri" ]; then
    build_ok() { docker build -q -f "$dir/Dockerfile" -t "$img" "$(cd "$ROOT/../.." && pwd)" >/dev/null 2>&1; }
  else
    build_ok() { docker build -q -t "$img" "$dir" >/dev/null 2>&1; }
  fi
  if ! build_ok; then
    echo "   BUILD FAILED"; echo "| $name | BUILD FAILED | | | | | | |" >> "$OUT"; continue
  fi
  size=$(docker images "$img" --format '{{.Size}}' | head -1)
  docker rm -f "bench-$name" >/dev/null 2>&1
  docker run -d --name "bench-$name" -p "$HOSTPORT:8080" "$img" >/dev/null 2>&1
  cold=$(wait_ready) || { echo "   never came up"; docker rm -f "bench-$name" >/dev/null 2>&1; echo "| $name | $size | DNF | | | | | |" >> "$OUT"; continue; }
  echo "   up in ${cold}ms, warming"
  oha -z "$WARM" -c "$CONC" --no-tui "http://127.0.0.1:$HOSTPORT/" >/dev/null 2>&1
  read r_rps r_p50 r_p99 <<< "$(oha_json "http://127.0.0.1:$HOSTPORT/")"
  read a_rps a_p50 a_p99 <<< "$(oha_json "http://127.0.0.1:$HOSTPORT/api/hello")"
  rss=$(peak_rss_during "bench-$name" "http://127.0.0.1:$HOSTPORT/api/hello")
  echo "   / ${r_rps} rps p99 ${r_p99}ms | /api ${a_rps} rps p99 ${a_p99}ms | peak ${rss}MiB"
  echo "| $name | $size | ${cold}ms | ${r_rps} | ${r_p99} | ${a_rps} | ${a_p99} | ${rss}MiB |" >> "$OUT"
  docker rm -f "bench-$name" >/dev/null 2>&1
done

echo; echo "=== results ==="; cat "$OUT"
