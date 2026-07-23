#!/usr/bin/env bash
# s45 comparison corpus: rebuild + smoke-test all six runtimes serially.
# Idempotent: removes any prior container, rebuilds the image, runs, curls
# both routes, asserts exact status/content-type/body, then cleans up.
# NOT a load benchmark — that must be run serially by the orchestrator.
set -uo pipefail

cd "$(dirname "$0")"

# runtime : dir : host-port : json-msg-runtime-name
RUNTIMES=(
  "go:go:8091:go"
  "ring-jetty:clj-ring-jetty:8092:ring-jetty"
  "http-kit:clj-httpkit:8093:http-kit"
  "bun:bun:8094:bun"
  "node:node:8095:node"
  "deno:deno:8096:deno"
)

pass_count=0
fail_count=0
declare -a RESULTS

expect() { # label actual expected
  if [ "$2" = "$3" ]; then
    echo "    ok: $1"
    return 0
  fi
  echo "    FAIL: $1 -> expected [$3] got [$2]"
  return 1
}

wait_ready() { # url
  for _ in $(seq 1 60); do
    if curl -sf -o /dev/null "$1" 2>/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}

for entry in "${RUNTIMES[@]}"; do
  IFS=":" read -r name dir port rt <<<"$entry"
  img="s45-$name"
  cname="s45-$name"
  echo "=== $name ($dir) ==="

  docker rm -f "$cname" >/dev/null 2>&1

  if ! docker build -t "$img" "$dir"; then
    echo "    build FAILED"
    RESULTS+=("$name|BUILD-FAIL|no")
    fail_count=$((fail_count+1))
    continue
  fi

  if ! docker run -d -p "$port:8080" --name "$cname" "$img" >/dev/null; then
    echo "    run FAILED"
    RESULTS+=("$name|RUN-FAIL|no")
    fail_count=$((fail_count+1))
    continue
  fi

  ok=1
  if ! wait_ready "http://localhost:$port/"; then
    echo "    server never became ready"
    ok=0
  else
    # Route 1: GET /
    code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$port/")
    ctype=$(curl -s -o /dev/null -w '%{content_type}' "http://localhost:$port/")
    body=$(curl -s "http://localhost:$port/")
    expect "/ status"       "$code"  "200" || ok=0
    expect "/ ctype"        "${ctype%%;*}" "text/plain" || ok=0
    expect "/ body"         "$body"  "$(printf 'hello\n')" || ok=0

    # Route 2: GET /api/hello
    code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$port/api/hello")
    ctype=$(curl -s -o /dev/null -w '%{content_type}' "http://localhost:$port/api/hello")
    body=$(curl -s "http://localhost:$port/api/hello")
    expect "/api/hello status" "$code" "200" || ok=0
    expect "/api/hello ctype"  "${ctype%%;*}" "application/json" || ok=0
    expect "/api/hello body"   "$body" "{\"msg\":\"hello from $rt\"}" || ok=0
  fi

  size=$(docker images "$img" --format '{{.Size}}' | head -1)

  docker rm -f "$cname" >/dev/null 2>&1

  if [ "$ok" = "1" ]; then
    echo "    SMOKE PASS  (image $size)"
    RESULTS+=("$name|$size|yes")
    pass_count=$((pass_count+1))
  else
    echo "    SMOKE FAIL  (image $size)"
    RESULTS+=("$name|$size|no")
    fail_count=$((fail_count+1))
  fi
  echo
done

echo "================ SUMMARY ================"
printf '%-12s %-12s %s\n' "runtime" "image" "smoke"
for r in "${RESULTS[@]}"; do
  IFS="|" read -r n s p <<<"$r"
  printf '%-12s %-12s %s\n' "$n" "$s" "$p"
done
echo "pass=$pass_count fail=$fail_count"
[ "$fail_count" = "0" ]
