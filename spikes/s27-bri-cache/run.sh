#!/usr/bin/env bash
# S27 driver — in-process + Redis cache probe, plus the config-composition
# proof through the REAL shipped bri.config. Throwaway (ADR 0027).
set -euo pipefail
cd "$(dirname "$0")"

NAME=s27redis
PORT=56380
export S27_REDIS="127.0.0.1:${PORT}"

if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}\$"; then
  if docker ps -a --format '{{.Names}}' | grep -q "^${NAME}\$"; then
    docker start "$NAME" >/dev/null
  else
    docker run -d --name "$NAME" -p ${PORT}:6379 redis:7-alpine >/dev/null
  fi
  sleep 2
fi

echo "== S27 criterion 5: cache config through the REAL bri.config =="
( cd ../.. && APP_CACHE__TTL=60 go run ./cmd/cljgo run spikes/s27-bri-cache/config-probe.cljg )

echo
echo "== S27 criteria 1-4,6: in-process + Redis cache =="
( cd probe && go run . )
