#!/usr/bin/env bash
# S26 driver — the in-memory (core.async) worker + the durable Postgres
# queue, both run, PASS/FAIL per exit criterion. Throwaway (ADR 0027).
set -euo pipefail
cd "$(dirname "$0")"

NAME=s25pg   # reuse S25's Postgres
PORT=55433
export S26_DSN="postgres://postgres:spike@127.0.0.1:${PORT}/spike"

if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}\$"; then
  if docker ps -a --format '{{.Names}}' | grep -q "^${NAME}\$"; then
    docker start "$NAME" >/dev/null
  else
    docker run -d --name "$NAME" -e POSTGRES_PASSWORD=spike -e POSTGRES_DB=spike \
      -p ${PORT}:5432 postgres:17-alpine >/dev/null
  fi
  for _ in $(seq 1 30); do docker exec "$NAME" pg_isready -q 2>/dev/null && break; sleep 1; done
  sleep 2
fi

echo "== S26 criterion 1: in-memory worker on clojure.core.async (interpreted) =="
( cd ../.. && go run ./cmd/cljgo run spikes/s26-bri-jobs/chan-queue.cljg )

echo
echo "== S26 criteria 2-8: durable Postgres queue =="
( cd probe && go run . )
