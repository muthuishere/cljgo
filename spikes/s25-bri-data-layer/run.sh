#!/usr/bin/env bash
# S25 driver — brings up Postgres in Docker, runs the probe, prints
# PASS/FAIL per exit criterion. Throwaway (ADR 0027).
set -euo pipefail
cd "$(dirname "$0")"

NAME=s25pg
PORT=55433
export S25_DSN="postgres://postgres:spike@127.0.0.1:${PORT}/spike"

started=0
if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}\$"; then
  if docker ps -a --format '{{.Names}}' | grep -q "^${NAME}\$"; then
    docker start "$NAME" >/dev/null
  else
    docker run -d --name "$NAME" -e POSTGRES_PASSWORD=spike -e POSTGRES_DB=spike \
      -p ${PORT}:5432 postgres:17-alpine >/dev/null
    started=1
  fi
  # wait for readiness
  for _ in $(seq 1 30); do
    if docker exec "$NAME" pg_isready -q 2>/dev/null; then break; fi
    sleep 1
  done
  sleep 2
fi

echo "== S25 data-layer probe =="
( cd probe && go run . )

if [ "$started" = "1" ]; then
  echo "(leaving $NAME running; 'docker rm -f $NAME' to clean up)"
fi
